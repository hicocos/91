package catalog

import (
	"context"
	"testing"
	"time"
)

func TestMediaListingSeparatesAudioFromVideo(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })
	now := time.Now()
	for _, item := range []*Video{
		{ID: "video", DriveID: "drive", FileID: "video", FileName: "clip.mp4", Title: "Shared title", MediaType: MediaTypeVideo, PreviewStatus: "pending", Ext: "mp4", Size: 100, PublishedAt: now, CreatedAt: now},
		{ID: "audio", DriveID: "drive", FileID: "audio", FileName: "song.flac", Title: "Shared title", MediaType: MediaTypeAudio, PreviewStatus: "pending", Ext: "flac", Size: 200, PublishedAt: now.Add(time.Second), CreatedAt: now.Add(time.Second)},
	} {
		if err := cat.UpsertVideo(ctx, item); err != nil {
			t.Fatalf("upsert %s: %v", item.ID, err)
		}
	}

	videos, total, err := cat.ListVideos(ctx, ListParams{Keyword: "Shared", Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(videos) != 1 || videos[0].ID != "video" {
		t.Fatalf("video listing = %#v total=%d err=%v", videos, total, err)
	}
	audios, total, err := cat.ListVideos(ctx, ListParams{MediaType: MediaTypeAudio, Keyword: "Shared", Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(audios) != 1 || audios[0].ID != "audio" {
		t.Fatalf("audio listing = %#v total=%d err=%v", audios, total, err)
	}

	if items, _ := cat.ListVideosByPreviewStatus(ctx, "drive", "pending", 10); len(items) != 1 || items[0].ID != "video" {
		t.Fatalf("preview candidates = %#v, want video only", items)
	}
	if items, _ := cat.ListVideosByThumbnailStatus(ctx, "drive", "pending", 10); len(items) != 1 || items[0].ID != "video" {
		t.Fatalf("thumbnail candidates = %#v, want video only", items)
	}
	if items, _ := cat.ListTranscodeCandidates(ctx, "drive", 10); len(items) != 0 {
		t.Fatalf("transcode candidates = %#v, want no audio", items)
	}
	if items, _ := cat.ListVideosNeedingFingerprint(ctx, "drive", 10); len(items) != 1 || items[0].ID != "video" {
		t.Fatalf("fingerprint candidates = %#v, want video only", items)
	}
	transcodes, err := cat.CountTranscodesByDrive(ctx)
	if err != nil || transcodes["drive"].Skipped != 0 {
		t.Fatalf("transcode counts = %#v err=%v, audio must not count as skipped video", transcodes, err)
	}
	fingerprints, err := cat.CountFingerprintsByDrive(ctx)
	if err != nil || fingerprints["drive"].Pending != 1 {
		t.Fatalf("fingerprint counts = %#v err=%v, want one pending video", fingerprints, err)
	}
	current, _, err := cat.VideoManagementCounts(ctx)
	if err != nil || current != 1 {
		t.Fatalf("video management current=%d err=%v, want video only", current, err)
	}
	ids, err := cat.ListVisibleVideoIDs(ctx)
	if err != nil || len(ids) != 1 || ids[0] != "video" {
		t.Fatalf("visible video ids = %#v err=%v", ids, err)
	}
}
