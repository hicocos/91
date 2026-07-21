package catalog

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/video-site/backend/internal/storageproviders"
)

func TestOAuthFlowTakeIsAtomicAcrossCatalogConnections(t *testing.T) {
	path := t.TempDir() + "/catalog.db"
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	record := storageproviders.OAuthFlowRecord{StateHash: "state", SessionHash: "session", Provider: "onedrive", RedirectURI: "https://app/callback", Nonce: "nonce", Pending: []byte{1}, ExpiresAt: time.Now().Add(time.Minute)}
	if err := first.PutOAuthFlow(record); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan bool, 2)
	for _, cat := range []*Catalog{first, second} {
		go func(c *Catalog) {
			<-start
			_, ok, err := c.TakeOAuthFlow("state", "session", "onedrive", "https://app/callback")
			results <- ok && err == nil
		}(cat)
	}
	close(start)
	if a, b := <-results, <-results; a == b {
		t.Fatalf("take winners=%v,%v want exactly one", a, b)
	}
}

func TestCatalogPersistsOAuthFlowRecords(t *testing.T) {
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	record := storageproviders.OAuthFlowRecord{StateHash: "state", SessionHash: "session", Provider: "onedrive", RedirectURI: "https://app/callback", Nonce: "nonce", Pending: []byte{1, 2, 3}, ExpiresAt: time.Unix(3000, 0)}
	if err := cat.PutOAuthFlow(record); err != nil {
		t.Fatal(err)
	}
	got, ok, err := cat.TakeOAuthFlow("state", "session", "onedrive", "https://app/callback")
	if err != nil || !ok {
		t.Fatalf("get ok=%v err=%v", ok, err)
	}
	if got.Provider != record.Provider || string(got.Pending) != string(record.Pending) || !got.ExpiresAt.Equal(record.ExpiresAt) {
		t.Fatalf("got=%#v", got)
	}
	if _, ok, err := cat.TakeOAuthFlow("state", "session", "onedrive", "https://app/callback"); err != nil || ok {
		t.Fatalf("after delete ok=%v err=%v", ok, err)
	}
}

func TestDeleteDriveSharesCredentialMergeLock(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	if err := cat.UpsertDrive(ctx, &Drive{ID: "g", Kind: "googledrive", Name: "g", Credentials: map[string]string{"refresh_token": "old"}}); err != nil {
		t.Fatal(err)
	}
	if err := cat.DeleteDrive(ctx, "g"); err != nil {
		t.Fatal(err)
	}
	if err := cat.MergeDriveCredentials(ctx, "g", map[string]string{"refresh_token": "late"}); err == nil {
		t.Fatal("late token merge recreated deleted drive")
	}
	if _, err := cat.GetDrive(ctx, "g"); err == nil {
		t.Fatal("deleted drive exists")
	}
}

func TestMergeDriveCredentialsPreservesConcurrentConfiguration(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	if err := cat.UpsertDrive(ctx, &Drive{ID: "g", Kind: "googledrive", Name: "new name", RootID: "new-root", Credentials: map[string]string{"client_secret": "new-secret", "refresh_token": "old-token"}, TeaserEnabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := cat.MergeDriveCredentials(ctx, "g", map[string]string{"access_token": "fresh-access", "refresh_token": "fresh-refresh"}); err != nil {
		t.Fatal(err)
	}
	got, err := cat.GetDrive(ctx, "g")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "new name" || got.RootID != "new-root" || got.Credentials["client_secret"] != "new-secret" || got.Credentials["access_token"] != "fresh-access" || got.Credentials["refresh_token"] != "fresh-refresh" {
		t.Fatalf("merged drive = %#v", got)
	}
}

func TestUpsertDriveUsesRootIDAsScanRootID(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() {
		if err := cat.Close(); err != nil {
			t.Fatalf("close catalog: %v", err)
		}
	})

	if err := cat.UpsertDrive(ctx, &Drive{
		ID:         "drive",
		Kind:       "p115",
		Name:       "115",
		RootID:     "root-folder",
		ScanRootID: "ignored-scan-root",
	}); err != nil {
		t.Fatalf("upsert drive: %v", err)
	}

	got, err := cat.GetDrive(ctx, "drive")
	if err != nil {
		t.Fatalf("get drive: %v", err)
	}
	if got.RootID != "root-folder" {
		t.Fatalf("rootId = %q, want root-folder", got.RootID)
	}
	if got.ScanRootID != "root-folder" {
		t.Fatalf("scanRootId = %q, want root-folder", got.ScanRootID)
	}
}

func TestUpsertDriveDefaultsRootIDByKind(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() {
		if err := cat.Close(); err != nil {
			t.Fatalf("close catalog: %v", err)
		}
	})

	cases := []struct {
		id   string
		kind string
		want string
	}{
		{id: "p115", kind: "p115", want: "0"},
		{id: "pikpak", kind: "pikpak", want: ""},
		{id: "guangyapan", kind: "guangyapan", want: ""},
		{id: "onedrive", kind: "onedrive", want: "root"},
		{id: "googledrive", kind: "googledrive", want: "root"},
		{id: "webdav", kind: "webdav", want: "/"},
		{id: "localstorage", kind: "localstorage", want: "/"},
		{id: "scriptcrawler", kind: "scriptcrawler", want: "/"},
	}
	for _, tc := range cases {
		if err := cat.UpsertDrive(ctx, &Drive{
			ID:   tc.id,
			Kind: tc.kind,
			Name: tc.kind,
		}); err != nil {
			t.Fatalf("upsert %s: %v", tc.kind, err)
		}
		got, err := cat.GetDrive(ctx, tc.id)
		if err != nil {
			t.Fatalf("get %s: %v", tc.kind, err)
		}
		if got.RootID != tc.want {
			t.Fatalf("%s rootId = %q, want %q", tc.kind, got.RootID, tc.want)
		}
		if got.ScanRootID != tc.want {
			t.Fatalf("%s scanRootId = %q, want %q", tc.kind, got.ScanRootID, tc.want)
		}
	}
}

