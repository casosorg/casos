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
- Monitor Center with Kubernetes health checks, Events, diagnosis, and optional Prometheus resource trends
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
- MySQL database
- A [Casdoor](https://casdoor.org) instance (for authentication)

Supported platforms: **Linux**, **macOS**, **Windows**

## Configuration

Edit `conf/app.conf` with your values:

```ini
appname       = casos
httpport      = 9000
runmode       = dev

; Database
driverName    = mysql
dataSourceName= user:pass@tcp(host:3306)/
dbName        = casos

; Casdoor
casdoorEndpoint     = https://your-casdoor-instance
clientId            = <your-client-id>
clientSecret        = <your-client-secret>
casdoorOrganization = <your-org>
casdoorApplication  = <your-app>

; Optional Prometheus integration
prometheusAddress      = http://prometheus.monitoring.svc:9090
prometheusQueryTimeout = 10s

; Kubernetes control plane
apiserverPort = 6443
apiserverBind = 127.0.0.1
dataDir       = /var/lib/casos
```

See [Prometheus metrics monitoring](docs/monitoring.md) for exporter
requirements, supported metrics, and API examples.

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
