package catalog

import (
	"context"
	"testing"
	"time"
)

func TestMediaTypeDefaultsToVideoAndRoundTripsAudio(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })

	now := time.Now()
	for _, item := range []*Video{
		{ID: "video", DriveID: "drive", FileID: "v", Title: "Video", PublishedAt: now, CreatedAt: now},
		{ID: "audio", DriveID: "drive", FileID: "a", Title: "Audio", MediaType: MediaTypeAudio, PublishedAt: now, CreatedAt: now},
	} {
		if err := cat.UpsertVideo(ctx, item); err != nil {
			t.Fatalf("upsert %s: %v", item.ID, err)
		}
	}

	video, err := cat.GetVideo(ctx, "video")
	if err != nil {
		t.Fatalf("get video: %v", err)
	}
	if video.MediaType != MediaTypeVideo {
		t.Fatalf("default media type = %q, want %q", video.MediaType, MediaTypeVideo)
	}
	audio, err := cat.GetVideo(ctx, "audio")
	if err != nil {
		t.Fatalf("get audio: %v", err)
	}
	if audio.MediaType != MediaTypeAudio {
		t.Fatalf("audio media type = %q, want %q", audio.MediaType, MediaTypeAudio)
	}
}
