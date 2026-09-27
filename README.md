<div align="center">

# CasOS

### Your own cloud in one file: for apps, AI models and AI agents.

Download one file and double-click it. You get a real Kubernetes cluster with a web
UI and an app store, on your laptop, your gaming PC or your server. It works on
Windows too. You need no YAML and no existing cluster.

[![Build](https://github.com/casosorg/casos/workflows/Build/badge.svg?style=flat-square)](https://github.com/casosorg/casos/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/v/release/casosorg/casos?style=flat-square&color=4f46e5)](https://github.com/casosorg/casos/releases/latest)
[![Go Report](https://goreportcard.com/badge/github.com/casosorg/casos?style=flat-square)](https://goreportcard.com/report/github.com/casosorg/casos)
[![License](https://img.shields.io/github/license/casosorg/casos?style=flat-square&color=22c55e)](https://github.com/casosorg/casos/blob/master/LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-blue?style=flat-square)](https://github.com/casosorg/casos/releases/latest)
[![Discord](https://img.shields.io/discord/1022748306096537660?logo=discord&label=discord&color=5865F2&style=flat-square)](https://discord.gg/6ma4BAmV7P)

**[Website](https://www.casos.net) · [Live demo](https://demo.casos.net) · [Quick start](#quick-start)**

**English | [简体中文](README_zh.md)**

</div>

---

| | |
|---|---|
| 🚀 **One file, one minute** | The Kubernetes API server, controller manager and scheduler are embedded in a single binary. The machine you start it on becomes a worker node. On Windows that node runs in WSL, and CasOS installs WSL for you if it is missing. |
| 🧠 **Private AI on your own GPU** | One click installs Ollama and Open WebUI and picks the largest model that fits your GPU memory, such as Qwen 3, DeepSeek R1, Gemma 3 or gpt-oss. It runs on your NVIDIA card, and nothing leaves the machine. Apps and editors on your network can use it through an OpenAI-compatible API at `/v1`, spread across every GPU machine that runs the model. |
| 🤖 **A cloud your AI agents can drive** | The built-in MCP server lets Claude Code, Cursor and Codex deploy apps, read their logs and roll them back. Agents can also open sandboxes to clone code, run commands and start GPU jobs. Each sandbox is deleted when its time runs out, and you can open it in VS Code to watch or take over. Everything runs on your hardware. |
| 🛠️ **Dev environments with a GPU** | DevBoxes give you VS Code in the browser, or desktop VS Code over SSH. Start one from scratch, from a Git URL, or from a repository on your own machine, in any image, and run jobs on the GPU. |
| 📦 **An app store that's already full** | Install apps from the sealos template catalogue and Helm charts. Managed databases come with backups and a console. |
| 🚢 **Deploy from Git** | Give it a public repository and casos builds it in the cluster, from its Dockerfile or, without one, as a Node.js, Python, Go or static project. It deploys the result with an address. There is no registry to set up, and a rebuild ships the latest commit. |
| 🐳 **Paste a docker-compose.yml** | Most self-hosted apps ship one, and casos runs it as it is. Every service starts in the project's own namespace and reaches the others by name. Services that publish a port get an address, `${VAR}` values come from a `.env` box, and GPU reservations use your NVIDIA card. A preview shows what anything casos cannot carry over becomes before you deploy. |

**Connect your AI agent with one command:**

```bash
claude mcp add --transport http --scope user casos http://localhost:20080/mcp --header "Authorization: Bearer <token>"
```

To create the token, open the account menu → **AI Agents**. That page also has
ready-made snippets for Cursor, Codex and any other MCP client.

**How it compares.** k3s and minikube give you a cluster. CasOS adds the UI, the app
store, private AI and agent access, and it runs natively on Windows. Coolify and
CasaOS give you one-click apps on Docker. CasOS gives you the same experience on
real Kubernetes, so you can grow it by adding machines over SSH.

## Quick start

Three steps and about five minutes. No prior Kubernetes knowledge needed.

### 1. Install

**Linux / macOS**

```bash
curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | bash
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1 | iex
```

The installer downloads the latest release, unpacks the executable and adds it to
your user `PATH`. On Windows the current PowerShell session can use it right away;
on Linux and macOS, open a new shell (or run the `source` command the installer
prints). Prefer not to run a script? Download the archive for your system from the
[releases page](https://github.com/casosorg/casos/releases/latest) and unpack it —
it holds a single executable, nothing else.

### 2. Start it

```bash
casos
```

On Windows you can also just **double-click `casos.exe`** — it starts the server
and opens your browser for you.

### 3. Open the UI

Go to **<http://localhost:20080>** and you are signed in as `admin`.

> ⚠️ **Change the password first.** A fresh install has the built-in account
> `admin` / `123` and signs you in automatically, which means *anyone* who can
> reach the port is an administrator. Open the account menu in the top-right
> corner → **My Account** → set a real password. The automatic sign-in stops the
> moment the password is no longer `123`.

### 4. Install your first app

Open the **App Store** and install anything you like. That is the whole workflow —
CasOS pulls the chart, schedules it, and shows you the running app.

The dashboard has a **first-run checklist** that tracks these steps and disappears
once all four are done. Every step is read from the server, so it stays accurate
after a refresh or a sign-in from another browser.

### What CasOS sets up by itself

You do not have to add nodes, install a CNI or configure storage. On startup CasOS:

- **Turns the machine it runs on into a worker node.** On Linux that is the host
  itself. On Windows it uses your WSL distribution — and installs WSL for you when
  there is none. Watch the progress on the **Machines** page.
- Starts an **ingress controller** and a **service load balancer**, so apps get a
  reachable address.
- Stores everything in **SQLite** under the data directory — no database to set up.

| Host | Worker node |
|------|-------------|
| Linux | The host itself. CasOS must run as root, or as a user with passwordless `sudo`. |
| Windows | The local WSL2 distribution, installed automatically when missing. A brand-new WSL install can need one Windows restart. |
| macOS | Not possible — a kubelet needs a Linux kernel. Add a Linux machine over SSH on the **Machines** page, or run CasOS inside a Linux VM. |

### If something goes wrong

| Symptom | Fix |
|---------|-----|
| The page does not load | Another program may hold port 20080. Change `httpport` in `conf/app.conf`. |
| The cluster has no node | Look for `automatic node setup` in the CasOS log. Fix what it reports, or add a machine by hand on the **Machines** page. |
| macOS refuses to run the binary | The release is not notarized. A browser download is quarantined: `xattr -d com.apple.quarantine ./casos`. The installer above is unaffected. |
| Lost the admin password | There is no recovery. Delete the row from the `user` table in `data/casos.db` and restart to get the default account back. |
| Reporting a bug | Include the output of `casos --version`. A released binary prints its tag, commit and build date; a self-built one reports `dev`. |

### Upgrade and uninstall

Rerun the install command to upgrade. To uninstall:

```bash
curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | bash -s -- --uninstall
```

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1))) -Uninstall
```

Your data directory is **not** deleted. It holds the SQLite databases, the
control-plane TLS material and the key that decrypts stored SSH credentials, so
removing it is a separate, deliberate step — the uninstaller prints the path and
the exact command for it.

<details>
<summary><b>Installer options</b> — pin a version, or choose the install directory</summary>

Both installers read the same settings:

| Variable | Default | Purpose |
|---|---|---|
| `CASOS_VERSION` | `latest` | Release tag, such as `v1.32.0` |
| `INSTALL_DIR` | `$HOME/.local/bin`, `%LOCALAPPDATA%\CasOS\bin` | Directory to install into |
| `CASOS_REPOSITORY` | `casosorg/casos` | Release repository to pull from |

On Linux and macOS, pass them to `bash`, not to `curl`:

```bash
curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | CASOS_VERSION=v1.32.0 INSTALL_DIR="$HOME/.local/bin" bash
```

PowerShell has no equivalent prefix syntax. `iex` runs the installer inside the
current session, so set the variables as `$env:` entries on their own lines first:

```powershell
$env:CASOS_VERSION = 'v1.32.0'
irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1 | iex
```

Those entries outlive the install and would pin any later run in the same window,
so clear the ones you no longer want:

```powershell
Remove-Item Env:\CASOS_VERSION
```

Releases are x86_64 only. An Apple Silicon Mac and a Windows on ARM machine both
get that build and run it under emulation; an arm64 Linux host has to
[build from source](#development).

</details>

## Features

- **Embedded Kubernetes** — API server, controller manager and scheduler in one
  binary. No external cluster and no `kubeadm`.
- **Zero-configuration worker node** — the machine you start CasOS on joins the
  cluster by itself; more machines are added over SSH from the **Machines** page.
- **App Store** — install and manage Helm releases from the UI.
- **Full resource management** — Deployments, StatefulSets, DaemonSets, Jobs,
  CronJobs, Pods, Services, Ingresses, ConfigMaps, Secrets, PVCs, StorageClasses,
  HPAs, quotas, namespaces and RBAC.
- **Observability** — dashboard, cluster topology, monitoring, log search, and pod
  terminals in the browser.
- **Security** — Casbin-backed admission and authorization policies, plus Trivy
  image scanning.
- **Sign-in that needs no setup** — a built-in `admin` account out of the box, with
  optional [Casdoor](https://casdoor.org) single sign-on.
- **Multi-language UI** (i18n).

## Configuration

Most people never need this section: CasOS runs on its defaults, and the setting a
fresh install is most likely to touch is the HTTP port.

Settings live in `conf/app.conf` next to the executable:

```ini
httpport      = 20080      ; web UI and REST API
apiserverPort = 20443      ; embedded Kubernetes API server
dataDir       = ./data     ; databases, TLS material, credential key

driverName    = sqlite     ; default; "mysql" is also supported
casdoorEndpoint =          ; blank = built-in admin account
autoEnrollLocalNode = true ; make this machine a worker node on startup
```

A binary started **without** `conf/app.conf` stores its data in the per-user data
directory instead: `~/.local/share/casos`, `~/Library/Application Support/CasOS`,
or `%LOCALAPPDATA%\CasOS`. CasOS logs the directory it resolved as
`casos data directory: <path>` on startup.

### Sign-in

`casdoorEndpoint` decides how you sign in, and nothing else has to be set up:

| `casdoorEndpoint` | Sign-in |
|---|---|
| blank (default) | Built-in `admin` account, stored in the CasOS database |
| set | [Casdoor](https://casdoor.org) single sign-on; the built-in account is never created |

To use Casdoor, fill in all five Casdoor settings before the first start. Each one
also reads an environment variable of the same name, so `clientSecret` does not
have to be written to disk:

```bash
casdoorEndpoint=https://your-casdoor-instance clientId=... clientSecret=... casdoorOrganization=... casdoorApplication=... casos
```

<details>
<summary><b>Data directory, MySQL, outbound proxy, manual nodes, upgrade notices</b></summary>

**Data directory.** SQLite stores CasOS business data in `data/casos.db` and
Kubernetes state in `data/kine/state.db`. The directory also holds the
control-plane TLS material and the key that encrypts stored SSH private keys, so an
installation that loses track of it can no longer decrypt existing machine
credentials.

> **Breaking change:** `dataDir` used to default to `/var/lib/casos`. It now
> defaults to `./data`, which is resolved against the working directory of the
> CasOS process — so starting CasOS from a different directory points it at a
> different, and most likely empty, data directory. Set `dataDir` to an absolute
> path such as `/var/lib/casos` for any system installation, make sure the service
> user can write to it, and move an existing `/var/lib/casos` to the new location
> before restarting.

**MySQL.** Set `driverName=mysql`, configure `dataSourceName` and `dbName`, and
optionally set a complete `kineEndpoint`. Switching an existing installation from
MySQL to SQLite starts with empty SQLite databases; existing MySQL data is not
imported automatically.

**Outbound proxy.** `socks5Proxy` accepts `host:port`, `socks5://` and `socks5h://`
addresses. When it is set, CasOS fails requests that cannot use the configured
proxy instead of silently falling back to direct access; destinations matched by
the CasOS process `NO_PROXY` continue to use direct access. When it is blank, CasOS
follows `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY`, and uses direct access when no
environment proxy is configured.

> **Upgrade notice:** the previous example default of `127.0.0.1:10808` has been
> removed. Set `socks5Proxy` explicitly before upgrading if that local proxy is a
> required control-plane dependency.

> **Upgrade notice:** earlier releases shipped `conf/app.conf` pointing at a shared
> public Casdoor demo application, credentials included. Those values are now
> blank. An installation that relied on them must configure its own Casdoor
> application, or clear the five settings to switch to the built-in account.

**Manual worker nodes.** Set `autoEnrollLocalNode = false` to manage nodes
yourself. [`worker-setup.md`](worker-setup.md) walks through building a WSL2 worker
by hand, and [`machine-setup.md`](machine-setup.md) covers preparing a machine for
SSH enrollment.

</details>

## Development

Prerequisites: [Go](https://golang.org/dl/) 1.26+, [Node.js](https://nodejs.org/)
20+ and [Yarn](https://classic.yarnpkg.com/) 1.x. All frontend commands run inside
`web/`, and **yarn** is the package manager — the lock file is `web/yarn.lock`.

```bash
git clone https://github.com/casosorg/casos.git
cd casos
```

**Backend** — serves the API and the UI on port 20080:

```bash
go run main.go
```

**Frontend** — dev server on port 8002, proxying the API to `localhost:20080`
(override with `BACKEND_URL`):

```bash
cd web
yarn install    # first time only
yarn start
```

**Tests and lint:**

```bash
cd web
yarn lint          # ESLint with --fix
yarn lint:ci       # the same check without --fix, as CI runs it
yarn test:unit     # unit tests
yarn ui:test       # Playwright end-to-end suite; starts its own backend and dev server
```

See [`web/FRONTEND.md`](web/FRONTEND.md) for the component contract and the
selector hooks the Playwright suite depends on.

### Building a release binary

The backend reads `web/build/` from disk, so during development a frontend rebuild
alone is enough. To produce the single self-contained file that releases ship,
build the frontend first and compile with `-tags embed`:

```bash
cd web && yarn build && cd ..
CGO_ENABLED=0 go build -trimpath -tags embed -o casos .
```

Every release publishes that binary for Linux, macOS and Windows on x86_64, as a
`.tar.gz` (Linux, macOS) or `.zip` (Windows) holding the executable alone.

### Project structure

```
casos/
├── main.go          # Entry point
├── conf/app.conf    # Backend configuration
├── controllers/     # HTTP controllers (Beego routing)
├── object/          # Business logic and data models
├── routers/         # Route configuration and filters
├── server/          # Embedded Kubernetes control plane
├── deploy/          # Node deployment and the local-node bootstrap
├── store/           # Helm and App Store logic
├── proxy/           # kube-proxy related logic
├── i18n/            # Backend translations
└── web/             # React frontend
    └── src/
        ├── pages/       # One file per screen
        ├── components/  # shadcn/ui components and shared widgets
        ├── backend/     # API client helpers
        ├── hooks/ lib/ locales/
        └── routes.jsx
```

| Layer | Technology |
|---|---|
| Backend | Go 1.26+, Beego, embedded Kubernetes, SQLite via kine (MySQL optional) |
| Frontend | React 18, shadcn/ui (Radix + Tailwind v4), Vite, recharts, i18next |
| Auth | Built-in account, or Casdoor (OAuth2 / OIDC) |

## License

[Apache 2.0](https://github.com/casosorg/casos/blob/master/LICENSE)
