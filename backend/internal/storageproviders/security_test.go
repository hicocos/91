package storageproviders

import (
	"net"
	"os"
	"testing"
	"time"
)

func TestValidateEndpointRejectsHTTPAndPrivateByDefault(t *testing.T) {
	os.Unsetenv("ALLOW_PRIVATE_STORAGE_ENDPOINTS")
	os.Unsetenv("ALLOW_INSECURE_STORAGE_ENDPOINTS")
	resolver := func(host string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.1")}, nil }
	if err := ValidateEndpoint("http://storage.example/b", resolver); err == nil {
		t.Fatal("expected insecure endpoint rejection")
	}
	if err := ValidateEndpoint("https://storage.example/b", resolver); err == nil {
		t.Fatal("expected private endpoint rejection")
	}
}

func TestProbeTokensAreBoundExpiringAndOneTime(t *testing.T) {
	now := time.Unix(1000, 0)
	s := NewProbeTokens([]byte("01234567890123456789012345678901"), func() time.Time { return now })
	token, err := s.Issue("session-a", "s3", "account-a", 2, map[string]string{"bucket": "a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Consume(token, "session-b", "s3", "account-a", 2, map[string]string{"bucket": "a"}); err == nil {
		t.Fatal("session mismatch accepted")
	}
	if err := s.Consume(token, "session-a", "s3", "account-a", 2, map[string]string{"bucket": "b"}); err == nil {
		t.Fatal("config mismatch accepted")
	}
	if err := s.Consume(token, "session-a", "s3", "account-a", 2, map[string]string{"bucket": "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Consume(token, "session-a", "s3", "account-a", 2, map[string]string{"bucket": "a"}); err == nil {
		t.Fatal("token reused")
	}
	expired, _ := s.Issue("session-a", "s3", "account-a", 2, map[string]string{"bucket": "a"})
	now = now.Add(3 * time.Minute)
	if err := s.Consume(expired, "session-a", "s3", "account-a", 2, map[string]string{"bucket": "a"}); err == nil {
		t.Fatal("expired token accepted")
	}
}
