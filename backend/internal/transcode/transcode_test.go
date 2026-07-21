package transcode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/video-site/backend/internal/catalog"
	"github.com/video-site/backend/internal/drives"
)

func TestDownloadRestrictedSourceRejectsLoopback(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte("private"))
	}))
	defer srv.Close()
	w := &Worker{hc: http.DefaultClient}
	err := w.downloadTo(context.Background(), &drives.StreamLink{URL: srv.URL + "/video", PublicNetworkOnly: true, Expires: time.Now().Add(time.Minute)}, filepath.Join(t.TempDir(), "video.tmp"))
	if err == nil {
		t.Fatal("restricted transcode download reached loopback")
	}
	if called {
		t.Fatal("restricted transcode download sent request to loopback")
	}
}

func TestNeedsTranscode(t *testing.T) {
	cases := []struct {
		name string
		info MediaInfo
		ext  string
		want bool
	}{
		{
			name: "h264 aac mp4 is compatible",
			info: MediaInfo{FormatName: "mov,mp4,m4a,3gp,3g2,mj2", VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"}},
			ext:  "mp4",
			want: false,
		},
		{
			name: "mpeg4 in avi needs transcode",
			info: MediaInfo{FormatName: "avi", VideoCodecs: []string{"mpeg4"}, AudioCodecs: []string{"mp3"}},
			ext:  "avi",
			want: true,
		},
		{
			name: "h264 in avi needs remux",
			info: MediaInfo{FormatName: "avi", VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"}},
			ext:  "avi",
			want: true,
		},
		{
			name: "hevc in mp4 needs transcode",
			info: MediaInfo{FormatName: "mov,mp4,m4a,3gp,3g2,mj2", VideoCodecs: []string{"hevc"}, AudioCodecs: []string{"aac"}},
			ext:  "mp4",
			want: true,
		},
		{
			name: "vp9 opus webm is compatible",
			info: MediaInfo{FormatName: "matroska,webm", VideoCodecs: []string{"vp9"}, AudioCodecs: []string{"opus"}},
			ext:  "webm",
			want: false,
		},
		{
			name: "h264 in mkv is conservative transcode",
			info: MediaInfo{FormatName: "matroska,webm", VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"}},
			ext:  "mkv",
			want: true,
		},
		{
			name: "pcm audio in mov needs transcode",
			info: MediaInfo{FormatName: "mov,mp4,m4a,3gp,3g2,mj2", VideoCodecs: []string{"h264"}, AudioCodecs: []string{"pcm_s16le"}},
			ext:  "mov",
			want: true,
		},
		{
			name: "video only h264 mp4 is compatible",
			info: MediaInfo{FormatName: "mov,mp4,m4a,3gp,3g2,mj2", VideoCodecs: []string{"h264"}},
			ext:  "mp4",
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NeedsTranscode(tc.info, tc.ext); got != tc.want {
				t.Fatalf("NeedsTranscode(%+v, %q) = %v, want %v", tc.info, tc.ext, got, tc.want)
			}
		})
	}
}

func TestBuildFFmpegArgsRemuxWhenCodecsCompatible(t *testing.T) {
	// AVI 里装 H.264+AAC：只需要换容器，应该走流拷贝
	info := MediaInfo{FormatName: "avi", VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"}}
	args := strings.Join(buildFFmpegArgs(info, "in.avi", "out.mp4"), " ")
	if !strings.Contains(args, "-c:v copy") {
		t.Fatalf("expected video stream copy, got: %s", args)
	}
	if !strings.Contains(args, "-c:a copy") {
		t.Fatalf("expected audio stream copy, got: %s", args)
	}
	if !strings.Contains(args, "+faststart") {
		t.Fatalf("expected faststart flag, got: %s", args)
	}
}

func TestBuildFFmpegArgsTranscodesIncompatibleCodecs(t *testing.T) {
	info := MediaInfo{FormatName: "avi", VideoCodecs: []string{"mpeg4"}, AudioCodecs: []string{"wmav2"}}
	args := strings.Join(buildFFmpegArgs(info, "in.avi", "out.mp4"), " ")
	if !strings.Contains(args, "-c:v libx264") {
		t.Fatalf("expected libx264 video encode, got: %s", args)
	}
	if !strings.Contains(args, "-c:a aac") {
		t.Fatalf("expected aac audio encode, got: %s", args)
	}
	if !strings.Contains(args, "yuv420p") {
		t.Fatalf("expected yuv420p pixel format, got: %s", args)
	}
}

func TestBuildFFmpegArgsDropsAudioWhenNoAudioStream(t *testing.T) {
	info := MediaInfo{FormatName: "avi", VideoCodecs: []string{"mpeg4"}}
	args := strings.Join(buildFFmpegArgs(info, "in.avi", "out.mp4"), " ")
	if !strings.Contains(args, "-an") {
		t.Fatalf("expected -an for video without audio, got: %s", args)
	}
}

func TestTranscodedName(t *testing.T) {
	for _, tc := range []struct {
		fileName, title, id, want string
	}{
		{"www.98T.la@167.avi", "www.98T.la@167", "p115-1", "www.98T.la@167.mp4"},
		{"", "标题", "p115-2", "标题.mp4"},
		{"a/b\\c.wmv", "", "p115-3", "a_b_c.mp4"},
	} {
		v := &catalog.Video{FileName: tc.fileName, Title: tc.title, ID: tc.id}
		if got := transcodedName(v); got != tc.want {
			t.Fatalf("transcodedName(%q,%q,%q) = %q, want %q", tc.fileName, tc.title, tc.id, got, tc.want)
		}
	}
}
