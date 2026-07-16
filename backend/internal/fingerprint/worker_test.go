package fingerprint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/video-site/backend/internal/catalog"
	"github.com/video-site/backend/internal/drives"
)

func TestComputeLocalFilesWithSameContentMatch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	body := []byte("same video bytes")
	a := filepath.Join(dir, "a.mp4")
	b := filepath.Join(dir, "b.mp4")
	if err := os.WriteFile(a, body, 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(b, body, 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	sumA, err := Compute(ctx, &fakeDrive{paths: map[string]string{"a": a}}, &catalog.Video{ID: "a", FileID: "a", Size: int64(len(body))}, Config{}, nil)
	if err != nil {
		t.Fatalf("compute a: %v", err)
	}
	sumB, err := Compute(ctx, &fakeDrive{paths: map[string]string{"b": b}}, &catalog.Video{ID: "b", FileID: "b", Size: int64(len(body))}, Config{}, nil)
	if err != nil {
		t.Fatalf("compute b: %v", err)
	}
	if sumA == "" || sumA != sumB {
		t.Fatalf("fingerprints = %q / %q, want same non-empty", sumA, sumB)
	}
}

func TestComputeRemoteUsesRangeSamples(t *testing.T) {
	ctx := context.Background()
	data := make([]byte, 10*1024*1024)
	for i := range data {
		data[i] = byte(i % 251)
	}
	var ranges []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawRange := r.Header.Get("Range")
		ranges = append(ranges, rawRange)
		var start, end int
		if _, err := fmt.Sscanf(rawRange, "bytes=%d-%d", &start, &end); err != nil {
			t.Fatalf("bad range %q: %v", rawRange, err)
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[start : end+1])
	}))
	defer srv.Close()

	drv := &fakeDrive{paths: map[string]string{"remote": srv.URL + "/video.mp4"}}
	sum, err := Compute(ctx, drv, &catalog.Video{ID: "remote", FileID: "remote", Size: int64(len(data))}, Config{
		SampleSizeBytes: 4,
		FullHashMaxSize: 8,
		HTTPClient:      srv.Client(),
	}, srv.Client())
	if err != nil {
		t.Fatalf("compute remote: %v", err)
	}
	if sum == "" {
		t.Fatal("fingerprint should not be empty")
	}
	want := []string{
		"bytes=0-3",
		"bytes=2097151-2097154",
		"bytes=4194302-4194305",
		"bytes=6291453-6291456",
		"bytes=8388604-8388607",
	}
	if fmt.Sprint(ranges) != fmt.Sprint(want) {
		t.Fatalf("ranges = %#v, want %#v", ranges, want)
	}
}

func TestComputeRemoteFollowsWebDAV302WithoutImplicitReferer(t *testing.T) {
	ctx := context.Background()
	data := []byte("0123456789")
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Referer"); got != "" {
			http.Error(w, "signed download rejects Referer", http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization leaked to redirect target: %q", got)
		}
		if got := r.Header.Get("Cookie"); got != "" {
			t.Errorf("Cookie leaked to redirect target: %q", got)
		}
		if got := r.Header.Get("Range"); got != "bytes=0-9" {
			t.Errorf("Range = %q, want bytes=0-9", got)
		}
		w.Header().Set("Content-Range", "bytes 0-9/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data)
	}))
	t.Cleanup(cdn.Close)

	openList := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Basic secret" {
			t.Errorf("OpenList Authorization = %q, want Basic secret", got)
		}
		http.Redirect(w, r, cdn.URL+"/signed/video.mp4", http.StatusFound)
	}))
	t.Cleanup(openList.Close)

	drv := &fakeDrive{
		paths: map[string]string{"remote": openList.URL + "/dav/video.mp4"},
		headers: http.Header{
			"Authorization": {"Basic secret"},
			"Cookie":        {"dav_session=secret"},
		},
	}
	sum, err := Compute(ctx, drv, &catalog.Video{
		ID:     "remote",
		FileID: "remote",
		Size:   int64(len(data)),
	}, Config{FullHashMaxSize: int64(len(data))}, nil)
	if err != nil {
		t.Fatalf("compute redirected remote: %v", err)
	}
	if sum == "" {
		t.Fatal("fingerprint should not be empty")
	}
}

