package guangyapan

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/video-site/backend/internal/drives"
)

func TestDriverRefreshListAndStream(t *testing.T) {
	var refreshed bool
	var listedRoot bool
	updates := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/token":
			refreshed = true
			writeTestJSON(w, map[string]any{
				"access_token":  "new-access",
				"refresh_token": "new-refresh",
			})
		case "/v1/user/me":
			if got := r.Header.Get("Authorization"); got != "Bearer new-access" {
				t.Fatalf("auth header = %q, want new access token", got)
			}
			writeTestJSON(w, map[string]any{"sub": "user-1"})
		case "/userres/v1/file/get_file_list":
			if got := r.Header.Get("Authorization"); got != "Bearer new-access" {
				t.Fatalf("api auth header = %q, want new access token", got)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode list body: %v", err)
			}
			if body["parentId"] != "" {
				t.Fatalf("parentId = %#v, want root empty string", body["parentId"])
			}
			listedRoot = true
			writeTestJSON(w, map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"total": 2,
					"list": []map[string]any{
						{"fileId": "dir-1", "parentId": "", "fileName": "Movies", "resType": 2},
						{"fileId": "file-1", "parentId": "", "fileName": "clip.mp4", "fileSize": 123, "gcid": "0123456789abcdef0123456789abcdef01234567", "resType": 1, "utime": 1700000000},
					},
				},
			})
		case "/nd.bizuserres.s/v1/get_res_download_url":
			writeTestJSON(w, map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{"signedURL": "https://cdn.example.test/clip.mp4"},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	d := New(Config{
		ID:             "gy",
		RefreshToken:   "old-refresh",
		AccountBaseURL: srv.URL,
		APIBaseURL:     srv.URL,
		OnCredentialsUpdate: func(values map[string]string) {
			for k, v := range values {
				updates[k] = v
			}
		},
	})
	if err := d.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !refreshed {
		t.Fatal("refresh token endpoint was not called")
	}
	if updates["access_token"] != "new-access" || updates["refresh_token"] != "new-refresh" {
		t.Fatalf("updates = %#v, want refreshed tokens", updates)
	}

	entries, err := d.List(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !listedRoot || len(entries) != 2 {
		t.Fatalf("listedRoot=%v entries=%#v", listedRoot, entries)
	}
	if !entries[0].IsDir || entries[1].ID != "file-1" || entries[1].Size != 123 || entries[1].Hash != "0123456789ABCDEF0123456789ABCDEF01234567" {
		t.Fatalf("entries = %#v", entries)
	}

	link, err := d.StreamURL(context.Background(), "file-1")
	if err != nil {
		t.Fatalf("stream url: %v", err)
	}
	if link.URL != "https://cdn.example.test/clip.mp4" {
		t.Fatalf("stream url = %q", link.URL)
	}
}

func TestDriverResolvesRootPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/user/me":
			writeTestJSON(w, map[string]any{"sub": "user-1"})
		case "/userres/v1/file/get_file_list":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode list body: %v", err)
			}
			parent, _ := body["parentId"].(string)
			switch parent {
			case "":
				writeTestJSON(w, listTestResponse([]map[string]any{
					{"fileId": "folder-a", "parentId": "", "fileName": "影视", "resType": 2},
				}))
			case "folder-a":
				writeTestJSON(w, listTestResponse([]map[string]any{
					{"fileId": "folder-b", "parentId": "folder-a", "fileName": "电影", "resType": 2},
				}))
			case "folder-b":
				writeTestJSON(w, listTestResponse([]map[string]any{
					{"fileId": "file-1", "parentId": "folder-b", "fileName": "movie.mp4", "fileSize": 456, "resType": 1},
				}))
			default:
				t.Fatalf("unexpected parent %q", parent)
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	d := New(Config{
		ID:             "gy",
		RootID:         "configured-root",
		RootPath:       "影视/电影",
		AccessToken:    "access",
		AccountBaseURL: srv.URL,
		APIBaseURL:     srv.URL,
	})
	if err := d.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if d.RootID() != "folder-b" {
		t.Fatalf("root id = %q, want folder-b", d.RootID())
	}
	entries, err := d.List(context.Background(), "")
	if err != nil {
		t.Fatalf("list resolved root: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "file-1" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestDriverFetchesSubtitlesByRequestGCID(t *testing.T) {
	const gcid = "0123456789abcdef0123456789abcdef01234567"

	var queriedSubtitles bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/misc/v1/get_subtitles":
			if got := r.Header.Get("Authorization"); got != "Bearer access" {
				t.Fatalf("subtitle auth header = %q, want bearer token", got)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode subtitle body: %v", err)
			}
			if body["gcid"] != strings.ToUpper(gcid) {
				t.Fatalf("subtitle gcid = %#v, want normalized uppercase", body["gcid"])
			}
			if body["name"] != "movie.mp4" {
				t.Fatalf("subtitle name = %#v, want movie.mp4", body["name"])
			}
			if body["duration"] != float64(257) {
				t.Fatalf("subtitle duration = %#v, want 257", body["duration"])
			}
			queriedSubtitles = true
			writeTestJSON(w, map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"list": []map[string]any{
						{
							"gcid":      gcid,
							"source":    1,
							"name":      "简体中文",
							"duration":  257000,
							"languages": []string{"zh-CN"},
							"url":       "https://sub.example.test/movie.srt?token=abc",
						},
						{
							"cid":       "cid-2",
							"source":    2,
							"name":      "English",
							"ext":       "vtt",
							"duration":  257,
							"languages": []string{"en"},
							"url":       "https://sub.example.test/movie.vtt",
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	d := New(Config{
		ID:             "gy",
		AccessToken:    "access",
		AccountBaseURL: srv.URL,
		APIBaseURL:     srv.URL,
	})
	subs, err := d.Subtitles(context.Background(), drives.SubtitleRequest{
		FileID:          "file-1",
		FileName:        "movie.mp4",
		ContentHash:     gcid,
		DurationSeconds: 257,
	})
	if err != nil {
		t.Fatalf("subtitles: %v", err)
	}
	if !queriedSubtitles {
		t.Fatalf("queriedSubtitles=%v", queriedSubtitles)
	}
	if len(subs) != 2 {
		t.Fatalf("subtitles = %#v, want 2 items", subs)
	}
	if subs[0].Ext != "srt" || subs[0].Language != "zh-CN" || subs[0].SourceLabel != "inner" || subs[0].DurationSeconds != 257 {
		t.Fatalf("first subtitle = %#v, want inferred srt zh-CN inner 257s", subs[0])
	}
	if subs[1].Ext != "vtt" || subs[1].Language != "en" || subs[1].SourceLabel != "online" {
		t.Fatalf("second subtitle = %#v, want vtt en online", subs[1])
	}
}

func TestDriverDoesNotFallbackToDownloadURLForSubtitleGCID(t *testing.T) {
	var queriedDetail bool
	var requestedStream bool
	var requestedLegacyGCID bool
	var queriedSubtitles bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/userres/v1/file/get_file_detail":
			queriedDetail = true
			writeTestJSON(w, map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"fileInfo":      map[string]any{"fileId": "file-1", "fileName": "movie.mp4"},
					"videoResource": []map[string]any{},
				},
			})
		case "/nd.bizuserres.s/v1/get_res_download_url":
			requestedStream = true
			writeTestJSON(w, map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{"signedURL": "https://cdn.example.test/movie.mp4?token=abc"},
			})
		case "/file_info/cli_api/hfe_query_gcid":
			requestedLegacyGCID = true
			writeTestJSON(w, map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{"gcid": "0123456789abcdef0123456789abcdef01234567"},
			})
		case "/misc/v1/get_subtitles":
			queriedSubtitles = true
			writeTestJSON(w, map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{"list": []map[string]any{}},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	d := New(Config{
		ID:             "gy",
		AccessToken:    "access",
		AccountBaseURL: srv.URL,
		APIBaseURL:     srv.URL,
	})
	_, err := d.Subtitles(context.Background(), drives.SubtitleRequest{
		FileID:          "file-1",
		FileName:        "movie.mp4",
		DurationSeconds: 257,
	})
	if err == nil || !strings.Contains(err.Error(), "gcid not found") {
		t.Fatalf("subtitles error = %v, want gcid not found", err)
	}
	if !queriedDetail {
		t.Fatalf("file detail was not queried")
	}
	if requestedStream || requestedLegacyGCID || queriedSubtitles {
		t.Fatalf("requestedStream=%v requestedLegacyGCID=%v queriedSubtitles=%v, want all false", requestedStream, requestedLegacyGCID, queriedSubtitles)
	}
}

func TestDriverFetchesSubtitlesFromFileDetailGCID(t *testing.T) {
	const gcid = "0123456789abcdef0123456789abcdef01234567"

	var queriedDetail bool
	var queriedSubtitles bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/userres/v1/file/get_file_detail":
			if got := r.Header.Get("Authorization"); got != "Bearer access" {
				t.Fatalf("detail auth header = %q, want bearer token", got)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode detail body: %v", err)
			}
			if body["fileId"] != "file-1" {
				t.Fatalf("detail fileId = %#v, want file-1", body["fileId"])
			}
			queriedDetail = true
			writeTestJSON(w, map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"fileInfo": map[string]any{
						"fileId":   "file-1",
						"fileName": "movie.mp4",
						"gcid":     gcid,
					},
					"videoResource": []map[string]any{
						{"gcid": "ffffffffffffffffffffffffffffffffffffffff"},
					},
				},
			})
		case "/misc/v1/get_subtitles":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode subtitle body: %v", err)
			}
			if body["gcid"] != strings.ToUpper(gcid) {
				t.Fatalf("subtitle gcid = %#v, want file detail gcid", body["gcid"])
			}
			queriedSubtitles = true
			writeTestJSON(w, map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"list": []map[string]any{
						{
							"gcid":      gcid,
							"source":    1,
							"name":      "简体中文",
							"ext":       "srt",
							"languages": []string{"zh-CN"},
							"url":       "https://sub.example.test/movie.srt",
						},
					},
				},
			})
		case "/nd.bizuserres.s/v1/get_res_download_url", "/file_info/cli_api/hfe_query_gcid":
			t.Fatalf("unexpected fallback path %s", r.URL.Path)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	d := New(Config{
		ID:             "gy",
		AccessToken:    "access",
		AccountBaseURL: srv.URL,
		APIBaseURL:     srv.URL,
	})
	subs, err := d.Subtitles(context.Background(), drives.SubtitleRequest{
		FileID:          "file-1",
		FileName:        "movie.mp4",
		DurationSeconds: 257,
	})
	if err != nil {
		t.Fatalf("subtitles: %v", err)
	}
	if !queriedDetail || !queriedSubtitles {
		t.Fatalf("queriedDetail=%v queriedSubtitles=%v", queriedDetail, queriedSubtitles)
	}
	if len(subs) != 1 || subs[0].Ext != "srt" || subs[0].Language != "zh-CN" {
		t.Fatalf("subtitles = %#v, want one srt subtitle", subs)
	}
}

