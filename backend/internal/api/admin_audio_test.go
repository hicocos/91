package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/video-site/backend/internal/catalog"
)

func TestAdminMediaListsAndStatsStaySeparated(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cat.Close() })
	now := time.Now()
	for _, item := range []*catalog.Video{
		{ID: "video", DriveID: "drive", FileID: "v", Title: "Video", MediaType: catalog.MediaTypeVideo, Size: 100, PublishedAt: now, CreatedAt: now},
		{ID: "audio", DriveID: "drive", FileID: "a", Title: "Audio", MediaType: catalog.MediaTypeAudio, Ext: "mp3", Size: 200, PublishedAt: now, CreatedAt: now},
	} {
		if err := cat.UpsertVideo(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	server := &AdminServer{Catalog: cat}

	assertList := func(path, wantID string) {
		t.Helper()
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if path == "/admin/api/audios" {
			server.handleAdminListAudios(rr, req)
		} else {
			server.handleAdminListVideos(rr, req)
		}
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		var body struct {
			Items []adminVideoDTO `json:"items"`
			Total int             `json:"total"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Total != 1 || len(body.Items) != 1 || body.Items[0].ID != wantID {
			t.Fatalf("%s body=%#v", path, body)
		}
	}
	assertList("/admin/api/videos", "video")
	assertList("/admin/api/audios", "audio")

	rr := httptest.NewRecorder()
	server.handleAudioStats(rr, httptest.NewRequest(http.MethodGet, "/admin/api/audios/stats", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("stats status=%d body=%s", rr.Code, rr.Body.String())
	}
	var stats struct {
		Current int `json:"current"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &stats); err != nil || stats.Current != 1 {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}

	counts, err := cat.CountMediaByDrive(ctx)
	if err != nil || counts["drive"].Video != 1 || counts["drive"].Audio != 1 {
		t.Fatalf("counts=%#v err=%v", counts, err)
	}
}
