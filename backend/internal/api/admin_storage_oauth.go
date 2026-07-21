package api

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/video-site/backend/internal/auth"
	"github.com/video-site/backend/internal/storageproviders"
)

var oauthInitMu sync.Mutex

type storageOAuthStartRequest struct {
	ID          string            `json:"id"`
	Credentials map[string]string `json:"credentials"`
}

func (a *AdminServer) oauthFlows() (*storageproviders.OAuthFlows, error) {
	oauthInitMu.Lock()
	defer oauthInitMu.Unlock()
	if a.OAuthFlows != nil {
		return a.OAuthFlows, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	flows, err := storageproviders.NewOAuthFlows(key, nil)
	if err != nil {
		return nil, err
	}
	a.OAuthFlows = flows
	return flows, nil
}

func (a *AdminServer) storageOAuthRedirectURI(provider string) (string, error) {
	origin := strings.TrimRight(strings.TrimSpace(a.PublicOrigin), "/")
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("fixed HTTPS public origin is required for OAuth")
	}
	return origin + "/admin/api/storage/oauth/" + url.PathEscape(provider) + "/callback", nil
}

func (a *AdminServer) handleStorageOAuthStart(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	if provider != "onedrive" && provider != "googledrive" {
		http.Error(w, "unsupported oauth provider", http.StatusBadRequest)
		return
	}
	session, ok := auth.SessionIdentityFromContext(r.Context())
	if !ok {
		http.Error(w, "missing authenticated session", http.StatusUnauthorized)
		return
	}
	var body storageOAuthStartRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if id := strings.TrimSpace(body.ID); id != "" {
		if existing, err := a.Catalog.GetDrive(r.Context(), id); err == nil {
			if existing.Kind != provider {
				http.Error(w, "oauth provider does not match account", http.StatusBadRequest)
				return
			}
			body.Credentials = mergeNonEmptyCredentials(existing, body.Credentials)
		}
	}
	clientID := strings.TrimSpace(body.Credentials["client_id"])
	if clientID == "" {
		http.Error(w, "client_id is required", http.StatusBadRequest)
		return
	}
	redirectURI, err := a.storageOAuthRedirectURI(provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	flows, err := a.oauthFlows()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	state, nonce, err := flows.Start(session, provider, redirectURI, body.Credentials)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	q := url.Values{"client_id": {clientID}, "redirect_uri": {redirectURI}, "response_type": {"code"}, "state": {state}}
	var authURL string
	if provider == "onedrive" {
		tenant := strings.TrimSpace(body.Credentials["tenant"])
		if tenant == "" {
			tenant = "common"
		}
		q.Set("scope", "offline_access Files.ReadWrite.All Sites.ReadWrite.All")
		authURL = "https://login.microsoftonline.com/" + url.PathEscape(tenant) + "/oauth2/v2.0/authorize?" + q.Encode()
	} else {
		q.Set("scope", "https://www.googleapis.com/auth/drive")
		q.Set("access_type", "offline")
		q.Set("prompt", "consent")
		authURL = "https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode()
	}
	writeJSON(w, http.StatusOK, map[string]string{"authUrl": authURL, "nonce": nonce, "provider": provider})
}

func (a *AdminServer) handleStorageOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	session, ok := auth.SessionIdentityFromContext(r.Context())
	if !ok {
		http.Error(w, "missing authenticated session", http.StatusUnauthorized)
		return
	}
	redirectURI, err := a.storageOAuthRedirectURI(provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	flows, err := a.oauthFlows()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	result, err := flows.Consume(r.URL.Query().Get("state"), session, provider, redirectURI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		http.Error(w, "oauth code is required", http.StatusBadRequest)
		return
	}
	tokens, err := a.exchangeStorageOAuthCode(r, provider, redirectURI, code, result.Config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	payload := map[string]any{"type": "storage-oauth-result", "provider": provider, "nonce": result.Nonce, "credentials": tokens}
	b, _ := json.Marshal(payload)
	// Prevent a token containing "</script>" from terminating the inert JSON
	// script element before the external CSP-approved script reads it.
	jsonPayload := strings.ReplaceAll(string(b), "<", "\\u003c")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	_, _ = fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>OAuth completed</title><script id="oauth-result" type="application/json">%s</script><script src="/admin/api/storage/oauth/callback.js"></script><p>Authorization completed. You may close this window.</p>`, jsonPayload)
}

func (a *AdminServer) handleStorageOAuthCallbackScript(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`(()=>{const e=document.getElementById("oauth-result");if(!e)return;let d;try{d=JSON.parse(e.textContent||"")}catch{return}if(window.opener)window.opener.postMessage(d,window.location.origin);window.close()})();`))
}

func (a *AdminServer) exchangeStorageOAuthCode(r *http.Request, provider, redirectURI, code string, config map[string]string) (map[string]string, error) {
	client := a.OAuthHTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	form := url.Values{"client_id": {config["client_id"]}, "code": {code}, "redirect_uri": {redirectURI}, "grant_type": {"authorization_code"}}
	if secret := strings.TrimSpace(config["client_secret"]); secret != "" {
		form.Set("client_secret", secret)
	}
	tokenURL := "https://oauth2.googleapis.com/token"
	if provider == "onedrive" {
		tenant := strings.TrimSpace(config["tenant"])
		if tenant == "" {
			tenant = "common"
		}
		tokenURL = "https://login.microsoftonline.com/" + url.PathEscape(tenant) + "/oauth2/v2.0/token"
		form.Set("scope", "offline_access Files.ReadWrite.All Sites.ReadWrite.All")
	} else if provider != "googledrive" {
		return nil, errors.New("unsupported oauth provider")
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || out.AccessToken == "" {
		return nil, fmt.Errorf("oauth token exchange failed: %s %s", out.Error, out.ErrorDescription)
	}
	if out.RefreshToken == "" {
		out.RefreshToken = config["refresh_token"]
	}
	return map[string]string{"access_token": out.AccessToken, "refresh_token": out.RefreshToken}, nil
}