func TestDriverSendSMSCodeUpdatesVerificationState(t *testing.T) {
	updates := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/shield/captcha/init":
			writeTestJSON(w, map[string]any{"captcha_token": "captcha-1"})
		case "/v1/auth/verification":
			writeTestJSON(w, map[string]any{"verification_id": "verify-1"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	d := New(Config{
		ID:             "gy",
		PhoneNumber:    "13800000000",
		SendCode:       true,
		AccountBaseURL: srv.URL,
		APIBaseURL:     srv.URL,
		OnCredentialsUpdate: func(values map[string]string) {
			for k, v := range values {
				updates[k] = v
			}
		},
	})
	err := d.Init(context.Background())
	if err == nil || !strings.Contains(err.Error(), "验证码已发送") {
		t.Fatalf("init err = %v, want verification prompt", err)
	}
	if updates["captcha_token"] != "captcha-1" || updates["verification_id"] != "verify-1" || updates["send_code"] != "false" {
		t.Fatalf("updates = %#v, want sms state saved", updates)
	}
	if updates["device_id"] == "" {
		t.Fatalf("updates = %#v, want generated device id saved", updates)
	}
}

func TestListHTTP429ReturnsRateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/userres/v1/file/get_file_list" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		writeTestJSON(w, map[string]any{"code": 429, "msg": "操作频繁，请稍后重试"})
	}))
	defer srv.Close()

	d := New(Config{
		ID:             "gy",
		AccessToken:    "access",
		AccountBaseURL: srv.URL,
		APIBaseURL:     srv.URL,
	})
	_, err := d.List(context.Background(), "")
	if err == nil {
		t.Fatal("list succeeded, want rate limit error")
	}
	var rateLimit *drives.RateLimitError
	if !errors.As(err, &rateLimit) {
		t.Fatalf("error = %T %[1]v, want RateLimitError", err)
	}
	if rateLimit.RetryAfter != 2*time.Minute {
		t.Fatalf("retry after = %s, want 2m", rateLimit.RetryAfter)
	}
}

