package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/video-site/backend/internal/catalog"
)

func TestLoginBansIPAfterThreeFailuresPermanently(t *testing.T) {
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() {
		if err := cat.Close(); err != nil {
			t.Fatalf("close catalog: %v", err)
		}
	})

	now := time.Unix(1_700_000_000, 0)
	authr := &Authenticator{
		Username: "admin",
		Password: "secret",
		Catalog:  cat,
		Now:      func() time.Time { return now },
	}

	for i := 0; i < loginFailThreshold-1; i++ {
		ok, err := authr.Login(httptest.NewRecorder(), loginRequest("203.0.113.10"), "admin", "wrong")
		if err != nil {
			t.Fatalf("failure %d returned error: %v", i+1, err)
		}
		if ok {
			t.Fatalf("failure %d returned ok", i+1)
		}
	}

	ok, err := authr.Login(httptest.NewRecorder(), loginRequest("203.0.113.10"), "admin", "wrong")
	if ok {
		t.Fatal("third failed login returned ok")
	}
	if !errors.Is(err, ErrLoginIPBanned) {
		t.Fatalf("third failed login error = %v, want ErrLoginIPBanned", err)
	}

	banned, err := cat.IsLoginIPBanned(loginRequest("203.0.113.10").Context(), "203.0.113.10")
	if err != nil {
		t.Fatalf("query ban: %v", err)
	}
	if !banned {
		t.Fatal("ip was not persisted as banned")
	}

	now = now.Add(loginFailWindow * 2)
	reloaded := &Authenticator{Username: "admin", Password: "secret", Catalog: cat, Now: func() time.Time { return now }}
	ok, err = reloaded.Login(httptest.NewRecorder(), loginRequest("203.0.113.10"), "admin", "secret")
	if ok {
		t.Fatal("permanently banned ip logged in with correct credentials")
	}
	if !errors.Is(err, ErrLoginIPBanned) {
		t.Fatalf("banned ip error = %v, want ErrLoginIPBanned", err)
	}
}

func TestSuccessfulLoginClearsFailedLoginWindow(t *testing.T) {
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() {
		if err := cat.Close(); err != nil {
			t.Fatalf("close catalog: %v", err)
		}
	})

	authr := &Authenticator{
		Username: "admin",
		Password: "secret",
		Catalog:  cat,
	}

	for i := 0; i < loginFailThreshold-1; i++ {
		if ok, err := authr.Login(httptest.NewRecorder(), loginRequest("203.0.113.11"), "admin", "wrong"); err != nil || ok {
			t.Fatalf("failed login %d ok=%v err=%v", i+1, ok, err)
		}
	}
	if ok, err := authr.Login(httptest.NewRecorder(), loginRequest("203.0.113.11"), "admin", "secret"); err != nil || !ok {
		t.Fatalf("successful login after failures ok=%v err=%v", ok, err)
	}
	if ok, err := authr.Login(httptest.NewRecorder(), loginRequest("203.0.113.11"), "admin", "wrong"); err != nil || ok {
		t.Fatalf("failure after successful login ok=%v err=%v", ok, err)
	}
}

func TestRequiredRejectsBannedUserSession(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() {
		if err := cat.Close(); err != nil {
			t.Fatalf("close catalog: %v", err)
		}
	})
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := cat.CreateUser(ctx, "viewer", hash, "user")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	authr := &Authenticator{Catalog: cat}
	rr := httptest.NewRecorder()
	role, err := authr.UserLogin(rr, loginRequest("203.0.113.30"), "viewer", "secret123")
	if err != nil || role != "user" {
		t.Fatalf("login role=%q err=%v", role, err)
	}
	if err := cat.SetUserBanned(ctx, userID, true); err != nil {
		t.Fatalf("ban user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/home", nil)
	req.AddCookie(rr.Result().Cookies()[0])
	res := httptest.NewRecorder()
	authr.Required(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.Code)
	}
}

func TestRequiredRejectsDeletedUserSession(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() {
		if err := cat.Close(); err != nil {
			t.Fatalf("close catalog: %v", err)
		}
	})
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := cat.CreateUser(ctx, "viewer", hash, "user")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	authr := &Authenticator{Catalog: cat}
	rr := httptest.NewRecorder()
	if role, err := authr.UserLogin(rr, loginRequest("203.0.113.31"), "viewer", "secret123"); err != nil || role != "user" {
		t.Fatalf("login role=%q err=%v", role, err)
	}
	if err := cat.DeleteUser(ctx, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/home", nil)
	req.AddCookie(rr.Result().Cookies()[0])
	res := httptest.NewRecorder()
	authr.Required(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.Code)
	}
}

func TestUserLoginOnlyFallsBackToConfigWhenUsersTableIsEmpty(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() {
		if err := cat.Close(); err != nil {
			t.Fatalf("close catalog: %v", err)
		}
	})
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := cat.CreateUser(ctx, "viewer", hash, "user"); err != nil {
		t.Fatalf("create user: %v", err)
	}

	authr := &Authenticator{Username: "legacy-admin", Password: "legacy-secret", Catalog: cat}
	role, err := authr.UserLogin(httptest.NewRecorder(), loginRequest("203.0.113.32"), "legacy-admin", "legacy-secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if role != "" {
		t.Fatalf("role = %q, want failed login", role)
	}
}

func TestClientIPUsesForwardedHeadersFromTrustedProxy(t *testing.T) {
	req := loginRequest("127.0.0.1")
	req.Header.Set("X-Forwarded-For", "203.0.113.12")

	if got := clientIP(req); got != "203.0.113.12" {
		t.Fatalf("client IP = %q, want trusted forwarded origin", got)
	}
}

func TestClientIPNormalizesMappedIPv4FromTrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/admin/api/login", strings.NewReader(`{}`))
	req.RemoteAddr = "[::ffff:127.0.0.1]:12345"
	req.Header.Set("X-Forwarded-For", "::ffff:203.0.113.12")

	if got := clientIP(req); got != "203.0.113.12" {
		t.Fatalf("client IP = %q, want normalized forwarded IPv4", got)
	}
}

func TestClientIPUsesRightmostForwardedHeaderFromTrustedProxy(t *testing.T) {
	req := loginRequest("127.0.0.1")
	req.Header.Set("X-Forwarded-For", "198.51.100.99, 203.0.113.12")

	if got := clientIP(req); got != "203.0.113.12" {
		t.Fatalf("client IP = %q, want rightmost forwarded IP", got)
	}
}

func TestClientIPIgnoresForwardedHeadersFromUntrustedRemote(t *testing.T) {
	req := loginRequest("198.51.100.20")
	req.Header.Set("X-Forwarded-For", "203.0.113.12")
	req.Header.Set("X-Real-IP", "203.0.113.13")

	if got := clientIP(req); got != "198.51.100.20" {
		t.Fatalf("client IP = %q, want remote address", got)
	}
}

func loginRequest(ip string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/admin/api/login", strings.NewReader(`{}`))
	req.RemoteAddr = ip + ":12345"
	return req
}
