# Worker Node Setup (WSL2)

This guide sets up a Kubernetes worker node inside WSL2 on Windows, connecting it to the casos control plane.

**Requirements:** WSL2 with Ubuntu, systemd enabled (`[boot] systemd=true` in `/etc/wsl.conf`), casos running on Windows.

> **Networking note:** Use NAT mode in `~/.wslconfig` (`networkingMode=NAT`). Mirrored mode has a known WSL2 bug that drops all network interfaces. With NAT, the Windows host is reachable from WSL2 via the default gateway IP.

---

## 1. Fix apt sources (use Tsinghua mirror)

```bash
sudo tee /etc/apt/sources.list.d/ubuntu.sources > /dev/null << 'EOF'
Types: deb
URIs: https://mirrors.tuna.tsinghua.edu.cn/ubuntu
Suites: resolute resolute-updates resolute-backports resolute-security
Components: main restricted universe multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
EOF

sudo apt update
```

## 2. Install containerd

```bash
sudo apt install -y containerd iptables
```

Configure containerd to use the systemd cgroup driver:

```bash
sudo mkdir -p /etc/containerd
containerd config default | sudo tee /etc/containerd/config.toml > /dev/null
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
```

Point the CRI image plugin at the hosts-dir for registry mirrors:

```bash
sudo python3 << 'EOF'
with open('/etc/containerd/config.toml') as f:
    lines = f.readlines()

in_registry = False
for i, line in enumerate(lines):
    if "io.containerd.cri.v1.images'.registry]" in line or 'io.containerd.cri.v1.images".registry]' in line:
        in_registry = True
    if in_registry and 'config_path' in line:
        lines[i] = "      config_path = '/etc/containerd/certs.d'\n"
        break

with open('/etc/containerd/config.toml', 'w') as f:
    f.writelines(lines)
print('done')
EOF
```

Create per-registry mirror configs:

```bash
# Docker Hub
sudo mkdir -p /etc/containerd/certs.d/docker.io
sudo tee /etc/containerd/certs.d/docker.io/hosts.toml > /dev/null << 'EOF'
server = "https://registry-1.docker.io"

[host."https://docker.1ms.run"]
  capabilities = ["pull", "resolve"]
EOF

# registry.k8s.io (pause image)
sudo mkdir -p /etc/containerd/certs.d/registry.k8s.io
sudo tee /etc/containerd/certs.d/registry.k8s.io/hosts.toml > /dev/null << 'EOF'
server = "https://registry.k8s.io"

[host."https://registry.aliyuncs.com/google_containers"]
  capabilities = ["pull", "resolve"]
EOF
```

Start and verify:

```bash
sudo systemctl enable --now containerd
sudo systemctl is-active containerd
```

## 3. Download kubelet

```bash
WINDOWS_IP=$(ip route | grep default | awk '{print $3}')
curl -Lo /tmp/kubelet https://dl.k8s.io/v1.36.1/bin/linux/amd64/kubelet \
  --proxy "http://$WINDOWS_IP:10809"
sudo install -o root -g root -m 0755 /tmp/kubelet /usr/local/bin/kubelet
kubelet --version
```

## 4. Get the Windows host IP

With NAT networking, the Windows host is the default gateway:

```bash
WINDOWS_IP=$(ip route | grep default | awk '{print $3}')
echo "Windows host IP: $WINDOWS_IP"
```

Verify casos is reachable:

```bash
curl -s "http://$WINDOWS_IP:9000/api/get-nodes"
```

## 5. Fetch worker kubeconfig from casos

```bash
sudo mkdir -p /etc/kubernetes

WINDOWS_IP=$(ip route | grep default | awk '{print $3}')

NODE_NAME=$(hostname)

curl -s "http://$WINDOWS_IP:9000/api/get-worker-kubeconfig?nodeName=$NODE_NAME" | \
  python3 -c "
import sys, json
d = json.load(sys.stdin)
open('/tmp/worker.kubeconfig', 'w').write(d['data']['kubeconfig'])
print('ok')
"

sudo mv /tmp/worker.kubeconfig /etc/kubernetes/worker.kubeconfig
```

The generated kubeconfig points to `https://127.0.0.1:6443`. Replace it with the Windows host IP so kubelet can reach the apiserver from inside WSL2:

```bash
WINDOWS_IP=$(ip route | grep default | awk '{print $3}')
sudo sed -i "s|https://127.0.0.1:6443|https://$WINDOWS_IP:6443|g" /etc/kubernetes/worker.kubeconfig
grep server /etc/kubernetes/worker.kubeconfig
```

## 6. Install the base CNI plugins

The kubelet requires the standard CNI (Container Network Interface) plugins. CasOS manages the Flannel plugin and network configuration separately through the `kube-flannel-ds` DaemonSet.

```bash
sudo mkdir -p /opt/cni/bin /etc/cni/net.d

curl -Lo /tmp/cni-plugins.tgz \
  https://github.com/containernetworking/plugins/releases/download/v1.5.1/cni-plugins-linux-amd64-v1.5.1.tgz
sudo tar -xzf /tmp/cni-plugins.tgz -C /opt/cni/bin
```

Do not create a temporary bridge conflist. After kubelet registers the node, the controller-manager assigns its PodCIDR and the host-networked Flannel DaemonSet installs `/opt/cni/bin/flannel` plus `/etc/cni/net.d/10-flannel.conflist`. This keeps one IPAM owner and one CNI configuration across all nodes.

## 7. Create kubelet config

In kubelet 1.36+, `nodeName` is set in the config file, not as a CLI flag:

