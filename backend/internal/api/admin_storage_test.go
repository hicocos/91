package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/video-site/backend/internal/catalog"
	"github.com/video-site/backend/internal/storageproviders"
)

func TestStorageOAuthRedirectURIUsesFixedOrigin(t *testing.T) {
	server := &AdminServer{PublicOrigin: "https://91s.lolicc.cc"}
	got, err := server.storageOAuthRedirectURI("googledrive")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://91s.lolicc.cc/admin/api/storage/oauth/googledrive/callback" {
		t.Fatalf("got=%q", got)
	}
	server.PublicOrigin = "http://evil.example"
	if _, err := server.storageOAuthRedirectURI("onedrive"); err == nil {
		t.Fatal("insecure public origin accepted")
	}
}

func TestOAuthFlowsConcurrentInitializationIsRaceFree(t *testing.T) {
	server := &AdminServer{}
	const workers = 32
	results := make(chan *storageproviders.OAuthFlows, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			flows, err := server.oauthFlows()
			results <- flows
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first *storageproviders.OAuthFlows
	for flows := range results {
		if first == nil {
			first = flows
		}
		if flows != first {
			t.Fatal("concurrent initialization returned different stores")
		}
	}
}

func TestLegacyDriveUpsertUsesStorageProbeBeforeSave(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	for _, kind := range []string{"onedrive", "googledrive", "webdav", "s3"} {
		id := "target-" + kind
		probed := ""
		server := &AdminServer{Catalog: cat, ProbeStorageAccount: func(_ context.Context, d *catalog.Drive) error { probed = d.Kind; return nil }}
		req := httptest.NewRequest(http.MethodPost, "/admin/api/drives", strings.NewReader(`{"id":"`+id+`","kind":"`+kind+`","name":"target","credentials":{"refresh_token":"tested"}}`))
		rr := httptest.NewRecorder()
		server.handleUpsertDrive(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s compatibility upsert status=%d body=%s", kind, rr.Code, rr.Body.String())
		}
		if probed != kind {
			t.Fatalf("%s was saved without provider probe", kind)
		}
		if _, err := cat.GetDrive(ctx, id); err != nil {
			t.Fatalf("%s compatibility upsert did not persist: %v", kind, err)
		}
	}
}

func TestStorageAccountSaveProbesBeforePersistAndFailedEditPreservesRow(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	if err := cat.UpsertDrive(ctx, &catalog.Drive{ID: "s3-main", Kind: "s3", Name: "old", RootID: "", Credentials: map[string]string{"bucket": "old", "secret_access_key": "secret"}}); err != nil {
		t.Fatal(err)
	}
	server := &AdminServer{Catalog: cat, ProbeStorageAccount: func(context.Context, *catalog.Drive) error { return errors.New("unreachable") }}
	req := storageAccountRequest(http.MethodPut, "/admin/api/storage/accounts/s3-main", "s3-main", `{"kind":"s3","name":"new","rootId":"","credentials":{"bucket":"new"}}`)
	rr := httptest.NewRecorder()
	server.handleSaveStorageAccount(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	got, _ := cat.GetDrive(ctx, "s3-main")
	if got.Name != "old" || got.Credentials["bucket"] != "old" || got.Credentials["secret_access_key"] != "secret" {
		t.Fatalf("failed edit polluted row: %#v", got)
	}
}

func TestStorageAccountSaveCanExplicitlyClearDeclaredCredentials(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	if err := cat.UpsertDrive(ctx, &catalog.Drive{ID: "dav-main", Kind: "webdav", Name: "old", RootID: "/", Credentials: map[string]string{"base_url": "https://dav.example.com", "username": "old-user", "password": "old-password"}}); err != nil {
		t.Fatal(err)
	}
	var probed *catalog.Drive
	server := &AdminServer{Catalog: cat, ProbeStorageAccount: func(_ context.Context, d *catalog.Drive) error { probed = d; return nil }}
	req := storageAccountRequest(http.MethodPut, "/admin/api/storage/accounts/dav-main", "dav-main", `{"kind":"webdav","name":"anonymous","rootId":"/","credentials":{"base_url":"https://dav.example.com"},"clearCredentials":["username","password"]}`)
	rr := httptest.NewRecorder()
	server.handleSaveStorageAccount(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("save=%d %s", rr.Code, rr.Body.String())
	}
	if _, ok := probed.Credentials["username"]; ok {
		t.Fatalf("probe retained cleared username: %#v", probed.Credentials)
	}
	if _, ok := probed.Credentials["password"]; ok {
		t.Fatalf("probe retained cleared password: %#v", probed.Credentials)
	}
	got, err := cat.GetDrive(ctx, "dav-main")
	if err != nil {
		t.Fatal(err)
	}
	if got.Credentials["username"] != "" || got.Credentials["password"] != "" {
		t.Fatalf("saved row retained cleared credentials: %#v", got.Credentials)
	}
}

func TestStorageAccountSaveMergesBlankSensitiveAndReturnsRedactedEdit(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	if err := cat.UpsertDrive(ctx, &catalog.Drive{ID: "s3-main", Kind: "s3", Name: "old", Credentials: map[string]string{"bucket": "old", "secret_access_key": "secret", "access_key_id": "ak"}}); err != nil {
		t.Fatal(err)
	}
	var probed *catalog.Drive
	server := &AdminServer{Catalog: cat, ProbeStorageAccount: func(_ context.Context, d *catalog.Drive) error { probed = d; return nil }}
	req := storageAccountRequest(http.MethodPut, "/admin/api/storage/accounts/s3-main", "s3-main", `{"kind":"s3","name":"new","credentials":{"bucket":"new","secret_access_key":""}}`)
	rr := httptest.NewRecorder()
	server.handleSaveStorageAccount(rr, req)
	if rr.Code != 200 {
		t.Fatalf("save=%d %s", rr.Code, rr.Body.String())
	}
	if probed.Credentials["secret_access_key"] != "secret" {
		t.Fatalf("probe lacked retained secret: %#v", probed.Credentials)
	}
	get := storageAccountRequest(http.MethodGet, "/admin/api/storage/accounts/s3-main", "s3-main", "")
	grr := httptest.NewRecorder()
	server.handleGetStorageAccount(grr, get)
	if grr.Code != 200 {
		t.Fatal(grr.Body.String())
	}
	var body struct {
		Credentials map[string]string `json:"credentials"`
		Configured  map[string]bool   `json:"configured"`
	}
	if err := json.Unmarshal(grr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Credentials["secret_access_key"] != "" || !body.Configured["secret_access_key"] {
		t.Fatalf("secret leaked/not marked: %s", grr.Body.String())
	}
	if body.Credentials["bucket"] != "new" {
		t.Fatalf("non-secret hidden: %s", grr.Body.String())
	}
}

func TestStorageAccountAttachFailureDoesNotRestoreConcurrentlyDeletedAccount(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	old := &catalog.Drive{ID: "s3-race", Kind: "s3", Name: "working", Credentials: map[string]string{"bucket": "old", "access_key_id": "ak", "secret_access_key": "secret"}}
	if err := cat.UpsertDrive(ctx, old); err != nil {
		t.Fatal(err)
	}
	server := &AdminServer{
		Catalog:             cat,
		ProbeStorageAccount: func(context.Context, *catalog.Drive) error { return nil },
		OnDriveSaved: func(id string) error {
			if err := cat.DeleteDrive(ctx, id); err != nil {
				t.Fatal(err)
			}
			return errors.New("attach boom after delete")
		},
	}
	rr := httptest.NewRecorder()
	server.handleSaveStorageAccount(rr, storageAccountRequest(http.MethodPut, "/admin/api/storage/accounts/s3-race", "s3-race", `{"kind":"s3","name":"broken","credentials":{"bucket":"new"}}`))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got, err := cat.GetDrive(ctx, "s3-race"); err == nil {
		t.Fatalf("attach rollback recreated deleted account: %#v", got)
	}
}

func TestStorageAccountAttachFailureRollsBackExistingAndNewRows(t *testing.T) {
	ctx := context.Background()
	cat, err := catalog.Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	old := &catalog.Drive{ID: "s3-old", Kind: "s3", Name: "working", Credentials: map[string]string{"bucket": "old", "access_key_id": "ak", "secret_access_key": "secret"}}
	if err := cat.UpsertDrive(ctx, old); err != nil {
		t.Fatal(err)
	}
	server := &AdminServer{
		Catalog:             cat,
		ProbeStorageAccount: func(context.Context, *catalog.Drive) error { return nil },
		OnDriveSaved:        func(string) error { return errors.New("attach boom") },
	}
	for _, tc := range []struct {
		id, body string
		existed  bool
	}{
		{"s3-old", `{"kind":"s3","name":"broken","credentials":{"bucket":"new"}}`, true},
		{"s3-new", `{"kind":"s3","name":"new","credentials":{"bucket":"new","access_key_id":"ak","secret_access_key":"secret"}}`, false},
	} {
		rr := httptest.NewRecorder()
		server.handleSaveStorageAccount(rr, storageAccountRequest(http.MethodPut, "/admin/api/storage/accounts/"+tc.id, tc.id, tc.body))
		if rr.Code != http.StatusBadGateway {
			t.Fatalf("%s status=%d body=%s", tc.id, rr.Code, rr.Body.String())
		}
		got, getErr := cat.GetDrive(ctx, tc.id)
		if tc.existed {
			if getErr != nil || got.Name != "working" || got.Credentials["bucket"] != "old" {
				t.Fatalf("existing rollback: got=%#v err=%v", got, getErr)
			}
		} else if getErr == nil {
			t.Fatalf("failed new account remained: %#v", got)
		}
	}
}

func storageAccountRequest(method, url, id, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, url, nil)
	} else {
		req = httptest.NewRequest(method, url, strings.NewReader(body))
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