func TestListCode429ReturnsRateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/userres/v1/file/get_file_list" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		writeTestJSON(w, map[string]any{"code": 429, "msg": "操作频繁，请稍后再试"})
	}))
	defer srv.Close()

	d := New(Config{
		ID:             "gy",
		AccessToken:    "access",
		AccountBaseURL: srv.URL,
		APIBaseURL:     srv.URL,
	})
	_, err := d.List(context.Background(), "")
	if err == nil {
		t.Fatal("list succeeded, want rate limit error")
	}
	var rateLimit *drives.RateLimitError
	if !errors.As(err, &rateLimit) {
		t.Fatalf("error = %T %[1]v, want RateLimitError", err)
	}
}

func TestListInvalidToken403DoesNotReturnRateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/userres/v1/file/get_file_list" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusForbidden)
		writeTestJSON(w, map[string]any{"code": 401, "msg": "invalid access token"})
	}))
	defer srv.Close()

	d := New(Config{
		ID:             "gy",
		AccessToken:    "access",
		AccountBaseURL: srv.URL,
		APIBaseURL:     srv.URL,
	})
	_, err := d.List(context.Background(), "")
	if err == nil {
		t.Fatal("list succeeded, want auth error")
	}
	var rateLimit *drives.RateLimitError
	if errors.As(err, &rateLimit) {
		t.Fatalf("error = %T %[1]v, want non-rate-limit error", err)
	}
}

func listTestResponse(items []map[string]any) map[string]any {
	return map[string]any{
		"code": 0,
		"msg":  "success",
		"data": map[string]any{
			"total": len(items),
			"list":  items,
		},
	}
}

func writeTestJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic(err)
	}
}
