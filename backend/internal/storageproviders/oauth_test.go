package storageproviders

import (
	"bytes"
	"testing"
	"time"
)

func TestPersistentOAuthFlowsSurviveRecreationAndConsumeOnce(t *testing.T) {
	now := time.Unix(2000, 0)
	store := &memoryOAuthStore{records: map[string]OAuthFlowRecord{}}
	key := bytes.Repeat([]byte{9}, 32)
	first, err := NewPersistentOAuthFlows(key, func() time.Time { return now }, store)
	if err != nil {
		t.Fatal(err)
	}
	state, nonce, err := first.Start("session", "googledrive", "https://app/callback", map[string]string{"client_secret": "secret"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewPersistentOAuthFlows(key, func() time.Time { return now }, store)
	if err != nil {
		t.Fatal(err)
	}
	got, err := second.Consume(state, "session", "googledrive", "https://app/callback")
	if err != nil {
		t.Fatal(err)
	}
	if got.Nonce != nonce || got.Config["client_secret"] != "secret" {
		t.Fatalf("got=%#v", got)
	}
	if _, err := first.Consume(state, "session", "googledrive", "https://app/callback"); err == nil {
		t.Fatal("consumed state reused")
	}
}

type memoryOAuthStore struct{ records map[string]OAuthFlowRecord }

func (m *memoryOAuthStore) PutOAuthFlow(r OAuthFlowRecord) error {
	m.records[r.StateHash] = r
	return nil
}
func (m *memoryOAuthStore) TakeOAuthFlow(k, sessionHash, provider, redirectURI string) (OAuthFlowRecord, bool, error) {
	r, ok := m.records[k]
	if !ok || r.SessionHash != sessionHash || r.Provider != provider || r.RedirectURI != redirectURI {
		return OAuthFlowRecord{}, false, nil
	}
	delete(m.records, k)
	return r, true, nil
}
func (m *memoryOAuthStore) DeleteOAuthFlow(k string) error { delete(m.records, k); return nil }

func TestOAuthFlowsHashEncryptBindExpireAndConsumeOnce(t *testing.T) {
	now := time.Unix(1000, 0)
	flows, err := NewOAuthFlows(bytes.Repeat([]byte{7}, 32), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	state, nonce, err := flows.Start("session-a", "onedrive", "https://app.example/callback", map[string]string{"client_secret": "top-secret", "name": "Drive"})
	if err != nil {
		t.Fatal(err)
	}
	if state == "" || nonce == "" {
		t.Fatal("missing state or nonce")
	}
	snapshot := flows.DebugSnapshot()
	if bytes.Contains(snapshot, []byte(state)) || bytes.Contains(snapshot, []byte("top-secret")) {
		t.Fatalf("state or pending secret stored in plaintext: %s", snapshot)
	}
	if _, err := flows.Consume(state, "session-b", "onedrive", "https://app.example/callback"); err == nil {
		t.Fatal("wrong session accepted")
	}
	if _, err := flows.Consume(state, "session-a", "googledrive", "https://app.example/callback"); err == nil {
		t.Fatal("wrong provider accepted")
	}
	got, err := flows.Consume(state, "session-a", "onedrive", "https://app.example/callback")
	if err != nil {
		t.Fatal(err)
	}
	if got.Config["client_secret"] != "top-secret" || got.Nonce != nonce {
		t.Fatalf("bad consumed flow: %#v", got)
	}
	if _, err := flows.Consume(state, "session-a", "onedrive", "https://app.example/callback"); err == nil {
		t.Fatal("state reused")
	}

	expired, _, _ := flows.Start("session-a", "google", "https://app.example/callback", map[string]string{"x": "y"})
	now = now.Add(11 * time.Minute)
	if _, err := flows.Consume(expired, "session-a", "google", "https://app.example/callback"); err == nil {
		t.Fatal("expired state accepted")
	}
}
