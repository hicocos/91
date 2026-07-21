package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/video-site/backend/internal/catalog"
	"github.com/video-site/backend/internal/storageproviders"
)

func (a *AdminServer) providerRegistry() *storageproviders.Registry {
	if a.StorageProviders != nil {
		return a.StorageProviders
	}
	return storageproviders.DefaultRegistry()
}

func (a *AdminServer) handleStorageProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.providerRegistry().Manifests())
}

type probeAccountRequest struct {
	ID               string            `json:"id"`
	Kind             string            `json:"kind"`
	Name             string            `json:"name"`
	RootID           string            `json:"rootId"`
	Credentials      map[string]string `json:"credentials"`
	ClearCredentials []string          `json:"clearCredentials,omitempty"`
}

var targetStorageKinds = map[string]bool{"onedrive": true, "googledrive": true, "webdav": true, "s3": true}

func sensitiveProviderFields(registry *storageproviders.Registry, kind string) map[string]bool {
	out := map[string]bool{}
	if descriptor, ok := registry.Lookup(kind); ok {
		for _, field := range descriptor.Manifest.Fields {
			if field.Sensitive {
				out[field.Key] = true
			}
		}
	}
	// access tokens are runtime credentials even when omitted from the onboarding manifest.
	out["access_token"] = true
	return out
}

func (a *AdminServer) handleProbeStorageAccount(w http.ResponseWriter, r *http.Request) {
	var body probeAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if _, ok := a.providerRegistry().Lookup(body.Kind); !ok {
		http.Error(w, "unsupported drive kind", http.StatusBadRequest)
		return
	}
	var existing *catalog.Drive
	if body.ID != "" {
		existing, _ = a.Catalog.GetDrive(r.Context(), body.ID)
	}
	body.Credentials = mergeStorageCredentials(a.providerRegistry(), body.Kind, existing, body.Credentials, body.ClearCredentials)
	candidate := &catalog.Drive{ID: body.ID, Kind: body.Kind, Name: body.Name, RootID: body.RootID, Credentials: body.Credentials}
	if a.ProbeStorageAccount == nil {
		http.Error(w, "storage probe unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := a.ProbeStorageAccount(r.Context(), candidate); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	// Probe tokens are issued by deployments wiring a persistent session key. The
	// compatibility endpoint deliberately performs no write, so a failed probe can
	// never pollute the drive table.
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *AdminServer) handleSaveStorageAccount(w http.ResponseWriter, r *http.Request) {
	var body probeAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	a.saveStorageAccount(w, r, body)
}

func (a *AdminServer) saveStorageAccount(w http.ResponseWriter, r *http.Request, body probeAccountRequest) {
	pathID := strings.TrimSpace(chi.URLParam(r, "id"))
	if pathID != "" {
		body.ID = pathID
	}
	body.ID = strings.TrimSpace(body.ID)
	body.Kind = strings.TrimSpace(body.Kind)
	body.Name = strings.TrimSpace(body.Name)
	if body.ID == "" || body.Kind == "" || body.Name == "" || !targetStorageKinds[body.Kind] {
		http.Error(w, "id, kind and name for a supported storage provider are required", http.StatusBadRequest)
		return
	}
	var existing *catalog.Drive
	if found, err := a.Catalog.GetDrive(r.Context(), body.ID); err == nil {
		existing = found
		if found.Kind != body.Kind {
			http.Error(w, "provider kind cannot be changed", http.StatusBadRequest)
			return
		}
	}
	body.Credentials = mergeStorageCredentials(a.providerRegistry(), body.Kind, existing, body.Credentials, body.ClearCredentials)
	if body.Kind == "googledrive" {
		delete(body.Credentials, "use_online_api")
		delete(body.Credentials, "api_url_address")
	}
	if body.Kind == "s3" {
		delete(body.Credentials, "root_prefix")
	}
	candidate := &catalog.Drive{ID: body.ID, Kind: body.Kind, Name: body.Name, RootID: body.RootID, Credentials: body.Credentials, Status: "disconnected", TeaserEnabled: true}
	if existing != nil {
		candidate.TeaserEnabled = existing.TeaserEnabled
		candidate.SkipDirIDs = existing.SkipDirIDs
	}
	if a.ProbeStorageAccount == nil {
		http.Error(w, "storage probe unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := a.ProbeStorageAccount(r.Context(), candidate); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := a.Catalog.UpsertDrive(r.Context(), candidate); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if a.OnDriveSaved != nil {
		if err := a.OnDriveSaved(candidate.ID); err != nil {
			// The persisted configuration and live attachment form one operation from
			// the administrator's perspective. Restore the exact previous row (or
			// remove a failed new row) so a restart cannot discard a working account.
			if existing != nil {
				restored, rollbackErr := a.Catalog.ReplaceDriveIfCurrent(r.Context(), candidate, existing)
				if rollbackErr != nil {
					http.Error(w, "attach failed and rollback failed: "+err.Error()+"; "+rollbackErr.Error(), http.StatusInternalServerError)
					return
				}
				if !restored {
					if _, getErr := a.Catalog.GetDrive(r.Context(), candidate.ID); getErr != nil {
						http.Error(w, "storage attach failed after account was removed: "+err.Error(), http.StatusBadGateway)
						return
					}
					http.Error(w, "storage attach failed and account changed concurrently: "+err.Error(), http.StatusConflict)
					return
				}
			} else if rollbackErr := a.Catalog.DeleteDrive(r.Context(), candidate.ID); rollbackErr != nil {
				http.Error(w, "attach failed and rollback failed: "+err.Error()+"; "+rollbackErr.Error(), http.StatusInternalServerError)
				return
			}
			http.Error(w, "storage attach failed: "+err.Error(), http.StatusBadGateway)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func mergeStorageCredentials(registry *storageproviders.Registry, kind string, existing *catalog.Drive, incoming map[string]string, clear []string) map[string]string {
	merged := mergeNonEmptyCredentials(existing, incoming)
	if kind == "onedrive" {
		delete(merged, "use_online_api")
		delete(merged, "api_url_address")
	}
	descriptor, ok := registry.Lookup(kind)
	if !ok {
		return merged
	}
	allowed := make(map[string]bool, len(descriptor.Manifest.Fields))
	for _, field := range descriptor.Manifest.Fields {
		allowed[field.Key] = true
	}
	for _, rawKey := range clear {
		key := strings.TrimSpace(rawKey)
		if allowed[key] {
			delete(merged, key)
		}
	}
	return merged
}

func (a *AdminServer) handleGetStorageAccount(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	drive, err := a.Catalog.GetDrive(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	credentials := make(map[string]string, len(drive.Credentials))
	configured := map[string]bool{}
	sensitive := sensitiveProviderFields(a.providerRegistry(), drive.Kind)
	for key, value := range drive.Credentials {
		if sensitive[key] {
			credentials[key] = ""
			configured[key] = strings.TrimSpace(value) != ""
		} else {
			credentials[key] = value
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"id": drive.ID, "kind": drive.Kind, "name": drive.Name, "rootId": drive.RootID, "credentials": credentials, "configured": configured})
}
