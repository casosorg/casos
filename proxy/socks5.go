package proxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/conf"
	"golang.org/x/net/proxy"
)

var (
	DefaultHttpClient *http.Client
	ProxyHttpClient   *http.Client
)

func InitHttpClient() {
	// not use proxy
	DefaultHttpClient = http.DefaultClient

	// use proxy
	ProxyHttpClient = getProxyHttpClient()
}

func isAddressOpen(address string) bool {
	timeout := time.Millisecond * 100
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		// cannot connect to address, proxy is not active
		return false
	}

	if conn != nil {
		defer conn.Close()
		logs.Info("Socks5 proxy enabled: %s", address)
		return true
	}

	return false
}

func getProxyHttpClient() *http.Client {
	socks5Proxy := conf.GetConfigString("socks5Proxy")
	if socks5Proxy == "" {
		return &http.Client{}
	}

	if !isAddressOpen(socks5Proxy) {
		return &http.Client{}
	}

	// https://stackoverflow.com/questions/33585587/creating-a-go-socks5-client
	dialer, err := proxy.SOCKS5("tcp", socks5Proxy, nil, proxy.Direct)
	if err != nil {
		panic(err)
	}

	tr := &http.Transport{Dial: dialer.Dial, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	return &http.Client{
		Transport: tr,
	}
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

func GetHttpClient(url string) *http.Client {
	if strings.Contains(url, "hub.docker.com") {
		return ProxyHttpClient
	} else {
		return DefaultHttpClient
	}
}
