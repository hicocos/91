package scanner

import (
	"context"
	"testing"

	"github.com/video-site/backend/internal/catalog"
	"github.com/video-site/backend/internal/drives"
)

func TestRunClassifiesAudioSeparatelyFromVideo(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })

	drv := &scannerFakeDrive{entries: []drives.Entry{
		{ID: "video-1", Name: "clip.mp4", Size: 123},
		{ID: "audio-1", Name: "song.flac", Size: 456},
		{ID: "text-1", Name: "notes.txt", Size: 20},
	}}
	callbacks := 0
	sc := NewWithAudio(cat, drv, []string{".mp4"}, []string{".flac"}, nil, func(*catalog.Video) {
		callbacks++
	})

	stats, err := sc.Run(ctx, "")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if stats.Scanned != 2 || stats.Added != 2 {
		t.Fatalf("stats = scanned %d added %d, want 2/2", stats.Scanned, stats.Added)
	}
	if stats.VideoScanned != 1 || stats.AudioScanned != 1 || stats.VideoAdded != 1 || stats.AudioAdded != 1 {
		t.Fatalf("classified stats = video %d/%d audio %d/%d, want 1/1 each", stats.VideoScanned, stats.VideoAdded, stats.AudioScanned, stats.AudioAdded)
	}
	if callbacks != 1 {
		t.Fatalf("video callbacks = %d, want 1", callbacks)
	}
	video, err := cat.GetVideo(ctx, "fake-drive-video-1")
	if err != nil || video.MediaType != catalog.MediaTypeVideo {
		t.Fatalf("video = %#v err=%v, want video media type", video, err)
	}
	audio, err := cat.GetVideo(ctx, "fake-drive-audio-1")
	if err != nil {
		t.Fatalf("get audio: %v", err)
	}
	if audio.MediaType != catalog.MediaTypeAudio || audio.PreviewStatus != "skipped" || audio.Quality != "" {
		t.Fatalf("audio = %#v, want audio with skipped video generation", audio)
	}
}
