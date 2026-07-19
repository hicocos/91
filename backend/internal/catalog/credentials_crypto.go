package catalog

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	credentialsKeySize        = 32
	credentialsEnvelopeMarker = "aes-256-gcm"
	credentialsEnvelopeV1     = 1
)

type credentialsCipher struct {
	aead cipher.AEAD
}

type credentialsEnvelope struct {
	Marker     string `json:"_credentials_envelope"`
	Version    int    `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func loadCredentialsCipher(dbPath string) (*credentialsCipher, error) {
	keyPath := strings.TrimSpace(os.Getenv("VIDEO_CREDENTIALS_KEY_FILE"))
	if keyPath == "" {
		keyPath = filepath.Join(filepath.Dir(dbPath), "credentials.key")
	}
	key, err := loadOrCreateCredentialsKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("catalog: credentials key %q: %w", keyPath, err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("catalog: initialize credentials cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("catalog: initialize credentials AEAD: %w", err)
	}
	return &credentialsCipher{aead: aead}, nil
}

func loadOrCreateCredentialsKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		return validateCredentialsKey(key)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".credentials-key-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary key: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	defer tmp.Close()
	if err := tmp.Chmod(0o600); err != nil {
		return nil, fmt.Errorf("chmod temporary key: %w", err)
	}
	key = make([]byte, credentialsKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	if _, err := tmp.Write(key); err != nil {
		return nil, fmt.Errorf("write temporary key: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return nil, fmt.Errorf("sync temporary key: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close temporary key: %w", err)
	}
	if err := os.Link(tmpPath, path); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("install key: %w", err)
		}
		key, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read concurrently generated key: %w", err)
		}
	}
	return validateCredentialsKey(key)
}

func validateCredentialsKey(key []byte) ([]byte, error) {
	if len(key) != credentialsKeySize {
		return nil, fmt.Errorf("invalid key length %d (want %d raw bytes)", len(key), credentialsKeySize)
	}
	return key, nil
}

func (c *credentialsCipher) encrypt(driveID string, credentials map[string]string) (string, error) {
	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return "", fmt.Errorf("marshal credentials: %w", err)
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate credentials nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, plaintext, []byte(driveID))
	envelope := credentialsEnvelope{
		Marker:     credentialsEnvelopeMarker,
		Version:    credentialsEnvelopeV1,
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(sealed),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("marshal credentials envelope: %w", err)
	}
	return string(encoded), nil
}

func (c *credentialsCipher) decrypt(driveID, stored string) (map[string]string, bool, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stored), &probe); err != nil {
		return nil, false, fmt.Errorf("invalid credentials JSON: %w", err)
	}
	_, encrypted := probe["_credentials_envelope"]
	if !encrypted {
		var plaintext map[string]string
		if err := json.Unmarshal([]byte(stored), &plaintext); err != nil {
			return nil, false, fmt.Errorf("invalid plaintext credentials: %w", err)
		}
		if plaintext == nil {
			plaintext = map[string]string{}
		}
		return plaintext, true, nil
	}

	var envelope credentialsEnvelope
	if err := json.Unmarshal([]byte(stored), &envelope); err != nil {
		return nil, false, fmt.Errorf("invalid credentials envelope: %w", err)
	}
	if envelope.Marker != credentialsEnvelopeMarker || envelope.Version != credentialsEnvelopeV1 {
		return nil, false, fmt.Errorf("unsupported credentials envelope marker %q version %d", envelope.Marker, envelope.Version)
	}
	nonce, err := base64.RawStdEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != c.aead.NonceSize() {
		return nil, false, fmt.Errorf("invalid credentials envelope nonce")
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, false, fmt.Errorf("invalid credentials envelope ciphertext: %w", err)
	}
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, []byte(driveID))
	if err != nil {
		return nil, false, fmt.Errorf("decrypt credentials: %w", err)
	}
	var credentials map[string]string
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return nil, false, fmt.Errorf("invalid decrypted credentials: %w", err)
	}
	if credentials == nil {
		credentials = map[string]string{}
	}
	return credentials, false, nil
}
