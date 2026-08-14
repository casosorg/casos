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
- Built-in local password authentication with first-run administrator setup
- Optional OAuth2 / OIDC authentication via [Casdoor](https://casdoor.org)

## Tech Stack

| Layer    | Technology                                |
|----------|-------------------------------------------|
| Backend  | Go 1.26+, Beego, MySQL (ORM)              |
| Frontend | React 18, Ant Design 6, recharts, i18next |
| Auth     | Local password or Casdoor (OAuth2 / OIDC) |

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

Casdoor is optional. The default local mode has no external authentication dependency.

Supported platforms: **Linux**, **macOS**, **Windows**

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

; Authentication: local or casdoor
authMode = local
; Optional: set this via an environment variable for unattended deployment.
; When blank, the generated first-run token is printed to the server log.
localSetupToken =
sessionLifetime = 86400

; Required only when authMode=casdoor
casdoorEndpoint     =
clientId            =
clientSecret        =
casdoorOrganization =
casdoorApplication  =

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
root.

### Authentication

With the default `authMode=local`, open CasOS in a browser on the machine that
runs it and set the password for the built-in `admin` account; no other step is
required. The password must contain at least 8 characters and at most 72 bytes.
CasOS stores only its bcrypt hash in the business database.

First-run setup from another machine requires a one-time setup token: CasOS
prints one to the server log, or you can fix it through the `localSetupToken`
environment variable (at least 16 characters) for unattended deployment and
reverse-proxy setups, where the server cannot tell whether a request is local.

Sessions are held in process memory and expire after `sessionLifetime` seconds
(24 hours by default), so restarting CasOS requires signing in again. In
`casdoor` mode, a session also ends once the Casdoor access token expires; there
is no silent refresh. Do not send local credentials over an untrusted HTTP
network; terminate HTTPS in front of CasOS for remote access.

To use Casdoor instead, set `authMode=casdoor` and configure all five Casdoor
settings shown above. CasOS validates that configuration at startup. Local
accounts remain stored when modes are switched, but sessions created in one
mode are not accepted in the other mode.

Existing configurations that omit `authMode` but have a non-empty
`casdoorEndpoint` continue to select Casdoor for upgrade safety. Set the mode
explicitly when updating the configuration.

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

### Lint

```bash
cd web

yarn lint:js    # ESLint
yarn lint:css   # Stylelint
yarn lint       # both
```

## License

[Apache 2.0](https://github.com/casosorg/casos/blob/master/LICENSE)
