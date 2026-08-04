package proxy

import (
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/conf"
	"golang.org/x/net/http/httpproxy"
)

var (
	DefaultHttpClient *http.Client
	ProxyHttpClient   *http.Client
)

func InitHttpClient() {
	policy, err := NewEgressPolicy(GetSocks5ProxyAddress(), httpproxy.FromEnvironment().NoProxy)
	if err != nil {
		logs.Error("Control-plane egress blocked by invalid proxy configuration: %v", err)
		policy = &EgressPolicy{
			proxyForURL: func(*url.URL) (*url.URL, error) {
				return nil, err
			},
		}
	}
	client := policy.HTTPClient()
	DefaultHttpClient = client
	ProxyHttpClient = client
}

func isAddressOpen(address string) bool {
	dialAddress, err := proxyDialAddress(address)
	if err != nil {
		return false
	}
	timeout := time.Millisecond * 100
	conn, err := net.DialTimeout("tcp", dialAddress, timeout)
	if err != nil {
		// cannot connect to address, proxy is not active
		return false
	}

	if conn != nil {
		defer conn.Close()
		logs.Info("Socks5 proxy enabled: %s", RedactProxyAddress(address))
		return true
	}

	return false
}

func proxyDialAddress(address string) (string, error) {
	normalized, err := normalizeSocks5ProxyAddress(address)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", invalidProxyAddressError(address)
	}
	return parsed.Host, nil
}

func GetSocks5ProxyAddress() string {
	return conf.GetConfigString("socks5Proxy")
}

func GetActiveSocks5ProxyAddress() string {
	socks5Proxy := GetSocks5ProxyAddress()
	if socks5Proxy == "" || !isAddressOpen(socks5Proxy) {
		return ""
	}
	return socks5Proxy
}

func GetHttpClient(_ string) *http.Client {
	return ProxyHttpClient
}