func TestComputeRemote429ReturnsRateLimit(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":429}}`))
	}))
	defer srv.Close()

	drv := &fakeDrive{paths: map[string]string{"remote": srv.URL + "/video.mp4"}}
	_, err := Compute(ctx, drv, &catalog.Video{ID: "remote", FileID: "remote", Size: 1024 * 1024}, Config{
		SampleSizeBytes: 4,
		FullHashMaxSize: 8,
		HTTPClient:      srv.Client(),
	}, srv.Client())
	if err == nil {
		t.Fatal("compute succeeded, want rate limit")
	}
	var rateLimit *drives.RateLimitError
	if !errors.As(err, &rateLimit) {
		t.Fatalf("error = %T %[1]v, want RateLimitError", err)
	}
	if rateLimit.RetryAfter != time.Minute {
		t.Fatalf("retry after = %s, want 1m", rateLimit.RetryAfter)
	}
}

func TestWopanRemoteRangeErrorsLookRateLimited(t *testing.T) {
	for _, tc := range []struct {
		rawURL string
		status int
	}{
		{rawURL: "https://gxdownload.pan.wo.cn:8445/openapi/download?fid=encoded", status: http.StatusForbidden},
		{rawURL: "https://du.smartont.net:8445/openapi/download?fid=encoded", status: http.StatusServiceUnavailable},
		{rawURL: "https://du.smartont.net:8445/openapi/download?fid=encoded", status: 509},
	} {
		if !remoteRangeResponseLooksRateLimited(tc.rawURL, tc.status, nil) {
			t.Fatalf("remoteRangeResponseLooksRateLimited(%q, %d) = false, want true", tc.rawURL, tc.status)
		}
	}
	if remoteRangeResponseLooksRateLimited("https://example.com/video.mp4", http.StatusForbidden, nil) {
		t.Fatal("generic 403 should not be treated as wopan rate limit")
	}
}

func TestGuangYaPanRemoteRangeErrorsLookRateLimited(t *testing.T) {
	for _, tc := range []struct {
		rawURL string
		status int
	}{
		{rawURL: "https://txgz02-httpdown.guangyacdn.com/download/?fid=encoded", status: http.StatusForbidden},
		{rawURL: "https://txgz02-httpdown.guangyacdn.com/download/?fid=encoded", status: http.StatusServiceUnavailable},
		{rawURL: "https://txgz02-httpdown.guangyacdn.com/download/?fid=encoded", status: 509},
	} {
		if !remoteRangeResponseLooksRateLimited(tc.rawURL, tc.status, nil) {
			t.Fatalf("remoteRangeResponseLooksRateLimited(%q, %d) = false, want true", tc.rawURL, tc.status)
		}
	}
	if remoteRangeResponseLooksRateLimited("https://example.com/video.mp4", http.StatusForbidden, nil) {
		t.Fatal("generic 403 should not be treated as guangyapan rate limit")
	}
}

func TestGoogleDriveRemoteRangeForbiddenLooksRateLimitedByURL(t *testing.T) {
	if !remoteRangeResponseLooksRateLimited("https://www.googleapis.com/drive/v3/files/file-1?alt=media", http.StatusForbidden, nil) {
		t.Fatal("google drive media 403 should be treated as rate limit by URL and status")
	}
}

type fakeDrive struct {
	paths   map[string]string
	headers http.Header
}

func (d *fakeDrive) Kind() string { return "fake" }
func (d *fakeDrive) ID() string   { return "fake" }
func (d *fakeDrive) Init(context.Context) error {
	return nil
}
func (d *fakeDrive) List(context.Context, string) ([]drives.Entry, error) {
	return nil, drives.ErrNotSupported
}
func (d *fakeDrive) Stat(context.Context, string) (*drives.Entry, error) {
	return nil, drives.ErrNotSupported
}
func (d *fakeDrive) StreamURL(_ context.Context, fileID string) (*drives.StreamLink, error) {
	return &drives.StreamLink{
		URL:     d.paths[fileID],
		Headers: d.headers.Clone(),
		Expires: time.Now().Add(time.Minute),
	}, nil
}
func (d *fakeDrive) Upload(context.Context, string, string, io.Reader, int64) (string, error) {
	return "", drives.ErrNotSupported
}
func (d *fakeDrive) EnsureDir(context.Context, string) (string, error) {
	return "", drives.ErrNotSupported
}
func (d *fakeDrive) RootID() string { return "root" }
