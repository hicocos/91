// Package streamhttp contains the HTTP redirect policy shared by consumers of
// drive StreamLink values.
package streamhttp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

// NewClient returns a streaming client that follows redirects without leaking
// drive credentials or adding an implicit Referer to a different origin.
//
// Go's net/http client normally synthesizes Referer while following redirects.
// Some signed download endpoints (including WoPan links returned by OpenList's
// WebDAV 302 mode) reject any Referer with HTTP 400. Explicit provider headers
// remain intact; only a Referer synthesized by net/http is removed.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: CheckRedirect,
	}
}

// NewNoRedirectClient returns a client for transparent HTTP relays. The first
// response is returned to the caller even when it is a redirect.
func NewNoRedirectClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// NewPublicNetworkClient returns an HTTP client for untrusted URLs. Every
// hostname is resolved immediately before dialing; all answers must be public,
// and the connection is pinned to one validated address so DNS cannot change
// between validation and connect. Redirects repeat the same policy per hop.
func NewPublicNetworkClient(timeout time.Duration) *http.Client {
	resolver := net.DefaultResolver
	dialer := &net.Dialer{}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addrs, err := resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		var public []netip.Addr
		for _, addr := range addrs {
			addr = addr.Unmap()
			if !IsPublicAddr(addr) {
				return nil, errors.New("streamhttp: non-public upstream address")
			}
			public = append(public, addr)
		}
		if len(public) == 0 {
			return nil, errors.New("streamhttp: upstream hostname resolved to no public addresses")
		}
		return dialPublicAddrs(ctx, dialer, network, port, public)
	}
	return &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: CheckRedirect,
	}
}

func dialPublicAddrs(ctx context.Context, dialer *net.Dialer, network, port string, addrs []netip.Addr) (net.Conn, error) {
	var errs []error
	for _, addr := range addrs {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
		errs = append(errs, err)
	}
	return nil, errors.Join(errs...)
}

// IsPublicAddr rejects all addresses that can target the host or private
// networks. IsPrivate covers RFC1918 and IPv6 ULA; mapped IPv4 is normalized.
func IsPublicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsValid() && addr.IsGlobalUnicast() &&
		!addr.IsLoopback() && !addr.IsPrivate() && !addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() && !addr.IsUnspecified()
}

// NewPinnedPublicNetworkClient is test support for deterministic resolver and
// dial behavior while retaining the production validation algorithm.
func NewPinnedPublicNetworkClient(timeout time.Duration, lookup func(context.Context, string) ([]netip.Addr, error), dial func(context.Context, string, string) (net.Conn, error)) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addrs, err := lookup(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			if !IsPublicAddr(addr) {
				return nil, errors.New("streamhttp: non-public upstream address")
			}
		}
		if len(addrs) == 0 {
			return nil, errors.New("streamhttp: upstream hostname resolved to no public addresses")
		}
		var errs []error
		for _, addr := range addrs {
			conn, err := dial(ctx, network, net.JoinHostPort(addr.Unmap().String(), port))
			if err == nil {
				return conn, nil
			}
			errs = append(errs, err)
		}
		return nil, errors.Join(errs...)
	}
	return &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: CheckRedirect}
}

// drainAndClose is kept small so callers can safely discard a blocked redirect
// response without retaining a connection.
func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 4<<10))
	_ = body.Close()
}

var _ = sync.Once{}

// CheckRedirect is suitable for http.Client.CheckRedirect when fetching a
// StreamLink. Range and other ordinary streaming headers are left untouched.
func CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 || sameOrigin(via[0].URL, req.URL) {
		return nil
	}

	// Preserve a provider-supplied Referer, but remove the source URL that
	// net/http automatically generated during this cross-origin redirect.
	if explicit, ok := via[0].Header["Referer"]; ok {
		req.Header.Del("Referer")
		for _, value := range explicit {
			req.Header.Add("Referer", value)
		}
	} else {
		req.Header.Del("Referer")
	}

	// net/http already avoids forwarding these headers to unrelated hosts.
	// Delete them explicitly as a defense in depth and also cover redirects to
	// the same hostname on a different scheme or port.
	req.Header.Del("Authorization")
	req.Header.Del("Proxy-Authorization")
	req.Header.Del("Cookie")
	return nil
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
