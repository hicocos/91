package wopan

import (
	"errors"
	"testing"

	"github.com/video-site/backend/internal/drives"
	sdk "github.com/video-site/backend/internal/drives/wopan/internal/client"
)

func TestFileToEntryUsesDirectoryIDAndFileFID(t *testing.T) {
	dir := fileToEntry(&sdk.File{
		Id:   "dir-object-id",
		Fid:  "0",
		Type: 0,
		Name: "collection",
	}, "root")
	if !dir.IsDir {
		t.Fatal("directory entry IsDir = false")
	}
	if dir.ID != "dir-object-id" {
		t.Fatalf("directory id = %q, want object id", dir.ID)
	}

	file := fileToEntry(&sdk.File{
		Id:   "file-object-id",
		Fid:  "fid/with/slash",
		Type: 1,
		Name: "clip.mp4",
		Size: 123,
	}, "dir-object-id")
	if file.IsDir {
		t.Fatal("file entry IsDir = true")
	}
	if file.ID != "fid/with/slash" {
		t.Fatalf("file id = %q, want fid for download", file.ID)
	}
}

func TestDeleteFileIDFromWopanFileUsesObjectIDForFID(t *testing.T) {
	got, ok := deleteFileIDFromWopanFile(&sdk.File{
		Id:   "file-object-id",
		Fid:  "fid/with/slash",
		Type: 1,
		Name: "clip.mp4",
		Size: 123,
	}, drives.SourceFile{
		FileID: "fid/with/slash",
		Name:   "clip.mp4",
		Size:   123,
	})
	if !ok {
		t.Fatal("delete file id not resolved")
	}
	if got != "file-object-id" {
		t.Fatalf("delete file id = %q, want object id", got)
	}
}

func TestDeleteFileIDFromWopanFileAcceptsObjectID(t *testing.T) {
	got, ok := deleteFileIDFromWopanFile(&sdk.File{
		Id:   "file-object-id",
		Fid:  "fid-1",
		Type: 1,
		Name: "clip.mp4",
		Size: 123,
	}, drives.SourceFile{
		FileID: "file-object-id",
		Name:   "clip.mp4",
		Size:   123,
	})
	if !ok {
		t.Fatal("delete file id not resolved")
	}
	if got != "file-object-id" {
		t.Fatalf("delete file id = %q, want object id", got)
	}
}

func TestDeleteFileIDFromWopanFileRejectsIDMismatch(t *testing.T) {
	if _, ok := deleteFileIDFromWopanFile(&sdk.File{
		Id:   "file-object-id",
		Fid:  "fid-1",
		Type: 1,
		Name: "clip.mp4",
		Size: 123,
	}, drives.SourceFile{
		FileID: "other-fid",
		Name:   "clip.mp4",
		Size:   123,
	}); ok {
		t.Fatal("delete file id resolved despite id mismatch")
	}
}

func TestWopanRequestErrorWrapsRateLimit(t *testing.T) {
	err := wopanRequestError("list", errors.New("request failed with status: 429 Too Many Requests"))
	var rateLimit *drives.RateLimitError
	if !errors.As(err, &rateLimit) {
		t.Fatalf("error = %T %[1]v, want RateLimitError", err)
	}
	if rateLimit.Provider != "wopan" {
		t.Fatalf("provider = %q, want wopan", rateLimit.Provider)
	}
}

func TestWopanRequestErrorLeavesNormalErrors(t *testing.T) {
	err := wopanRequestError("download url", errors.New("invalid access token"))
	var rateLimit *drives.RateLimitError
	if errors.As(err, &rateLimit) {
		t.Fatalf("error = %T %[1]v, want non-rate-limit error", err)
	}
}
