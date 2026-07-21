package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withCredentialsKeyFile(t *testing.T, path string) {
	t.Helper()
	previous, existed := os.LookupEnv("VIDEO_CREDENTIALS_KEY_FILE")
	if err := os.Setenv("VIDEO_CREDENTIALS_KEY_FILE", path); err != nil {
		t.Fatalf("set credentials key env: %v", err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("VIDEO_CREDENTIALS_KEY_FILE", previous)
		} else {
			_ = os.Unsetenv("VIDEO_CREDENTIALS_KEY_FILE")
		}
	})
}

func TestUpsertDriveEncryptsCredentialsAtRest(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cat, err := Open(filepath.Join(dir, "catalog.db"))
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })

	want := map[string]string{
		"cookie":        "secret-cookie-value",
		"refresh_token": "secret-refresh-token",
	}
	if err := cat.UpsertDrive(ctx, &Drive{
		ID:          "encrypted-drive",
		Kind:        "p115",
		Name:        "Encrypted Drive",
		Credentials: want,
	}); err != nil {
		t.Fatalf("upsert drive: %v", err)
	}

	var stored string
	if err := cat.db.QueryRowContext(ctx,
		`SELECT credentials FROM drives WHERE id = ?`, "encrypted-drive").Scan(&stored); err != nil {
		t.Fatalf("read stored credentials: %v", err)
	}
	if strings.Contains(stored, "secret-cookie-value") || strings.Contains(stored, "secret-refresh-token") {
		t.Fatalf("credentials stored in plaintext: %s", stored)
	}
	if !strings.Contains(stored, `"_credentials_envelope":"aes-256-gcm"`) {
		t.Fatalf("stored credentials are not an AES-GCM envelope: %s", stored)
	}

	got, err := cat.GetDrive(ctx, "encrypted-drive")
	if err != nil {
		t.Fatalf("get drive: %v", err)
	}
	if got.Credentials["cookie"] != want["cookie"] || got.Credentials["refresh_token"] != want["refresh_token"] {
		t.Fatalf("credentials = %#v, want %#v", got.Credentials, want)
	}

	keyPath := filepath.Join(dir, "credentials.key")
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read generated key: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key length = %d, want 32", len(key))
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat generated key: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("key mode = %04o, want 0600", got)
	}
}

func TestGetDriveLazilyMigratesLegacyPlaintextCredentials(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cat, err := Open(filepath.Join(dir, "catalog.db"))
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })

	legacy := `{"cookie":"legacy-secret"}`
	now := int64(1700000000000)
	if _, err := cat.db.ExecContext(ctx, `
INSERT INTO drives (id, kind, name, root_id, scan_root_id, credentials, status, teaser_enabled, skip_dir_ids, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"legacy-drive", "p115", "Legacy", "0", "0", legacy, "ok", 1, "[]", now, now); err != nil {
		t.Fatalf("seed legacy drive: %v", err)
	}

	got, err := cat.GetDrive(ctx, "legacy-drive")
	if err != nil {
		t.Fatalf("get legacy drive: %v", err)
	}
	if got.Credentials["cookie"] != "legacy-secret" {
		t.Fatalf("credentials = %#v, want legacy secret", got.Credentials)
	}

	var stored string
	if err := cat.db.QueryRowContext(ctx,
		`SELECT credentials FROM drives WHERE id = ?`, "legacy-drive").Scan(&stored); err != nil {
		t.Fatalf("read migrated credentials: %v", err)
	}
	if strings.Contains(stored, "legacy-secret") || !strings.Contains(stored, `"_credentials_envelope":"aes-256-gcm"`) {
		t.Fatalf("legacy credentials were not migrated to an envelope: %s", stored)
	}
}

func TestListDrivesLazilyMigratesAllLegacyPlaintextCredentials(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })

	for _, id := range []string{"legacy-one", "legacy-two"} {
		legacy := `{"token":"` + id + `-secret"}`
		if _, err := cat.db.ExecContext(ctx, `
INSERT INTO drives (id, kind, name, root_id, scan_root_id, credentials, status, teaser_enabled, skip_dir_ids, created_at, updated_at)
VALUES (?, 'p115', ?, '0', '0', ?, 'ok', 1, '[]', 1, 1)`, id, id, legacy); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	drives, err := cat.ListDrives(ctx)
	if err != nil {
		t.Fatalf("list drives: %v", err)
	}
	if len(drives) != 2 {
		t.Fatalf("listed %d drives, want 2", len(drives))
	}
	for _, drive := range drives {
		if drive.Credentials["token"] != drive.ID+"-secret" {
			t.Fatalf("%s credentials = %#v", drive.ID, drive.Credentials)
		}
		var stored string
		if err := cat.db.QueryRowContext(ctx, `SELECT credentials FROM drives WHERE id = ?`, drive.ID).Scan(&stored); err != nil {
			t.Fatalf("read %s stored credentials: %v", drive.ID, err)
		}
		if strings.Contains(stored, "-secret") {
			t.Fatalf("%s remained plaintext: %s", drive.ID, stored)
		}
	}
}

func TestOpenUsesExplicitCredentialsKeyFileAndReopensEncryptedData(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "catalog.db")
	keyPath := filepath.Join(dir, "keys", "video.key")
	if err := os.Mkdir(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatalf("mkdir key dir: %v", err)
	}
	withCredentialsKeyFile(t, keyPath)

	cat, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	if err := cat.UpsertDrive(ctx, &Drive{ID: "drive", Kind: "p115", Credentials: map[string]string{"token": "survives-reopen"}}); err != nil {
		t.Fatalf("upsert drive: %v", err)
	}
	if err := cat.Close(); err != nil {
		t.Fatalf("close catalog: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("explicit key was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials.key")); !os.IsNotExist(err) {
		t.Fatalf("default key unexpectedly exists: %v", err)
	}

	cat, err = Open(dbPath)
	if err != nil {
		t.Fatalf("reopen catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })
	got, err := cat.GetDrive(ctx, "drive")
	if err != nil {
		t.Fatalf("get drive after reopen: %v", err)
	}
	if got.Credentials["token"] != "survives-reopen" {
		t.Fatalf("credentials after reopen = %#v", got.Credentials)
	}
}

func TestOpenRejectsInvalidCredentialsKeyLength(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "bad.key")
	if err := os.WriteFile(keyPath, []byte("too-short"), 0o600); err != nil {
		t.Fatalf("write bad key: %v", err)
	}
	withCredentialsKeyFile(t, keyPath)

	if cat, err := Open(filepath.Join(dir, "catalog.db")); err == nil {
		_ = cat.Close()
		t.Fatal("catalog opened with an invalid credentials key")
	}
}

func TestGetDriveFailsClosedForTamperedEncryptedCredentials(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })
	if err := cat.UpsertDrive(ctx, &Drive{ID: "drive", Kind: "p115", Credentials: map[string]string{"token": "secret"}}); err != nil {
		t.Fatalf("upsert drive: %v", err)
	}
	if _, err := cat.db.ExecContext(ctx,
		`UPDATE drives SET credentials = json_set(credentials, '$.ciphertext', 'AAAA') WHERE id = 'drive'`); err != nil {
		t.Fatalf("tamper credentials: %v", err)
	}

	if got, err := cat.GetDrive(ctx, "drive"); err == nil {
		t.Fatalf("tampered credentials returned %#v instead of an error", got.Credentials)
	}
	if got, err := cat.ListDrives(ctx); err == nil {
		t.Fatalf("list returned %d drives despite tampered credentials", len(got))
	}
}
