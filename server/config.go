package server

import (
	"fmt"
	"net"
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
	WebhookPort               int                // HTTPS port for the Casbin admission webhook server
	DSN                       string             // MySQL DSN forwarded to kine
	SandboxImage              string             // containerd sandbox (pause) image, empty = upstream default
	CoreDNSImage              string             // CoreDNS image used by the built-in DNS bootstrap
	LocalPathProvisionerImage string             // local-path-provisioner controller image
	LocalPathHelperImage      string             // helper pod image used by local-path-provisioner
	FlannelImage              string             // Flannel daemon image used by the built-in network bootstrap
	FlannelCNIPluginImage     string             // Flannel CNI plugin image installed on worker hosts
	IngressControllerImage    string             // Traefik image used by the built-in Ingress controller
	ServiceLBImage            string             // hostPort proxy image used by the built-in ServiceLB
	StorageProvisionerEnabled bool               // install the built-in local-path provisioner for local clusters
	RegistryMirrorMode        RegistryMirrorMode // imageRegistryMirror mode, evaluated on each target worker
	IngressControllerEnabled  bool               // install the built-in Traefik controller
	ServiceLBEnabled          bool               // run the built-in bare-metal LoadBalancer controller
}

// ConfigFromAppConf reads server config from the beego app.conf.
func ConfigFromAppConf() (Config, error) {
	dataDir := conf.GetConfigStringDefault("dataDir", "/var/lib/casos")
	bind := conf.GetConfigStringDefault("apiserverBind", outboundIP())
	port := conf.GetConfigIntDefault("apiserverPort", 6443)
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

	webhookPort := conf.GetConfigIntDefault("webhookPort", 9443)

	sandboxImage := conf.GetConfigStringDefault("sandboxImage", "registry.k8s.io/pause:3.10.1")

	storageProvisionerEnabled := conf.GetConfigBoolDefault("storageProvisionerEnabled", true)
	coreDNSImage := conf.GetConfigStringDefault("coreDNSImage", "docker.io/coredns/coredns:1.12.4")
	localPathProvisionerImage := conf.GetConfigStringDefault("localPathProvisionerImage", "docker.io/rancher/local-path-provisioner:v0.0.32")
	localPathHelperImage := conf.GetConfigStringDefault("localPathHelperImage", "docker.io/library/busybox:1.37.0")
	flannelImage := conf.GetConfigStringDefault("flannelImage", defaultFlannelImage)
	flannelCNIPluginImage := conf.GetConfigStringDefault("flannelCNIPluginImage", defaultFlannelCNIPluginImage)
	registryMirrorMode, err := resolveRegistryMirrorMode()
	if err != nil {
		return Config{}, err
	}
	ingressControllerEnabled := conf.GetConfigBoolDefault("ingressControllerEnabled", false)
	serviceLBEnabled := conf.GetConfigBoolDefault("serviceLBEnabled", false)
	ingressControllerImage := conf.GetConfigStringDefault("ingressControllerImage", defaultIngressControllerImage)
	serviceLBImage := conf.GetConfigStringDefault("serviceLBImage", defaultServiceLBImage)
	config := Config{
		DataDir:                   dataDir,
		ApiserverBind:             bind,
		AdvertiseAddress:          advertise,
		ApiserverPort:             port,
		WebhookPort:               webhookPort,
		DSN:                       dsn,
		SandboxImage:              sandboxImage,
		CoreDNSImage:              coreDNSImage,
		LocalPathProvisionerImage: localPathProvisionerImage,
		LocalPathHelperImage:      localPathHelperImage,
		FlannelImage:              flannelImage,
		FlannelCNIPluginImage:     flannelCNIPluginImage,
		IngressControllerImage:    ingressControllerImage,
		ServiceLBImage:            serviceLBImage,
		StorageProvisionerEnabled: storageProvisionerEnabled,
		RegistryMirrorMode:        registryMirrorMode,
		IngressControllerEnabled:  ingressControllerEnabled,
		ServiceLBEnabled:          serviceLBEnabled,
	}
	normalizeApplicationAccessConfig(&config)
	return config, nil
}

func normalizeApplicationAccessConfig(config *Config) {
	if config != nil && config.IngressControllerEnabled && !config.ServiceLBEnabled {
		logrus.Warn("ingressControllerEnabled requires serviceLBEnabled; disabling the built-in Ingress controller")
		config.IngressControllerEnabled = false
	}
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
