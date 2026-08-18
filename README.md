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
- Built-in `admin` account out of the box, with optional [Casdoor](https://casdoor.org) single sign-on

## Tech Stack

| Layer    | Technology                                |
|----------|-------------------------------------------|
| Backend  | Go 1.26+, Beego, SQLite by default (MySQL optional) |
| Frontend | React 18, Ant Design 6, recharts, i18next |
| Auth     | Built-in account, or Casdoor (OAuth2 / OIDC) |

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
- Optionally, a [Casdoor](https://casdoor.org) instance, if you want single sign-on instead of the built-in account

Supported platforms: **Linux**, **macOS**, **Windows**

## Install

Linux and macOS (x86_64):

```bash
curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | bash
```

Windows PowerShell (x86_64):

```powershell
irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1 | iex
```

The installer downloads the matching archive from the latest GitHub release,
unpacks the executable, and adds CasOS to your user `PATH`. On Linux
and macOS, open a new shell or run the `source` command printed by the
installer before running `casos`; the installer also prints the absolute
executable path. On Windows, the current PowerShell session is updated
immediately. Releases are x86_64 only: an Apple Silicon Mac and a Windows on
ARM machine both get that build and run it under emulation, while an arm64
Linux host has to build from source. Open
`http://localhost:9000` after starting CasOS. Rerun the command to upgrade. Set
`CASOS_VERSION` to a release tag such as `v1.32.0` to install that version.
Both installers read the same settings:

| Variable            | Default                                    | Purpose                          |
|---------------------|--------------------------------------------|----------------------------------|
| `CASOS_VERSION`     | `latest`                                   | Release tag, such as `v1.32.0`   |
| `INSTALL_DIR`       | `$HOME/.local/bin`, `%LOCALAPPDATA%\CasOS\bin` | Directory to install into    |
| `CASOS_REPOSITORY`  | `casosorg/casos`                           | Release repository to pull from  |

For a piped Linux or macOS install, pass installer variables to `bash`, not to
`curl`:

```bash
curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | CASOS_VERSION=v1.32.0 INSTALL_DIR="$HOME/.local/bin" bash
```

PowerShell has no equivalent prefix syntax. `iex` runs the installer inside the
current session, so set the variables as `$env:` entries on their own lines
first:

```powershell
$env:CASOS_VERSION = 'v1.32.0'
irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1 | iex
```

Those entries outlive the install and would pin any later run in the same
window, so clear the ones you no longer want:

```powershell
Remove-Item Env:\CASOS_VERSION
```

Check what you ended up with, which is also what to quote in a bug report:

```bash
casos --version
```

A released binary prints its tag, commit and build date; a binary you built
yourself reports `dev`.

## First run

CasOS needs at least one Kubernetes worker node before it can schedule
workloads. After signing in, use the dashboard's first-run checklist to finish
these steps:

1. Change the default `admin` password (`123`) from the account menu.
2. Add a machine with SSH credentials on the **Machines** page.
3. Deploy that machine as a worker node. The node must join the cluster and be
   ready before workloads can run.
4. Install the first application from the **App Store**.

The checklist derives its state from the cluster, so it remains accurate after
refreshing the browser or signing in from another session.

### Uninstall

The uninstaller removes the binary and the `PATH` entry the installer added.
Pass the same `INSTALL_DIR` you installed with, if it was not the default.

```bash
curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | bash -s -- --uninstall
```

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1))) -Uninstall
```

Your data directory is **not** deleted. It holds the SQLite databases, the
control-plane TLS material and the key that decrypts stored SSH credentials, so
removing it is a separate, deliberate step. The uninstaller prints the path and
the exact command for it.

### Notes

The macOS binaries are not notarized. The installer is unaffected because
`curl` does not mark downloads with `com.apple.quarantine`, but a build fetched
manually through a browser is quarantined and Gatekeeper refuses to run it —
including the executable unpacked from a quarantined archive. Clear the
attribute after such a download:

```bash
xattr -d com.apple.quarantine ./casos
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

; Casdoor (optional, see Sign-in below)
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

### Sign-in

`casdoorEndpoint` decides how you sign in, and nothing else has to be set up:

| `casdoorEndpoint` | Sign-in |
|---|---|
| blank (default) | Built-in account, stored in the CasOS database |
| set | Casdoor single sign-on; the built-in account is never created |

On first start CasOS creates a single built-in account, `admin`, with the
password `123`, and signs you straight in so a fresh install has nothing to
configure. **That also means anyone who can reach the CasOS port is an
administrator until you change that password.** Open the account menu in the
top-right corner, pick *My Account*, and set a real one before exposing CasOS
beyond your own machine — the automatic sign-in stops the moment the password is
no longer `123`. There is no password recovery: if you lose it, delete the row
from the `user` table in `data/casos.db` and restart CasOS to get the default
account back.

To use Casdoor instead, fill in all five Casdoor settings before the first
start. Each one also reads an environment variable of the same name, so
`clientSecret` does not have to be written to disk:

```bash
casdoorEndpoint=https://your-casdoor-instance clientId=... clientSecret=... casdoorOrganization=... casdoorApplication=... casos
```

**Upgrade notice:** earlier releases shipped with `conf/app.conf` pointing at a
shared public Casdoor demo application, credentials included. Those values are
now blank. An installation that relied on them must configure its own Casdoor
application, or clear the five settings to switch to the built-in account.

## Development

### Backend

```bash
go run main.go
```

### Frontend

```bash
cd web2

# Install dependencies (first time only)
yarn install

# Start dev server — port 8002, proxies API to localhost:9000
yarn start
```

`web/` is the previous Ant Design frontend. It still builds and the backend will
still serve it, but `web2/` is what ships.

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
cd web2

# Production build (outputs to web2/build/)
yarn build
```

Serve the `web2/build/` directory with any static file server, or let the Go
backend serve it directly. The backend prefers `web2/build/` and falls back to
`web/build/`, so deleting the former is all it takes to go back to the old UI.

### Standalone binary

Building with `-tags embed` compiles `web2/build/` into the backend, producing a
single file that serves the UI without needing the directory beside it. Run
`yarn build` first — the build tag requires `web2/build/` to exist.

```bash
cd web2 && yarn build && cd ..
CGO_ENABLED=0 go build -trimpath -tags embed -o casos .
```

`GENERATE_SOURCEMAP=false` keeps the `.map` files out of the binary; without it
they are embedded as well and add tens of megabytes.

Every release publishes this binary for Linux, macOS, and Windows on x86_64 on the
[releases page](https://github.com/casosorg/casos/releases/latest). Each one
ships as an archive — `.tar.gz` for Linux and macOS, `.zip` for Windows —
holding the executable alone; compressing a statically linked Kubernetes
control plane cuts the download to roughly a third.

### Lint

```bash
cd web

yarn lint:js    # ESLint
yarn lint:css   # Stylelint
yarn lint       # both
```

## License

[Apache 2.0](https://github.com/casosorg/casos/blob/master/LICENSE)
