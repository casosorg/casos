package deploy

import "fmt"

func bridgeCNIConfig(podCIDR string) string {
	return fmt.Sprintf(`{
  "cniVersion": "1.0.0",
  "name": "casos-bridge",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni0",
      "isGateway": true,
      "ipMasq": true,
      "ipam": {
        "type": "host-local",
        "ranges": [[{"subnet": %q}]],
        "routes": [{"dst": "0.0.0.0/0"}]
      }
    },
    {"type": "portmap", "capabilities": {"portMappings": true}},
    {"type": "loopback"}
  ]
}
`, podCIDR)
}
