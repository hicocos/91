package catalog

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSessionTokensAreHashedAtRestAndExpiredSessionsAreCleaned(t *testing.T) {
	path := t.TempDir() + "/catalog.db"
	cat, err := Open(path)
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	ctx := context.Background()
	const token = "raw-session-token"
	if err := cat.CreateSessionUntil(ctx, token, time.Now().Add(-time.Minute), 7); err != nil {
		t.Fatalf("create session: %v", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	defer db.Close()
	var stored string
	if err := db.QueryRow(`SELECT token FROM admin_sessions`).Scan(&stored); err != nil {
		t.Fatalf("read stored token: %v", err)
	}
	if stored == token {
		t.Fatal("raw session token was stored in SQLite")
	}
	if _, found, err := cat.GetSession(ctx, token); err != nil || !found {
		t.Fatalf("lookup by raw token found=%v err=%v", found, err)
	}
	removed, err := cat.DeleteExpiredSessions(ctx, time.Now())
	if err != nil || removed != 1 {
		t.Fatalf("cleanup removed=%d err=%v, want 1", removed, err)
	}
	if _, found, err := cat.GetSession(ctx, token); err != nil || found {
		t.Fatalf("expired session remained found=%v err=%v", found, err)
	}
	if err := cat.Close(); err != nil {
		t.Fatalf("close catalog: %v", err)
	}
}

func TestCatalogPingFailsAfterClose(t *testing.T) {
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	if err := cat.Ping(context.Background()); err != nil {
		t.Fatalf("ping open catalog: %v", err)
	}
	if err := cat.Close(); err != nil {
		t.Fatalf("close catalog: %v", err)
	}
	if err := cat.Ping(context.Background()); err == nil {
		t.Fatal("ping closed catalog succeeded")
	}
}
