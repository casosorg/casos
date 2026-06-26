package server

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/casosorg/casos/conf"
)

// Config holds control-plane settings populated from app.conf.
type Config struct {
	DataDir          string
	ApiserverBind    string // actual bind / SAN IP (may be loopback in dev)
	AdvertiseAddress string // non-loopback IP registered as kubernetes service endpoint
	ApiserverPort    int
	WebhookPort      int    // HTTPS port for the Casbin admission webhook server
	DSN              string // MySQL DSN forwarded to kine
	SandboxImage     string // containerd sandbox (pause) image, empty = upstream default
	Socks5Proxy      string // outbound socks5 proxy, e.g. 127.0.0.1:10808
}

// ConfigFromAppConf reads server config from the beego app.conf.
func ConfigFromAppConf() (Config, error) {
	dataDir := conf.GetConfigString("dataDir")
	if dataDir == "" {
		dataDir = "/var/lib/casos"
	}
	bind := conf.GetConfigString("apiserverBind")
	if bind == "" {
		bind = outboundIP()
	}
	port := configInt("apiserverPort")
	if port == 0 {
		port = 6443
	}
	dsn := conf.GetConfigString("dataSourceName")
	if dsn == "" {
		return Config{}, fmt.Errorf("dataSourceName not set in app.conf")
	}
	dbName := conf.GetConfigString("dbName")
	if dbName == "" {
		dbName = "casos"
	}
	dsn = injectDBName(dsn, dbName)

	advertise := outboundIP()
	if advertise == "127.0.0.1" || advertise == "::1" {
		advertise = bind
	}

	webhookPort := configInt("webhookPort")
	if webhookPort == 0 {
		webhookPort = 9443
	}

	socks5Proxy := conf.GetConfigString("socks5Proxy")

	sandboxImage := conf.GetConfigString("sandboxImage")
	if sandboxImage == "" {
		if socks5Proxy != "" {
			sandboxImage = "registry.aliyuncs.com/google_containers/pause:3.10.1"
		} else {
			sandboxImage = "registry.k8s.io/pause:3.10.1"
		}
	}

	return Config{
		DataDir:          dataDir,
		ApiserverBind:    bind,
		AdvertiseAddress: advertise,
		ApiserverPort:    port,
		WebhookPort:      webhookPort,
		DSN:              dsn,
		SandboxImage:     sandboxImage,
		Socks5Proxy:      socks5Proxy,
	}, nil
}

func configInt(key string) int {
	// Preserve the old AppConfig.Int behavior here: missing or malformed values
	// should fall back to 0 instead of panicking like conf.GetConfigInt.
	value := conf.GetConfigString(key)
	if value == "" {
		return 0
	}
	res, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return res
}

// injectDBName inserts dbName into a MySQL DSN of the form
// user:pass@tcp(host:port)/ (trailing slash, no database).
// If a database is already present it is replaced.
func injectDBName(dsn, dbName string) string {
	idx := strings.LastIndex(dsn, "/")
	if idx < 0 {
		return dsn + dbName
	}
	base := dsn[:idx+1]
	rest := dsn[idx+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		return base + dbName + rest[q:]
	}
	return base + dbName
}

// outboundIP returns the preferred non-loopback outbound IP of this machine.
func outboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
