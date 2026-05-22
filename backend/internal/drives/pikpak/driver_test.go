package pikpak

import (
	"testing"
	"time"

	"github.com/video-site/backend/internal/drives"
)

func TestNewDefaults(t *testing.T) {
	d := New(Config{
		ID:       "pikpak-main",
		Username: "user@example.com",
		Password: "secret",
		RootID:   "0",
	})

	if d.Kind() != "pikpak" {
		t.Fatalf("kind = %q, want pikpak", d.Kind())
	}
	if d.ID() != "pikpak-main" {
		t.Fatalf("id = %q, want pikpak-main", d.ID())
	}
	if d.RootID() != "" {
		t.Fatalf("root id = %q, want empty PikPak root", d.RootID())
	}
	if d.platform != "web" {
		t.Fatalf("platform = %q, want web", d.platform)
	}
	if d.deviceID == "" {
		t.Fatal("device id should be generated")
	}
	if d.userAgent == "" {
		t.Fatal("user agent should be selected")
	}
}

func TestFileToEntry(t *testing.T) {
	mod := time.Date(2026, 5, 10, 12, 30, 0, 0, time.UTC)
	f := file{
		ID:            "file-id",
		Name:          "movie.mp4",
		Kind:          "drive#file",
		Hash:          "hash-value",
		Size:          "12345",
		ThumbnailLink: "https://thumbnail.example/movie.jpg",
		ModifiedTime:  mod,
	}

	got := fileToEntry(f, "parent-id")

	if got.ID != "file-id" {
		t.Fatalf("id = %q, want file-id", got.ID)
	}
	if got.Name != "movie.mp4" {
		t.Fatalf("name = %q, want movie.mp4", got.Name)
	}
	if got.IsDir {
		t.Fatal("file should not be a directory")
	}
	if got.Size != 12345 {
		t.Fatalf("size = %d, want 12345", got.Size)
	}
	if got.ParentID != "parent-id" {
		t.Fatalf("parent id = %q, want parent-id", got.ParentID)
	}
	if got.MimeType != "video/mp4" {
		t.Fatalf("mime = %q, want video/mp4", got.MimeType)
	}
	if got.ThumbnailURL != "https://thumbnail.example/movie.jpg" {
		t.Fatalf("thumbnail = %q, want remote thumbnail", got.ThumbnailURL)
	}
	if got.Hash != "hash-value" {
		t.Fatalf("hash = %q, want hash-value", got.Hash)
	}
	if !got.ModTime.Equal(mod) {
		t.Fatalf("mod time = %v, want %v", got.ModTime, mod)
	}
}

func TestFolderToEntry(t *testing.T) {
	f := file{
		ID:   "folder-id",
		Name: "Videos",
		Kind: "drive#folder",
	}

	got := fileToEntry(f, "")

	if !got.IsDir {
		t.Fatal("folder should be a directory")
	}
	if got.Size != 0 {
		t.Fatalf("size = %d, want 0", got.Size)
	}
}

func TestEnsureDirStillUnsupported(t *testing.T) {
	d := New(Config{ID: "pikpak-main"})

	if _, err := d.EnsureDir(nil, "/previews"); err != drives.ErrNotSupported {
		t.Fatalf("EnsureDir error = %v, want ErrNotSupported", err)
	}
	// Upload 的真实实现见 upload_test.go。
}