```bash
sudo mkdir -p /var/lib/kubelet

NODE_NAME=$(hostname)
sudo tee /var/lib/kubelet/config.yaml > /dev/null << EOF
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
nodeName: $NODE_NAME
cgroupDriver: systemd
failSwapOn: false
containerRuntimeEndpoint: unix:///run/containerd/containerd.sock
clusterDNS:
  - 10.43.0.10
clusterDomain: cluster.local
EOF
```

## 8. Extract the cluster CA certificate

The kubelet needs the cluster CA to authenticate requests from the API server (e.g. log streaming). It is already embedded in the worker kubeconfig — extract it:

```bash
python3 -c "
import base64
kc = open('/etc/kubernetes/worker.kubeconfig').read()
for line in kc.splitlines():
    line = line.strip()
    if line.startswith('certificate-authority-data:'):
        ca_b64 = line.split(':', 1)[1].strip()
        open('/etc/kubernetes/ca.crt', 'wb').write(base64.b64decode(ca_b64))
        print('ca.crt written')
        break
"
sudo mv /etc/kubernetes/ca.crt /etc/kubernetes/ca.crt  # already root-owned if run with sudo
```

## 9. Create the kubelet systemd service

```bash
sudo tee /etc/systemd/system/kubelet.service > /dev/null << 'EOF'
[Unit]
Description=Kubernetes Kubelet
After=containerd.service
Requires=containerd.service

[Service]
ExecStart=/usr/local/bin/kubelet \
  --kubeconfig=/etc/kubernetes/worker.kubeconfig \
  --config=/var/lib/kubelet/config.yaml \
  --client-ca-file=/etc/kubernetes/ca.crt \
  --register-node=true \
  --v=2
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now kubelet
```

## 10. Install kube-proxy

kube-proxy sets up iptables rules for NodePort and ClusterIP services. Without it, NodePort services will not listen on any port and cannot be accessed from outside the cluster.

```bash
WINDOWS_IP=$(ip route | grep default | awk '{print $3}')
curl -Lo /tmp/kube-proxy https://dl.k8s.io/v1.36.1/bin/linux/amd64/kube-proxy \
  --proxy "http://$WINDOWS_IP:10809"
sudo install -o root -g root -m 0755 /tmp/kube-proxy /usr/local/bin/kube-proxy
```

Create the config:

```bash
sudo mkdir -p /var/lib/kube-proxy

sudo tee /var/lib/kube-proxy/config.yaml > /dev/null << 'EOF'
apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
clientConnection:
  kubeconfig: /etc/kubernetes/worker.kubeconfig
mode: iptables
clusterCIDR: 10.244.0.0/16
EOF
```

Create the systemd service:

```bash
sudo tee /etc/systemd/system/kube-proxy.service > /dev/null << 'EOF'
[Unit]
Description=Kubernetes Kube-Proxy
After=network.target

[Service]
ExecStart=/usr/local/bin/kube-proxy \
  --config=/var/lib/kube-proxy/config.yaml \
  --v=2
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now kube-proxy
```

Verify — port 31000 should appear within a few seconds:

```bash
sudo systemctl status kube-proxy
ss -tlnp | grep 31000
```

## 11. Verify the node joined the cluster

Check kubelet logs:

```bash
sudo journalctl -u kubelet -n 30 --no-pager
```

Query casos for registered nodes:

```bash
WINDOWS_IP=$(ip route | grep default | awk '{print $3}')
curl -s "http://$WINDOWS_IP:9000/api/get-nodes" | python3 -m json.tool
```

The node should appear with `"status": "Ready"` after the Flannel pod on that node becomes Ready.

---

## Troubleshooting

| Symptom                                                                  | Cause                                                                            | Fix                                                                                                                                                        |
|--------------------------------------------------------------------------|----------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Network is unreachable` in WSL2                                         | `networkingMode=mirrored` failed                                                 | Set `networkingMode=NAT` in `~/.wslconfig`, run `wsl --shutdown`                                                                                           |
| `unknown flag: --node-name`                                              | Removed in kubelet 1.36                                                          | Set `nodeName` in `config.yaml` instead                                                                                                                    |
| `connection refused` to apiserver                                        | Wrong IP in kubeconfig                                                           | Re-run the `sed` command in step 5 with current `$WINDOWS_IP`                                                                                              |
| `x509: certificate is valid for 127.0.0.1, not <WSL2 gateway IP>`        | API server cert was generated before casos included all interface IPs in the SAN | On Windows: delete `<dataDir>/tls/apiserver.crt` and `apiserver.key`, then restart casos — it will regenerate the cert with all interface IPs              |
| Node stuck in `NotReady`                                                 | containerd not running                                                           | `sudo systemctl status containerd`                                                                                                                         |
| Node stuck in `NotReady` with `NetworkPluginNotReady`                    | Flannel plugin or conflist was not installed                                     | Check `ls /opt/cni/bin/flannel`, `cat /etc/cni/net.d/10-flannel.conflist`, and the `kube-flannel-ds` pod logs                                              |
| Pod stuck in `ImagePullBackOff` / i/o timeout pulling images             | Docker Hub / registry.k8s.io unreachable in restricted areas                     | Follow the registry mirror steps in section 2; verify with `sudo ctr images pull --hosts-dir /etc/containerd/certs.d docker.io/library/hello-world:latest` |
| `the server has asked for the client to provide credentials` on pod logs | kubelet missing `--client-ca-file`, can't verify API server client cert          | Extract CA from worker kubeconfig (step 8) and add `--client-ca-file=/etc/kubernetes/ca.crt` to kubelet service                                            |
| NodePort `ERR_CONNECTION_REFUSED` / port not listening                   | kube-proxy not installed or not running                                          | Follow step 10 to install kube-proxy; check `sudo systemctl status kube-proxy`                                                                             |
