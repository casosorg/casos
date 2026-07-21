package server

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/casosorg/casos/conf"
	"github.com/sirupsen/logrus"
)

// Config holds control-plane settings populated from app.conf.
type Config struct {
	DataDir                   string
	ApiserverBind             string // actual bind / SAN IP (may be loopback in dev)
	AdvertiseAddress          string // non-loopback IP registered as kubernetes service endpoint
	ApiserverPort             int
	WebhookPort               int    // HTTPS port for the Casbin admission webhook server
	DSN                       string // MySQL DSN forwarded to kine
	SandboxImage              string // containerd sandbox (pause) image, empty = upstream default
	Socks5Proxy               string // outbound socks5 proxy, e.g. 127.0.0.1:10808
	CoreDNSImage              string // CoreDNS image used by the built-in DNS bootstrap
	LocalPathProvisionerImage string // local-path-provisioner controller image
	LocalPathHelperImage      string // helper pod image used by local-path-provisioner
	FlannelImage              string // Flannel daemon image used by the built-in network bootstrap
	FlannelCNIPluginImage     string // Flannel CNI plugin image installed on worker hosts
	StorageProvisionerEnabled bool   // install the built-in local-path provisioner for local clusters
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

	storageProvisionerEnabled := configBool("storageProvisionerEnabled", true)
	coreDNSImage := configStringDefault("coreDNSImage", "docker.1ms.run/coredns/coredns:1.12.4")
	localPathProvisionerImage := configStringDefault("localPathProvisionerImage", "docker.1ms.run/rancher/local-path-provisioner:v0.0.32")
	localPathHelperImage := configStringDefault("localPathHelperImage", "docker.1ms.run/library/busybox:1.37.0")
	flannelImage := configStringDefault("flannelImage", defaultFlannelImage)
	flannelCNIPluginImage := configStringDefault("flannelCNIPluginImage", defaultFlannelCNIPluginImage)

	return Config{
		DataDir:                   dataDir,
		ApiserverBind:             bind,
		AdvertiseAddress:          advertise,
		ApiserverPort:             port,
		WebhookPort:               webhookPort,
		DSN:                       dsn,
		SandboxImage:              sandboxImage,
		Socks5Proxy:               socks5Proxy,
		CoreDNSImage:              coreDNSImage,
		LocalPathProvisionerImage: localPathProvisionerImage,
		LocalPathHelperImage:      localPathHelperImage,
		FlannelImage:              flannelImage,
		FlannelCNIPluginImage:     flannelCNIPluginImage,
		StorageProvisionerEnabled: storageProvisionerEnabled,
	}, nil
}

func configInt(key string) int {
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

func configBool(key string, defaultValue bool) bool {
	value := strings.TrimSpace(conf.GetConfigString(key))
	if value == "" {
		return defaultValue
	}
	switch strings.ToLower(value) {
	case "yes", "y", "on":
		return true
	case "no", "n", "off":
		return false
	}
	res, err := strconv.ParseBool(value)
	if err != nil {
		logrus.Warnf("invalid boolean config %s=%q, using default %t", key, value, defaultValue)
		return defaultValue
	}
	return res
}

func configStringDefault(key, defaultValue string) string {
	value := strings.TrimSpace(conf.GetConfigString(key))
	if value == "" {
		return defaultValue
	}
	return value
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
