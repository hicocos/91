package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/video-site/backend/internal/catalog"
)

func (a *AdminServer) handleAdminListVideos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 100
	}
	items, total, err := a.Catalog.ListVideos(r.Context(), catalog.ListParams{
		Keyword:   q.Get("keyword"),
		DriveID:   q.Get("driveId"),
		MediaType: catalog.MediaTypeVideo,
		Page:      page,
		PageSize:  size,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if a.GetPreviewGenerationVideoIDs != nil {
		generating := a.GetPreviewGenerationVideoIDs()
		for _, item := range items {
			if item != nil && generating[item.ID] {
				item.PreviewStatus = "generating"
			}
		}
	}
	videoIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil {
			videoIDs = append(videoIDs, item.ID)
		}
	}
	tagMetadata, err := a.Catalog.ListVideoTagMetadata(r.Context(), videoIDs)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	mappedItems := mapAdminVideos(items)
	for i := range mappedItems {
		metadata := tagMetadata[mappedItems[i].ID]
		if len(metadata) == 0 {
			continue
		}
		mappedItems[i].TagSources = make(map[string]string, len(metadata))
		mappedItems[i].TagEvidence = make(map[string]string, len(metadata))
		for label, item := range metadata {
			mappedItems[i].TagSources[label] = item.Source
			if item.Evidence != "" {
				mappedItems[i].TagEvidence[label] = item.Evidence
			}
		}
		if len(mappedItems[i].TagEvidence) == 0 {
			mappedItems[i].TagEvidence = nil
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": mappedItems,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// handleVideoStats 返回后台视频管理两个标签页的计数（当前/拉黑）。
func (a *AdminServer) handleVideoStats(w http.ResponseWriter, r *http.Request) {
	current, blacklisted, err := a.Catalog.VideoManagementCounts(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"current":     current,
		"blacklisted": blacklisted,
	})
}

// handleListBlacklist 分页返回黑名单（墓碑）视频。
func (a *AdminServer) handleListBlacklist(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 100
	}
	items, total, err := a.Catalog.ListDeletedVideos(r.Context(), catalog.ListParams{
		Keyword:  q.Get("keyword"),
		DriveID:  q.Get("driveId"),
		Page:     page,
		PageSize: size,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

const blacklistSourceDeleteAction = "delete-blacklist-sources"

func (a *AdminServer) handlePrepareBlacklistSourceDelete(w http.ResponseWriter, r *http.Request) {
	var body BlacklistSourceDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := normalizeBlacklistSourceDeleteRequest(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if a.Catalog == nil {
		writeErr(w, http.StatusInternalServerError, errors.New("catalog is required"))
		return
	}
	var (
		items []*catalog.DeletedVideo
		err   error
	)
	if body.DeleteAllSources {
		items, err = a.Catalog.ListDeletedVideosPendingSourceDeletion(r.Context())
	} else {
		items, err = a.Catalog.ListDeletedVideosPendingSourceDeletionByIDs(r.Context(), body.IDs)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil {
			ids = append(ids, item.ID)
		}
	}
	sort.Strings(ids)
	scope := blacklistSourceDeleteScope(body.DeleteAllSources, ids)
	nonce, expiresAt, err := a.prepareDestructiveConfirmationWithSnapshot(r, blacklistSourceDeleteAction, scope, ids)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"nonce":     nonce,
		"expiresAt": expiresAt.UTC().Format(time.RFC3339Nano),
		"ids":       ids,
	})
}

func (a *AdminServer) handleStartBlacklistSourceDelete(w http.ResponseWriter, r *http.Request) {
	var body BlacklistSourceDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := normalizeBlacklistSourceDeleteRequest(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	requestedIDs := append([]string(nil), body.IDs...)
	sort.Strings(requestedIDs)
	scope := blacklistSourceDeleteScope(body.DeleteAllSources, requestedIDs)
	confirmation, ok := a.consumeDestructiveConfirmation(r, body.Nonce, blacklistSourceDeleteAction, scope)
	if !ok {
		writeErr(w, http.StatusPreconditionFailed, errors.New("source deletion confirmation is invalid or expired"))
		return
	}
	if body.DeleteAllSources {
		body.DeleteAllSources = false
		body.IDs = append([]string(nil), confirmation.Snapshot...)
	}
	body.Nonce = ""
	accepted := false
	if a.OnStartBlacklistSourceDelete != nil {
		accepted = a.OnStartBlacklistSourceDelete(body)
	}
	resp := map[string]any{
		"ok":       true,
		"accepted": accepted,
		"status":   a.blacklistSourceDeleteStatus(r.Context()),
	}
	if !accepted {
		resp["message"] = "黑名单源文件删除任务已在运行"
	}
	writeJSON(w, http.StatusAccepted, resp)
}

func blacklistSourceDeleteScope(deleteAll bool, ids []string) string {
	if deleteAll {
		return "all"
	}
	return "ids:" + strings.Join(ids, ",")
}

func normalizeBlacklistSourceDeleteRequest(req *BlacklistSourceDeleteRequest) error {
	if req == nil {
		return errors.New("blacklist source delete request is required")
	}
	seen := make(map[string]bool, len(req.IDs))
	ids := req.IDs[:0]
	for _, id := range req.IDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	req.IDs = ids

	hasIDs := len(req.IDs) > 0
	switch {
	case req.DeleteAllSources && hasIDs:
		return errors.New("deleteAllSources and ids cannot be used together")
	case !req.DeleteAllSources && !hasIDs:
		return errors.New("deleteAllSources=true or ids is required")
	default:
		return nil
	}
}

func (a *AdminServer) handleBlacklistSourceDeleteStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.blacklistSourceDeleteStatus(r.Context()))
}

func (a *AdminServer) blacklistSourceDeleteStatus(ctx context.Context) BlacklistSourceDeleteStatus {
	var status BlacklistSourceDeleteStatus
	if a.GetBlacklistSourceDeleteStatus == nil {
		status.State = "idle"
	} else {
		status = a.GetBlacklistSourceDeleteStatus()
	}
	if status.State == "" {
		status.State = "idle"
	}
	if a.Catalog != nil {
		if pending, err := a.Catalog.CountDeletedVideosPendingSourceDeletion(ctx); err == nil {
			status.Pending = pending
		}
	}
	return status
}

// handleRemoveBlacklist 允许视频在后续手动/定时任务中重新入库，不会立即触发
// 扫盘或爬取。不可重新发现的来源会返回 409。
func (a *AdminServer) handleRemoveBlacklist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.Catalog.RemoveDeletedVideo(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, catalog.ErrDeletedVideoNotRestorable) {
			writeErr(w, http.StatusConflict, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
