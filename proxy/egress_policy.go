package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpproxy"
)

// EgressPolicy selects the control-plane proxy for each outbound request.
type EgressPolicy struct {
	proxyForURL func(*url.URL) (*url.URL, error)
}

// NewEgressPolicy builds a request-level proxy policy. A blank proxy address
// preserves the standard library's environment proxy behavior.
func NewEgressPolicy(proxyAddress, noProxy string) (*EgressPolicy, error) {
	proxyAddress = strings.TrimSpace(proxyAddress)
	if proxyAddress == "" {
		return &EgressPolicy{
			proxyForURL: func(requestURL *url.URL) (*url.URL, error) {
				return http.ProxyFromEnvironment(&http.Request{URL: requestURL})
			},
		}, nil
	}

	normalized, err := normalizeSocks5ProxyAddress(proxyAddress)
	if err != nil {
		return nil, err
	}
	proxyConfig := &httpproxy.Config{
		HTTPProxy:  normalized,
		HTTPSProxy: normalized,
		NoProxy:    noProxy,
	}
	return &EgressPolicy{proxyForURL: proxyConfig.ProxyFunc()}, nil
}

// Proxy implements the proxy callback used by http.Transport.
func (p *EgressPolicy) Proxy(req *http.Request) (*url.URL, error) {
	if p == nil || p.proxyForURL == nil {
		return nil, nil
	}
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("cannot select proxy for a request without a URL")
	}
	return p.proxyForURL(req.URL)
}

// HTTPClient returns a client whose transport retains the standard defaults
// while applying this policy independently to every request and redirect.
func (p *EgressPolicy) HTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = p.Proxy
	return &http.Client{Transport: transport}
}

func normalizeSocks5ProxyAddress(raw string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if !strings.Contains(candidate, "://") {
		candidate = "socks5h://" + candidate
	}

	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", invalidProxyAddressError(raw)
	}
	if parsed.Scheme != "socks5" && parsed.Scheme != "socks5h" {
		return "", invalidProxyAddressError(raw)
	}
	if parsed.Hostname() == "" || parsed.Port() == "" {
		return "", invalidProxyAddressError(raw)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", invalidProxyAddressError(raw)
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", invalidProxyAddressError(raw)
	}

	normalized := &url.URL{
		Scheme: "socks5h",
		User:   parsed.User,
		Host:   net.JoinHostPort(parsed.Hostname(), parsed.Port()),
	}
	return normalized.String(), nil
}

func invalidProxyAddressError(raw string) error {
	return fmt.Errorf("invalid SOCKS5 proxy address %q", RedactProxyAddress(raw))
}

// RedactProxyAddress removes a proxy password while retaining the endpoint for
// diagnostics. Malformed values are fully redacted because they cannot be
// parsed without risking credential disclosure.
func RedactProxyAddress(raw string) string {
	candidate := strings.TrimSpace(raw)
	addedScheme := !strings.Contains(candidate, "://")
	if addedScheme {
		candidate = "redact://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return "REDACTED"
	}
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(parsed.User.Username(), "REDACTED")
		}
	}
	redacted := parsed.String()
	if addedScheme {
		redacted = strings.TrimPrefix(redacted, "redact://")
	}
	return redacted
}
