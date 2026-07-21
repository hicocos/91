package s3

import (
	"context"
	"encoding/xml"

	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/video-site/backend/internal/drives"
)

func TestPathStyleListStatDeleteAndPresign(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.EscapedPath())
		if !strings.Contains(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
			t.Errorf("missing sigv4 auth")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
			_ = xml.NewEncoder(w).Encode(listResult{CommonPrefixes: []commonPrefix{{Prefix: "root/folder/"}}, Contents: []object{{Key: "root/video.mp4", Size: 12, ETag: "etag"}}})
		case r.Method == http.MethodHead:
			w.Header().Set("Content-Length", "12")
			w.Header().Set("Content-Type", "video/mp4")
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer srv.Close()
	d := New(Config{ID: "s", Endpoint: srv.URL, Region: "us-east-1", Bucket: "bucket", AccessKey: "ak", SecretKey: "sk", ForcePathStyle: true, RootPrefix: "/root//", HTTPClient: srv.Client()})
	d.allowInsecure = true
	d.allowPrivate = true
	entries, err := d.List(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ID != "root/folder/" || entries[1].ID != "root/video.mp4" || entries[1].Hash != "" {
		t.Fatalf("entries=%+v", entries)
	}
	e, err := d.Stat(context.Background(), "root/video.mp4")
	if err != nil || e.Size != 12 {
		t.Fatalf("stat=%+v err=%v", e, err)
	}
	if err := d.Remove(context.Background(), "root/video.mp4"); err != nil {
		t.Fatal(err)
	}
	link, err := d.StreamURL(context.Background(), "root/video.mp4")
	if err != nil || !strings.Contains(link.URL, "X-Amz-Signature=") {
		t.Fatalf("link=%+v err=%v", link, err)
	}
}

func TestNormalizePrefix(t *testing.T) {
	if got := normalizePrefix(" /a//b/ "); got != "a/b/" {
		t.Fatalf("got %q", got)
	}
}

func TestS3DoesNotAdvertiseOptionalUploadInterfaces(t *testing.T) {
	d := any(New(Config{}))
	if _, ok := d.(drives.Uploader); ok {
		t.Fatal("S3 unexpectedly implements drives.Uploader")
	}
	if _, ok := d.(drives.DirectoryEnsurer); ok {
		t.Fatal("S3 unexpectedly implements drives.DirectoryEnsurer")
	}
}

func TestRootBoundaryRejectsSiblingPrefix(t *testing.T) {
	d := New(Config{RootPrefix: "root"})
	if _, err := d.resolveObject("rooted/file"); err == nil {
		t.Fatal("sibling prefix escaped configured root")
	}
	if got, err := d.resolveDir("rooted"); err == nil || got != "" {
		t.Fatalf("outside directory resolved to %q with err=%v", got, err)
	}
}

func TestInvalidListAndStatFailWithoutHTTP(t *testing.T) {
	calls := 0
	d := New(Config{RootPrefix: "tenant-a", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, nil
	})}})
	if _, err := d.List(context.Background(), "tenant-b"); err == nil {
		t.Fatal("outside-root List succeeded")
	}
	if _, err := d.Stat(context.Background(), "tenant-b/video.mp4"); err == nil {
		t.Fatal("outside-root Stat succeeded")
	}
	if calls != 0 {
		t.Fatalf("HTTP calls = %d, want zero", calls)
	}
}

func TestBucketRootCannotBeListed(t *testing.T) {
	d := New(Config{})
	if _, err := d.List(context.Background(), ""); err == nil {
		t.Fatal("bucket root List succeeded")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSessionTokenIsIncludedInSignedHeaders(t *testing.T) {
	d := New(Config{Endpoint: "https://s3.example.com", Region: "us-east-1", Bucket: "b", AccessKey: "ak", SecretKey: "sk", SessionToken: "session", ForcePathStyle: true})
	req, _ := http.NewRequest(http.MethodGet, d.baseURL("x"), nil)
	if err := d.sign(req, "UNSIGNED-PAYLOAD", time.Unix(0, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	auth := req.Header.Get("Authorization")
	if !strings.Contains(auth, "SignedHeaders=host;x-amz-content-sha256;x-amz-date;x-amz-security-token") {
		t.Fatalf("session token not signed: %s", auth)
	}
}