func TestUpsertDriveIgnoresRootIDForLocalStorageAndScriptCrawler(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() {
		if err := cat.Close(); err != nil {
			t.Fatalf("close catalog: %v", err)
		}
	})

	for _, tc := range []struct {
		id   string
		kind string
	}{
		{id: "localstorage", kind: "localstorage"},
		{id: "scriptcrawler", kind: "scriptcrawler"},
	} {
		if err := cat.UpsertDrive(ctx, &Drive{
			ID:         tc.id,
			Kind:       tc.kind,
			Name:       tc.kind,
			RootID:     "manual-root",
			ScanRootID: "manual-scan-root",
		}); err != nil {
			t.Fatalf("upsert %s: %v", tc.kind, err)
		}
		got, err := cat.GetDrive(ctx, tc.id)
		if err != nil {
			t.Fatalf("get %s: %v", tc.kind, err)
		}
		if got.RootID != "/" {
			t.Fatalf("%s rootId = %q, want /", tc.kind, got.RootID)
		}
		if got.ScanRootID != "/" {
			t.Fatalf("%s scanRootId = %q, want /", tc.kind, got.ScanRootID)
		}
	}
}

func TestSetDriveRuntimeStatusTracksPlaybackFailureAndRecovery(t *testing.T) {
	ctx := context.Background()
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })

	drive := &Drive{
		ID:     "drive",
		Kind:   "p115",
		Name:   "115",
		Status: "ok",
		Credentials: map[string]string{
			"cookie": "credential-must-be-preserved",
		},
	}
	if err := cat.UpsertDrive(ctx, drive); err != nil {
		t.Fatalf("upsert drive: %v", err)
	}
	if err := cat.SetDriveRuntimeStatus(ctx, drive.ID, "error", "user not login"); err != nil {
		t.Fatalf("set error status: %v", err)
	}

	got, err := cat.GetDrive(ctx, drive.ID)
	if err != nil {
		t.Fatalf("get failed drive: %v", err)
	}
	if got.Status != "error" || !strings.Contains(got.LastError, "not login") {
		t.Fatalf("status=%q lastError=%q, want playback error", got.Status, got.LastError)
	}
	if got.Credentials["cookie"] != "credential-must-be-preserved" {
		t.Fatalf("credentials changed: %#v", got.Credentials)
	}

	if err := cat.SetDriveRuntimeStatus(ctx, drive.ID, "ok", ""); err != nil {
		t.Fatalf("recover status: %v", err)
	}
	got, err = cat.GetDrive(ctx, drive.ID)
	if err != nil {
		t.Fatalf("get recovered drive: %v", err)
	}
	if got.Status != "ok" || got.LastError != "" {
		t.Fatalf("status=%q lastError=%q, want recovered drive", got.Status, got.LastError)
	}
}

func TestSetDriveRuntimeStatusRejectsMissingDrive(t *testing.T) {
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })
	if err := cat.SetDriveRuntimeStatus(context.Background(), "missing", "ok", ""); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing drive status error = %v, want sql.ErrNoRows", err)
	}
}

func TestSetDriveRuntimeStatusRejectsInvalidState(t *testing.T) {
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })

	if err := cat.SetDriveRuntimeStatus(context.Background(), "drive", "pending", ""); err == nil {
		t.Fatal("invalid runtime status unexpectedly accepted")
	}
}
