package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/video-site/backend/internal/catalog"
)

type AudioDTO struct {
	ID          string   `json:"id"`
	Href        string   `json:"href"`
	Title       string   `json:"title"`
	Author      string   `json:"author"`
	Duration    string   `json:"duration"`
	Size        int64    `json:"size"`
	Ext         string   `json:"ext"`
	SourceLabel string   `json:"sourceLabel,omitempty"`
	Views       int      `json:"views"`
	PublishedAt string   `json:"publishedAt"`
	Tags        []string `json:"tags"`
}

type AudioDetailDTO struct {
	AudioDTO
	AudioSrc      string     `json:"audioSrc"`
	Description   string     `json:"description"`
	RelatedAudios []AudioDTO `json:"relatedAudios"`
}

func (s *Server) handleAudios(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	if size <= 0 {
		size = 30
	}
	params := catalog.ListParams{
		Keyword:   q.Get("q"),
		Tag:       q.Get("tag"),
		MediaType: catalog.MediaTypeAudio,
		Sort:      q.Get("sort"),
		Page:      page,
		PageSize:  size,
	}
	items, total, err := s.Catalog.ListVideos(r.Context(), params)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"items": s.mapAudios(r, items),
		"total": total,
		"page":  params.Page,
		"size":  params.PageSize,
	})
}

func (s *Server) handleAudioDetail(w http.ResponseWriter, r *http.Request) {
	v, ok := s.visibleMedia(w, r, routeParam(r, "id"), catalog.MediaTypeAudio)
	if !ok {
		return
	}
	items, _, err := s.Catalog.ListVideos(r.Context(), catalog.ListParams{
		MediaType: catalog.MediaTypeAudio,
		Sort:      "latest",
		Page:      1,
		PageSize:  8,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	related := make([]*catalog.Video, 0, 6)
	for _, item := range items {
		if item.ID == v.ID {
			continue
		}
		related = append(related, item)
		if len(related) == 6 {
			break
		}
	}
	detail := AudioDetailDTO{
		AudioDTO:      s.mapAudio(r, v),
		AudioSrc:      s.videoSource(v),
		Description:   v.Description,
		RelatedAudios: s.mapAudios(r, related),
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleAudioView(w http.ResponseWriter, r *http.Request) {
	v, ok := s.visibleMedia(w, r, routeParam(r, "id"), catalog.MediaTypeAudio)
	if !ok {
		return
	}
	views, err := s.Catalog.IncrementView(r.Context(), v.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"views": views})
}

func (s *Server) visibleMedia(w http.ResponseWriter, r *http.Request, id, mediaType string) (*catalog.Video, bool) {
	v, err := s.Catalog.GetVideo(r.Context(), id)
	if err != nil || v.Hidden || v.MediaType != mediaType {
		writeErr(w, http.StatusNotFound, sql.ErrNoRows)
		return nil, false
	}
	return v, true
}

func (s *Server) mapAudio(r *http.Request, v *catalog.Video) AudioDTO {
	tags := v.Tags
	if tags == nil {
		tags = []string{}
	}
	dto := AudioDTO{
		ID:          v.ID,
		Href:        "/audio/" + pathSegment(v.ID),
		Title:       v.Title,
		Author:      v.Author,
		Duration:    formatDuration(v.DurationSeconds),
		Size:        v.Size,
		Ext:         strings.ToLower(v.Ext),
		Views:       v.Views,
		PublishedAt: v.PublishedAt.Format("2006-01-02"),
		Tags:        tags,
	}
	if d, err := s.Catalog.GetDrive(r.Context(), v.DriveID); err == nil {
		dto.SourceLabel = driveKindLabel(d.Kind)
	}
	return dto
}

func (s *Server) mapAudios(r *http.Request, items []*catalog.Video) []AudioDTO {
	out := make([]AudioDTO, 0, len(items))
	for _, item := range items {
		out = append(out, s.mapAudio(r, item))
	}
	return out
}
