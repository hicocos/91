package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/video-site/backend/internal/catalog"
)

func TestAudioHandlersListDetailAndRejectWrongMedia(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })
	now := time.Now()
	for _, item := range []*catalog.Video{
		{ID: "video", DriveID: "drive", FileID: "video", Title: "Video", MediaType: catalog.MediaTypeVideo, Ext: "mp4", Size: 100, PublishedAt: now, CreatedAt: now},
		{ID: "audio-1", DriveID: "drive", FileID: "audio-1", Title: "Song one", Author: "Artist", MediaType: catalog.MediaTypeAudio, Ext: "flac", Size: 2048, DurationSeconds: 125, PublishedAt: now.Add(time.Second), CreatedAt: now.Add(time.Second)},
		{ID: "audio-2", DriveID: "drive", FileID: "audio-2", Title: "Song two", MediaType: catalog.MediaTypeAudio, Ext: "mp3", Size: 1024, PublishedAt: now, CreatedAt: now},
	} {
		if err := cat.UpsertVideo(ctx, item); err != nil {
			t.Fatalf("upsert %s: %v", item.ID, err)
		}
	}
	srv := &Server{Catalog: cat}
	router := chi.NewRouter()
	router.Get("/api/audios", srv.handleAudios)
	router.Get("/api/audio/{id}", srv.handleAudioDetail)
	router.Get("/api/video/{id}", srv.handleVideoDetail)

	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, httptest.NewRequest(http.MethodGet, "/api/audios?page=1&size=10", nil))
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	var list struct {
		Items []AudioDTO `json:"items"`
		Total int        `json:"total"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Total != 2 || len(list.Items) != 2 || list.Items[0].Href != "/audio/audio-1" {
		t.Fatalf("audio list = %#v", list)
	}

	detailRR := httptest.NewRecorder()
	router.ServeHTTP(detailRR, httptest.NewRequest(http.MethodGet, "/api/audio/audio-1", nil))
	if detailRR.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailRR.Code, detailRR.Body.String())
	}
	var detail AudioDetailDTO
	if err := json.Unmarshal(detailRR.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.AudioSrc != "/p/stream/audio-1" || detail.Ext != "flac" || detail.Size != 2048 {
		t.Fatalf("audio detail = %#v", detail)
	}
	if len(detail.RelatedAudios) != 1 || detail.RelatedAudios[0].ID != "audio-2" {
		t.Fatalf("related audios = %#v", detail.RelatedAudios)
	}

	for _, path := range []string{"/api/audio/video", "/api/video/audio-1"} {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d body=%s, want 404", path, rr.Code, rr.Body.String())
		}
	}
}
