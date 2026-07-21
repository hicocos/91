package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/video-site/backend/internal/auth"
)

const sourceDeleteConfirmationTTL = 2 * time.Minute

const deleteVideoSourceAction = "delete-video-source"

type destructiveConfirmation struct {
	SessionIdentity string
	Action          string
	Scope           string
	Snapshot        []string
	ExpiresAt       time.Time
}

func (a *AdminServer) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *AdminServer) handlePrepareDeleteVideoSource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeErr(w, http.StatusBadRequest, errors.New("invalid video id"))
		return
	}
	nonce, expiresAt, err := a.prepareDestructiveConfirmation(r, deleteVideoSourceAction, id)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"nonce":     nonce,
		"expiresAt": expiresAt.UTC().Format(time.RFC3339Nano),
	})
}

func (a *AdminServer) prepareDestructiveConfirmation(r *http.Request, action, scope string) (string, time.Time, error) {
	return a.prepareDestructiveConfirmationWithSnapshot(r, action, scope, nil)
}

func (a *AdminServer) prepareDestructiveConfirmationWithSnapshot(r *http.Request, action, scope string, snapshot []string) (string, time.Time, error) {
	sessionIdentity, ok := auth.SessionIdentityFromContext(r.Context())
	if !ok {
		return "", time.Time{}, errors.New("authenticated session identity is required")
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", time.Time{}, err
	}
	nonce := hex.EncodeToString(raw[:])
	expiresAt := a.now().Add(sourceDeleteConfirmationTTL)
	a.destructiveConfirmationMu.Lock()
	defer a.destructiveConfirmationMu.Unlock()
	if a.destructiveConfirmations == nil {
		a.destructiveConfirmations = make(map[string]destructiveConfirmation)
	}
	for key, confirmation := range a.destructiveConfirmations {
		if !a.now().Before(confirmation.ExpiresAt) {
			delete(a.destructiveConfirmations, key)
		}
	}
	a.destructiveConfirmations[nonce] = destructiveConfirmation{
		SessionIdentity: sessionIdentity,
		Action:          action,
		Scope:           scope,
		Snapshot:        append([]string(nil), snapshot...),
		ExpiresAt:       expiresAt,
	}
	return nonce, expiresAt, nil
}

func (a *AdminServer) consumeDestructiveConfirmation(r *http.Request, nonce, action, scope string) (destructiveConfirmation, bool) {
	sessionIdentity, ok := auth.SessionIdentityFromContext(r.Context())
	if !ok {
		return destructiveConfirmation{}, false
	}
	nonce = strings.TrimSpace(nonce)
	if nonce == "" {
		return destructiveConfirmation{}, false
	}
	a.destructiveConfirmationMu.Lock()
	defer a.destructiveConfirmationMu.Unlock()
	confirmation, ok := a.destructiveConfirmations[nonce]
	if !ok || !a.now().Before(confirmation.ExpiresAt) ||
		confirmation.SessionIdentity != sessionIdentity ||
		confirmation.Action != action || confirmation.Scope != scope {
		if ok && !a.now().Before(confirmation.ExpiresAt) {
			delete(a.destructiveConfirmations, nonce)
		}
		return destructiveConfirmation{}, false
	}
	delete(a.destructiveConfirmations, nonce)
	return confirmation, true
}
