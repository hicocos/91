package storageproviders

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type Resolver func(host string) ([]net.IP, error)

// NewEndpointHTTPClient pins each connection to an address validated inside
// DialContext, preventing DNS rebinding between validation and connect.
func NewEndpointHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	resolver := net.DefaultResolver
	dialer := &net.Dialer{}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addrs, err := resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve storage endpoint: %w", err)
		}
		allowPrivate := envTrue("ALLOW_PRIVATE_STORAGE_ENDPOINTS")
		var lastErr error
		for _, addr := range addrs {
			addr = addr.Unmap()
			if !allowPrivate && !publicAddr(addr) {
				return nil, errors.New("private storage endpoint denied; set ALLOW_PRIVATE_STORAGE_ENDPOINTS=true to allow it")
			}
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, errors.New("storage endpoint resolved to no addresses")
	}
	return &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func publicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsValid() && addr.IsGlobalUnicast() && !addr.IsPrivate() && !addr.IsLoopback() && !addr.IsLinkLocalUnicast() && !addr.IsLinkLocalMulticast() && !addr.IsUnspecified()
}

func ValidateEndpoint(raw string, resolver Resolver) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return errors.New("invalid storage endpoint")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && envTrue("ALLOW_INSECURE_STORAGE_ENDPOINTS")) {
		return errors.New("storage endpoint must use HTTPS; set ALLOW_INSECURE_STORAGE_ENDPOINTS=true to allow HTTP")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("storage endpoint must not contain userinfo, query, or fragment")
	}
	if resolver == nil {
		resolver = net.LookupIP
	}
	ips, err := resolver(u.Hostname())
	if err != nil {
		return fmt.Errorf("resolve storage endpoint: %w", err)
	}
	if !envTrue("ALLOW_PRIVATE_STORAGE_ENDPOINTS") {
		for _, ip := range ips {
			if !isPublic(ip) {
				return errors.New("private storage endpoint denied; set ALLOW_PRIVATE_STORAGE_ENDPOINTS=true to allow it")
			}
		}
	}
	return nil
}
func envTrue(k string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(k)))
	return v == "1" || v == "true" || v == "yes"
}
func isPublic(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast()
}

type probeClaims struct {
	Session, Provider, Account, Digest, Nonce string
	Revision                                  int64
	Expires                                   int64
}
type ProbeTokens struct {
	key  []byte
	now  func() time.Time
	mu   sync.Mutex
	used map[string]struct{}
}

func NewProbeTokens(key []byte, now func() time.Time) *ProbeTokens {
	if now == nil {
		now = time.Now
	}
	return &ProbeTokens{key: append([]byte(nil), key...), now: now, used: map[string]struct{}{}}
}
func configDigest(c map[string]string) string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(c[k]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (s *ProbeTokens) Issue(session, provider, account string, revision int64, config map[string]string) (string, error) {
	nonceBytes := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", account, s.now().UnixNano(), len(s.used))))
	c := probeClaims{session, provider, account, configDigest(config), hex.EncodeToString(nonceBytes[:16]), revision, s.now().Add(2 * time.Minute).Unix()}
	b, _ := json.Marshal(c)
	payload := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func (s *ProbeTokens) Consume(token, session, provider, account string, revision int64, config map[string]string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return errors.New("invalid probe token")
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(parts[0]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return errors.New("invalid probe token")
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errors.New("invalid probe token")
	}
	var c probeClaims
	if json.Unmarshal(b, &c) != nil {
		return errors.New("invalid probe token")
	}
	if c.Session != session || c.Provider != provider || c.Account != account || c.Revision != revision || c.Digest != configDigest(config) {
		return errors.New("probe token binding mismatch")
	}
	if s.now().Unix() > c.Expires {
		return errors.New("probe token expired")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.used[c.Nonce]; ok {
		return errors.New("probe token already used")
	}
	s.used[c.Nonce] = struct{}{}
	return nil
}
