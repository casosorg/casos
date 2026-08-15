<div align="center">

# CasOS

**A cloud operating system built on Kubernetes**

[![Build](https://github.com/casosorg/casos/workflows/Build/badge.svg?style=flat-square)](https://github.com/casosorg/casos/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/v/release/casosorg/casos?style=flat-square&color=4f46e5)](https://github.com/casosorg/casos/releases/latest)
[![Go Report](https://goreportcard.com/badge/github.com/casosorg/casos?style=flat-square)](https://goreportcard.com/report/github.com/casosorg/casos)
[![License](https://img.shields.io/github/license/casosorg/casos?style=flat-square&color=22c55e)](https://github.com/casosorg/casos/blob/master/LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-blue?style=flat-square)](https://github.com/casosorg/casos/releases/latest)
[![Discord](https://img.shields.io/discord/1022748306096537660?logo=discord&label=discord&color=5865F2&style=flat-square)](https://discord.gg/6ma4BAmV7P)

**Official Website: [https://www.casos.net](https://www.casos.net)**

**Live Demo: [https://demo.casos.net](https://demo.casos.net)**

</div>

---

## What is CasOS?

CasOS is a cloud operating system built on Kubernetes. It embeds the Kubernetes API server, controller manager, and scheduler, so you do **not** need an existing Kubernetes cluster or a separate control plane — CasOS is the platform itself. Run a single binary and get a fully functional cloud OS with a built-in web UI.

## Features

- Embedded Kubernetes stack (API server, controller manager, scheduler) — no external cluster needed
- Cluster resource management: Nodes, Namespaces, Pods, Services, ConfigMaps, ServiceAccounts, ClusterRoleBindings
- Dashboard with cluster overview
- DockerHub image browser
- Multi-language support (i18n)
- Authentication via [Casdoor](https://casdoor.org)

## Tech Stack

| Layer    | Technology                                |
|----------|-------------------------------------------|
| Backend  | Go 1.26+, Beego, MySQL (ORM)              |
| Frontend | React 18, Ant Design 6, recharts, i18next |
| Auth     | Casdoor (OAuth2 / OIDC)                   |

## Project Structure

```
casos/
├── main.go                  # Entry point
├── conf/app.conf            # Backend configuration
├── controllers/             # HTTP controllers (Beego routing)
├── object/                  # Business logic and data models
├── routers/                 # Route configuration and filters
├── proxy/                   # kube-proxy related logic
└── web/                     # React frontend
    └── src/
        ├── App.js
        ├── DashboardPage.js
        ├── ManagementPage.js
        ├── PodListPage.js
        ├── NodeListPage.js
        ├── NamespaceListPage.js
        ├── ServiceListPage.js
        ├── ConfigMapListPage.js
        ├── ServiceAccountListPage.js
        ├── ClusterRoleBindingListPage.js
        └── backend/         # API client helpers
```

## Prerequisites

- **Backend**: [Go](https://golang.org/dl/) 1.26+
- **Frontend**: [Node.js](https://nodejs.org/) 20+ and [Yarn](https://classic.yarnpkg.com/) 1.x
- A [Casdoor](https://casdoor.org) instance (for authentication)

Supported platforms: **Linux**, **macOS**, **Windows**

## Install

Linux (amd64 or arm64):

```bash
curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | bash
```

Windows PowerShell (x64 or arm64):

```powershell
irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1 | iex
```

The installer downloads the matching binary from the latest GitHub release,
verifies it against `SHA256SUMS`, and adds CasOS to your user `PATH`. On Linux,
open a new shell or run the `source` command printed by the installer before
running `casos`; the installer also prints the absolute executable path. On
Windows, the current PowerShell session is updated immediately. Open
`http://localhost:9000` after starting CasOS. Rerun the command to upgrade. Set
`CASOS_VERSION` to a release tag such as `v1.32.0` to install that version.
For a piped Linux install, pass installer variables to `bash`, not to `curl`:

```bash
curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | CASOS_VERSION=v1.32.0 INSTALL_DIR="$HOME/.local/bin" bash
```

## Configuration

Edit `conf/app.conf` with your values:

```ini
appname       = casos
httpport      = 9000
runmode       = dev

; Database
driverName    = sqlite
dataSourceName=
dbName        = casos
kineEndpoint  =

; Casdoor
casdoorEndpoint     = https://your-casdoor-instance
clientId            = <your-client-id>
clientSecret        = <your-client-secret>
casdoorOrganization = <your-org>
casdoorApplication  = <your-app>

; Optional control-plane SOCKS5 proxy
; Leave blank to use environment proxy settings or direct access.
; When set, the proxy is required for requests not matched by NO_PROXY.
socks5Proxy =

; Kubernetes control plane
apiserverPort = 6443
apiserverBind = 127.0.0.1
dataDir       = ./data
```

SQLite is the default and stores CasOS business data in `data/casos.db` and
Kubernetes state in `data/kine/state.db`. The `data` directory is ignored by
Git and is writable when running the development command from the repository
root. A binary that runs without `conf/app.conf` and without a `dataDir`
environment variable falls back to the per-user data directory
(`~/.local/share/casos`, `~/Library/Application Support/CasOS`, or
`%LOCALAPPDATA%\CasOS`) instead of a working-directory-relative path.

> **Breaking change:** `dataDir` used to default to `/var/lib/casos`. It now
> defaults to `./data`, which is resolved against the working directory of the
> CasOS process, so starting CasOS from a different directory points it at a
> different — and most likely empty — data directory. Besides the two SQLite
> databases, `dataDir` also holds the control-plane TLS material and the key
> that encrypts stored SSH private keys, so a service that loses track of it
> can no longer decrypt existing machine credentials. Set `dataDir` to an
> absolute path such as `/var/lib/casos` for any system installation, make sure
> the service user can write to it, and move an existing `/var/lib/casos` to
> the new location before restarting. CasOS logs the directory it resolved as
> `casos data directory: <path>` on startup.

MySQL remains available by setting `driverName=mysql`, configuring
`dataSourceName` and `dbName`, and optionally setting a complete
`kineEndpoint`. Switching an existing installation from MySQL to SQLite starts
with empty SQLite databases; existing MySQL data is not imported automatically.

The control-plane proxy accepts `host:port`, `socks5://`, and `socks5h://`
addresses. When `socks5Proxy` is set, CasOS fails requests that cannot use the
configured proxy instead of silently falling back to direct access. Destinations
matched by the CasOS process `NO_PROXY` environment variable continue to use
direct access. When `socks5Proxy` is blank, CasOS follows `HTTP_PROXY`,
`HTTPS_PROXY`, and `NO_PROXY`, and uses direct access when no environment proxy
is configured.

**Upgrade notice:** the previous example default of `127.0.0.1:10808` has been
removed. Set `socks5Proxy` explicitly before upgrading if that local proxy is a
required control-plane dependency.

## Development

### Backend

```bash
go run main.go
```

### Frontend

```bash
cd web

# Install dependencies (first time only)
yarn install

# Start dev server — port 8001, proxies API to localhost:9000
yarn start
```

## Deployment

### Backend

```bash
# Build binary
go build -o casos .

# Run
./casos
```

### Frontend

```bash
cd web

# Production build (outputs to web/build/)
yarn build
```

Serve the `web/build/` directory with any static file server, or let the Go backend serve it directly.

### Standalone binary

Building with `-tags embed` compiles `web/build/` into the backend, producing a
single file that serves the UI without needing the directory beside it. Run
`yarn build` first — the build tag requires `web/build/` to exist.

```bash
cd web && GENERATE_SOURCEMAP=false yarn build && cd ..
CGO_ENABLED=0 go build -trimpath -tags embed -o casos .
```

`GENERATE_SOURCEMAP=false` keeps the `.map` files out of the binary; without it
they are embedded as well and add tens of megabytes.

Every release publishes these binaries for Linux and Windows on amd64 and
arm64, alongside a `SHA256SUMS` file, on the
[releases page](https://github.com/casosorg/casos/releases/latest).

### Lint

```bash
cd web

yarn lint:js    # ESLint
yarn lint:css   # Stylelint
yarn lint       # both
```

## License

[Apache 2.0](https://github.com/casosorg/casos/blob/master/LICENSE)
