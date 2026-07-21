package storageproviders

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"
)

const OAuthFlowTTL = 10 * time.Minute

type oauthFlow struct {
	StateHash   string    `json:"stateHash"`
	SessionHash string    `json:"sessionHash"`
	Provider    string    `json:"provider"`
	RedirectURI string    `json:"redirectUri"`
	Nonce       string    `json:"nonce"`
	Pending     []byte    `json:"pending"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// OAuthFlowRecord is safe to persist: state/session are hashes and Pending is
// AES-GCM ciphertext authenticated with the provider name.
type OAuthFlowRecord struct {
	StateHash   string
	SessionHash string
	Provider    string
	RedirectURI string
	Nonce       string
	Pending     []byte
	ExpiresAt   time.Time
}

type OAuthFlowStore interface {
	PutOAuthFlow(record OAuthFlowRecord) error
	TakeOAuthFlow(stateHash, sessionHash, provider, redirectURI string) (OAuthFlowRecord, bool, error)
	DeleteOAuthFlow(stateHash string) error
}

type OAuthResult struct {
	Nonce  string
	Config map[string]string
}

type OAuthFlows struct {
	aead  cipher.AEAD
	now   func() time.Time
	mu    sync.Mutex
	flows map[string]oauthFlow
	store OAuthFlowStore
}

func NewOAuthFlows(key []byte, now func() time.Time) (*OAuthFlows, error) {
	return NewPersistentOAuthFlows(key, now, nil)
}

func NewPersistentOAuthFlows(key []byte, now func() time.Time, store OAuthFlowStore) (*OAuthFlows, error) {
	if len(key) != 32 {
		return nil, errors.New("oauth flow key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return &OAuthFlows{aead: aead, now: now, flows: map[string]oauthFlow{}, store: store}, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func (s *OAuthFlows) Start(session, provider, redirectURI string, config map[string]string) (string, string, error) {
	if session == "" || provider == "" || redirectURI == "" {
		return "", "", errors.New("oauth flow binding required")
	}
	state, err := randomHex(32)
	if err != nil {
		return "", "", err
	}
	nonce, err := randomHex(16)
	if err != nil {
		return "", "", err
	}
	plain, err := json.Marshal(config)
	if err != nil {
		return "", "", err
	}
	iv := make([]byte, s.aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, iv); err != nil {
		return "", "", err
	}
	pending := append(iv, s.aead.Seal(nil, iv, plain, []byte(provider))...)
	f := oauthFlow{StateHash: hashString(state), SessionHash: hashString(session), Provider: provider, RedirectURI: redirectURI, Nonce: nonce, Pending: pending, ExpiresAt: s.now().Add(OAuthFlowTTL)}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		if err := s.store.PutOAuthFlow(toOAuthFlowRecord(f)); err != nil {
			return "", "", err
		}
	} else {
		s.flows[f.StateHash] = f
	}
	return state, nonce, nil
}

func (s *OAuthFlows) Consume(state, session, provider, redirectURI string) (OAuthResult, error) {
	key := hashString(state)
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionHash := hashString(session)
	f, ok, err := s.takeLocked(key, sessionHash, provider, redirectURI)
	if err != nil {
		return OAuthResult{}, err
	}
	if !ok {
		return OAuthResult{}, errors.New("invalid oauth state")
	}
	if f.SessionHash != sessionHash || f.Provider != provider || f.RedirectURI != redirectURI {
		return OAuthResult{}, errors.New("oauth state binding mismatch")
	}
	if s.now().After(f.ExpiresAt) {
		_ = s.deleteLocked(key)
		return OAuthResult{}, errors.New("oauth state expired")
	}
	if s.store == nil {
		delete(s.flows, key)
	}
	n := s.aead.NonceSize()
	if len(f.Pending) < n {
		return OAuthResult{}, errors.New("invalid pending config")
	}
	plain, err := s.aead.Open(nil, f.Pending[:n], f.Pending[n:], []byte(provider))
	if err != nil {
		return OAuthResult{}, err
	}
	var config map[string]string
	if err = json.Unmarshal(plain, &config); err != nil {
		return OAuthResult{}, err
	}
	return OAuthResult{Nonce: f.Nonce, Config: config}, nil
}

func (s *OAuthFlows) takeLocked(key, sessionHash, provider, redirectURI string) (oauthFlow, bool, error) {
	if s.store == nil {
		f, ok := s.flows[key]
		return f, ok, nil
	}
	record, ok, err := s.store.TakeOAuthFlow(key, sessionHash, provider, redirectURI)
	return fromOAuthFlowRecord(record), ok, err
}

func (s *OAuthFlows) deleteLocked(key string) error {
	if s.store != nil {
		return s.store.DeleteOAuthFlow(key)
	}
	delete(s.flows, key)
	return nil
}

func toOAuthFlowRecord(f oauthFlow) OAuthFlowRecord {
	return OAuthFlowRecord{StateHash: f.StateHash, SessionHash: f.SessionHash, Provider: f.Provider, RedirectURI: f.RedirectURI, Nonce: f.Nonce, Pending: append([]byte(nil), f.Pending...), ExpiresAt: f.ExpiresAt}
}

func fromOAuthFlowRecord(r OAuthFlowRecord) oauthFlow {
	return oauthFlow{StateHash: r.StateHash, SessionHash: r.SessionHash, Provider: r.Provider, RedirectURI: r.RedirectURI, Nonce: r.Nonce, Pending: append([]byte(nil), r.Pending...), ExpiresAt: r.ExpiresAt}
}

func (s *OAuthFlows) DebugSnapshot() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(s.flows)
	return b
}
