package api

import (
	"net/http"
	"strconv"

	"github.com/video-site/backend/internal/catalog"
)

func (a *AdminServer) handleAdminListAudios(w http.ResponseWriter, r *http.Request) {
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
		Keyword: q.Get("keyword"), DriveID: q.Get("driveId"), MediaType: catalog.MediaTypeAudio,
		Page: page, PageSize: size,
	})
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
	metadata, err := a.Catalog.ListVideoTagMetadata(r.Context(), ids)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	mapped := mapAdminVideos(items)
	for i := range mapped {
		entries := metadata[mapped[i].ID]
		if len(entries) == 0 {
			continue
		}
		mapped[i].TagSources = make(map[string]string, len(entries))
		mapped[i].TagEvidence = make(map[string]string, len(entries))
		for label, entry := range entries {
			mapped[i].TagSources[label] = entry.Source
			if entry.Evidence != "" {
				mapped[i].TagEvidence[label] = entry.Evidence
			}
		}
		if len(mapped[i].TagEvidence) == 0 {
			mapped[i].TagEvidence = nil
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": mapped, "total": total, "page": page, "size": size})
}

func (a *AdminServer) handleAudioStats(w http.ResponseWriter, r *http.Request) {
	_, total, err := a.Catalog.ListVideos(r.Context(), catalog.ListParams{MediaType: catalog.MediaTypeAudio, Page: 1, PageSize: 1})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": total})
}
