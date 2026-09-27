<div align="center">

# CasOS

### 一个文件，拥有你自己的云：跑应用、跑大模型、给 AI Agent 用。

下载一个文件，双击运行。你的笔记本、游戏电脑或服务器上就有了一个真正的 Kubernetes
集群，自带 Web 界面和应用商店。Windows 也能用。不用写 YAML，也不需要现成的集群。

[![Build](https://github.com/casosorg/casos/workflows/Build/badge.svg?style=flat-square)](https://github.com/casosorg/casos/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/v/release/casosorg/casos?style=flat-square&color=4f46e5)](https://github.com/casosorg/casos/releases/latest)
[![Go Report](https://goreportcard.com/badge/github.com/casosorg/casos?style=flat-square)](https://goreportcard.com/report/github.com/casosorg/casos)
[![License](https://img.shields.io/github/license/casosorg/casos?style=flat-square&color=22c55e)](https://github.com/casosorg/casos/blob/master/LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-blue?style=flat-square)](https://github.com/casosorg/casos/releases/latest)
[![Discord](https://img.shields.io/discord/1022748306096537660?logo=discord&label=discord&color=5865F2&style=flat-square)](https://discord.gg/6ma4BAmV7P)

**[官网](https://www.casos.net) · [在线演示](https://demo.casos.net) · [快速开始](#快速开始)**

**[English](README.md) | 简体中文**

</div>

---

| | |
|---|---|
| 🚀 **一个文件，一分钟** | Kubernetes API Server、Controller Manager 和 Scheduler 都内嵌在一个可执行文件里。运行它的机器会自动成为工作节点。在 Windows 上，节点运行在 WSL 里；没有 WSL 的话 CasOS 会自动安装。 |
| 🧠 **用自己的显卡跑私有 AI** | 一键装好 Ollama 和 Open WebUI，并按显存自动挑选能装下的最大模型，比如 Qwen 3、DeepSeek R1、Gemma 3 或 gpt-oss。模型跑在你的 NVIDIA 显卡上，数据不出本机。局域网里的应用和编辑器可以通过 `/v1` 上的 OpenAI 兼容 API 调用它，请求会分摊到所有跑着这个模型的显卡机器上。 |
| 🤖 **AI Agent 能直接操作的云** | 内置 MCP Server，Claude Code、Cursor、Codex 可以直接部署应用、查看日志、回滚版本。Agent 还能开沙箱：克隆代码、执行命令、跑 GPU 任务。沙箱到期自动删除，你也可以随时用 VS Code 打开，旁观或接手。一切都跑在你自己的机器上。 |
| 🛠️ **带 GPU 的开发环境** | DevBox 提供浏览器里的 VS Code，也支持桌面版 VS Code 通过 SSH 连接。可以从零开始、从 Git 地址或本机仓库、在任意镜像里启动，并在 GPU 上跑任务。 |
| 📦 **开箱即满的应用商店** | 直接安装 sealos 模板市场和 Helm Chart 里的应用。托管数据库自带备份和控制台。 |
| 🚢 **从 Git 部署** | 填一个公开仓库，casos 在集群里完成构建：有 Dockerfile 就用它，没有的话按 Node.js、Python、Go 或静态网站自动构建。构建完直接部署并分配访问地址。不用自己搭镜像仓库，重新构建就能上线最新提交。 |
| 🐳 **粘贴 docker-compose.yml 直接部署** | 大多数自托管应用都自带 compose 文件，casos 可以原样跑起来。每个服务运行在项目自己的命名空间里，相互之间按服务名访问。发布了端口的服务会分配访问地址，`${VAR}` 从 `.env` 输入框取值，GPU 预留会用上你的 NVIDIA 显卡。部署前的预览会说明哪些配置没法照搬、会被怎样处理。 |

**一条命令接入你的 AI Agent：**

```bash
claude mcp add --transport http --scope user casos http://localhost:20080/mcp --header "Authorization: Bearer <token>"
```

打开右上角账号菜单 → **AI Agents** 创建 token。那个页面也有 Cursor、Codex 和其他
MCP 客户端的现成配置。

**和其他项目的区别。** k3s、minikube 只给你一个集群；CasOS 在此之上还带界面、应用
商店、私有 AI 和 Agent 接入，并且能在 Windows 上原生运行。Coolify、CasaOS 在 Docker
上提供一键应用；CasOS 提供同样的体验，但底层是真正的 Kubernetes，通过 SSH 添加机器
就能扩展。

## 快速开始

三步，大约五分钟。无需任何 Kubernetes 基础。

### 1. 安装

**Linux / macOS**

```bash
curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | bash
```

**Windows（PowerShell）**

```powershell
irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1 | iex
```

安装脚本会下载最新版本，解压出可执行文件并加入到你的用户 `PATH` 中。在 Windows 上，
当前 PowerShell 会话可以立即使用；在 Linux 和 macOS 上，请打开一个新的 shell（或执行
安装脚本打印出的 `source` 命令）。不想运行脚本？从
[发布页面](https://github.com/casosorg/casos/releases/latest)下载对应系统的压缩包并解压
即可——里面只有一个可执行文件，别无其他。

### 2. 启动

```bash
casos
```

在 Windows 上你也可以直接**双击 `casos.exe`**——它会启动服务并自动为你打开浏览器。

### 3. 打开界面

访问 **<http://localhost:20080>**，你已经以 `admin` 身份登录。

> ⚠️ **请先修改密码。** 全新安装带有内置账号 `admin` / `123` 并会自动登录，这意味着
> *任何*能访问该端口的人都是管理员。点击右上角的账号菜单 → **我的账号** → 设置一个真
> 正的密码。一旦密码不再是 `123`，自动登录就会停止。

### 4. 安装第一个应用

打开**应用商店**，安装任何你喜欢的应用。整个流程就是这样——CasOS 会拉取 Chart、完成
调度，并把运行中的应用展示给你。

仪表盘上有一个**首次运行清单**，会跟踪上述步骤，四项全部完成后自动消失。每一步的状态
都从服务端读取，所以刷新页面或换一个浏览器登录后依然准确。

### CasOS 自动完成的工作

你不需要添加节点、安装 CNI 或配置存储。启动时 CasOS 会：

- **把运行它的这台机器变成工作节点。** 在 Linux 上就是主机本身；在 Windows 上使用你的
  WSL 发行版——如果没有，它会为你安装 WSL。可以在**机器**页面查看进度。
- 启动 **Ingress 控制器**和**服务负载均衡器**，让应用获得可访问的地址。
- 把所有数据存放在数据目录下的 **SQLite** 中——无需搭建数据库。

| 主机 | 工作节点 |
|------|---------|
| Linux | 主机本身。CasOS 必须以 root 运行，或以拥有免密 `sudo` 的用户运行。 |
| Windows | 本地的 WSL2 发行版，缺失时会自动安装。全新安装的 WSL 可能需要重启一次 Windows。 |
| macOS | 不支持——kubelet 需要 Linux 内核。请在**机器**页面通过 SSH 添加一台 Linux 机器，或在 Linux 虚拟机中运行 CasOS。 |

### 出问题了怎么办

| 现象 | 处理方式 |
|------|---------|
| 页面打不开 | 可能有其他程序占用了 20080 端口。修改 `conf/app.conf` 中的 `httpport`。 |
| 集群里没有节点 | 在 CasOS 日志中查找 `automatic node setup`，按它的提示修复，或在**机器**页面手动添加机器。 |
| macOS 拒绝运行该二进制 | 发布版未经过公证。通过浏览器下载的文件会被隔离：`xattr -d com.apple.quarantine ./casos`。使用上面的安装脚本则不受影响。 |
| 忘记管理员密码 | 无法找回。删除 `data/casos.db` 中 `user` 表的对应记录并重启，即可恢复默认账号。 |
| 提交 Bug | 请附上 `casos --version` 的输出。发布版二进制会打印标签、提交号和构建日期；自行编译的版本显示 `dev`。 |

### 升级与卸载

重新执行安装命令即可升级。卸载：

```bash
curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | bash -s -- --uninstall
```

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1))) -Uninstall
```

你的数据目录**不会**被删除。它保存着 SQLite 数据库、控制平面的 TLS 材料，以及用于解密
已保存的 SSH 凭据的密钥，因此删除它是一个独立且需要慎重决定的动作——卸载程序会打印出
该目录的路径和对应的删除命令。

<details>
<summary><b>安装选项</b> —— 指定版本，或选择安装目录</summary>

两个安装脚本读取相同的配置项：

| 变量 | 默认值 | 用途 |
|---|---|---|
| `CASOS_VERSION` | `latest` | 版本标签，例如 `v1.32.0` |
| `INSTALL_DIR` | `$HOME/.local/bin`、`%LOCALAPPDATA%\CasOS\bin` | 安装到哪个目录 |
| `CASOS_REPOSITORY` | `casosorg/casos` | 从哪个发布仓库拉取 |

在 Linux 和 macOS 上，把它们传给 `bash`，而不是 `curl`：

```bash
curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | CASOS_VERSION=v1.32.0 INSTALL_DIR="$HOME/.local/bin" bash
```

PowerShell 没有等价的前缀语法。`iex` 在当前会话中运行安装脚本，所以要先用单独的
`$env:` 语句设置变量：

```powershell
$env:CASOS_VERSION = 'v1.32.0'
irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1 | iex
```

这些变量在安装结束后依然存在，会让同一窗口中后续的安装也被固定到该版本，所以不再需要
时请清除它们：

```powershell
Remove-Item Env:\CASOS_VERSION
```

发布版本仅提供 x86_64 构建。Apple Silicon 的 Mac 和 ARM 版 Windows 都会拿到该构建并通过
模拟运行；arm64 的 Linux 主机则需要[从源码构建](#开发)。

</details>

## 功能特性

- **内嵌 Kubernetes** —— API Server、Controller Manager 和 Scheduler 都在同一个二进制
  文件里。不需要外部集群，也不需要 `kubeadm`。
- **零配置工作节点** —— 启动 CasOS 的这台机器会自行加入集群；更多机器可以在**机器**页面
  通过 SSH 添加。
- **应用商店** —— 在界面上安装和管理 Helm Release。
- **完整的资源管理** —— Deployment、StatefulSet、DaemonSet、Job、CronJob、Pod、Service、
  Ingress、ConfigMap、Secret、PVC、StorageClass、HPA、配额、命名空间和 RBAC。
- **可观测性** —— 仪表盘、集群拓扑、监控、日志搜索，以及浏览器中的 Pod 终端。
- **安全** —— 基于 Casbin 的准入与授权策略，以及 Trivy 镜像扫描。
- **开箱即用的登录** —— 内置 `admin` 账号，也可选接入 [Casdoor](https://casdoor.org)
  单点登录。
- **多语言界面**（i18n）。

## 配置

大多数人永远用不到这一节：CasOS 使用默认配置即可运行，而全新安装最可能需要修改的设置
就是 HTTP 端口。

配置文件位于可执行文件旁边的 `conf/app.conf`：

```ini
httpport      = 20080      ; Web 界面和 REST API
apiserverPort = 20443      ; 内嵌的 Kubernetes API Server
dataDir       = ./data     ; 数据库、TLS 材料、凭据密钥

driverName    = sqlite     ; 默认值；同时支持 "mysql"
casdoorEndpoint =          ; 留空 = 使用内置 admin 账号
autoEnrollLocalNode = true ; 启动时把本机变成工作节点
```

**没有** `conf/app.conf` 时启动的二进制会把数据存放到用户级数据目录：
`~/.local/share/casos`、`~/Library/Application Support/CasOS` 或
`%LOCALAPPDATA%\CasOS`。CasOS 会在启动时以
`casos data directory: <path>` 的形式打印它解析出的目录。

### 登录方式

`casdoorEndpoint` 决定你如何登录，除此之外无需任何配置：

| `casdoorEndpoint` | 登录方式 |
|---|---|
| 留空（默认） | 内置 `admin` 账号，保存在 CasOS 数据库中 |
| 已设置 | [Casdoor](https://casdoor.org) 单点登录；不会创建内置账号 |

要使用 Casdoor，请在首次启动前填好全部五项 Casdoor 配置。每一项都可以通过同名的环境
变量读取，因此 `clientSecret` 不必写入磁盘：

```bash
casdoorEndpoint=https://your-casdoor-instance clientId=... clientSecret=... casdoorOrganization=... casdoorApplication=... casos
```

<details>
<summary><b>数据目录、MySQL、出站代理、手动节点、升级提示</b></summary>

**数据目录。** SQLite 把 CasOS 的业务数据存放在 `data/casos.db`，把 Kubernetes 状态存放
在 `data/kine/state.db`。该目录还保存着控制平面的 TLS 材料，以及加密已保存的 SSH 私钥
所用的密钥，因此一旦丢失该目录，安装实例就再也无法解密现有的机器凭据。

> **破坏性变更：** `dataDir` 过去默认为 `/var/lib/casos`，现在默认为 `./data`，它是相对
> CasOS 进程的工作目录解析的——因此从不同目录启动 CasOS 会指向不同的、且很可能是空的
> 数据目录。任何系统级安装都请把 `dataDir` 设置为绝对路径，例如 `/var/lib/casos`，确保
> 服务用户对其有写权限，并在重启前把已有的 `/var/lib/casos` 迁移到新位置。

**MySQL。** 设置 `driverName=mysql`，配置 `dataSourceName` 和 `dbName`，并可选地设置完整
的 `kineEndpoint`。把已有安装从 MySQL 切换回 SQLite 会从空的 SQLite 数据库开始；已有的
MySQL 数据不会自动导入。

**出站代理。** `socks5Proxy` 接受 `host:port`、`socks5://` 和 `socks5h://` 形式的地址。
设置该项后，对于无法走所配置代理的请求，CasOS 会直接失败，而不会悄悄回退到直连；被
CasOS 进程的 `NO_PROXY` 匹配到的目标仍然走直连。该项留空时，CasOS 会遵循 `HTTP_PROXY`、
`HTTPS_PROXY` 和 `NO_PROXY`，在没有配置环境代理时使用直连。

> **升级提示：** 此前示例中的默认值 `127.0.0.1:10808` 已被移除。如果该本地代理是控制平面
> 必需的依赖，请在升级前显式设置 `socks5Proxy`。

> **升级提示：** 早期版本的 `conf/app.conf` 指向一个公共的 Casdoor 演示应用，且包含了
> 凭据。这些值现在均为空。依赖它们的安装实例必须配置自己的 Casdoor 应用，或者清空这五项
> 配置以切换到内置账号。

**手动添加工作节点。** 设置 `autoEnrollLocalNode = false` 可以自行管理节点。
[`worker-setup.md`](worker-setup.md) 介绍了如何手动搭建 WSL2 工作节点，
[`machine-setup.md`](machine-setup.md) 介绍了如何准备一台机器以便通过 SSH 纳管。

</details>

## 开发

前置条件：[Go](https://golang.org/dl/) 1.26+、[Node.js](https://nodejs.org/) 20+ 和
[Yarn](https://classic.yarnpkg.com/) 1.x。所有前端命令都在 `web/` 目录下执行，包管理器
是 **yarn**——锁文件为 `web/yarn.lock`。

```bash
git clone https://github.com/casosorg/casos.git
cd casos
```

**后端** —— 在 20080 端口提供 API 和界面：

```bash
go run main.go
```

**前端** —— 开发服务器运行在 8002 端口，把 API 代理到 `localhost:20080`
（可用 `BACKEND_URL` 覆盖）：

```bash
cd web
yarn install    # 仅首次需要
yarn start
```

**测试与代码检查：**

```bash
cd web
yarn lint          # 带 --fix 的 ESLint
yarn lint:ci       # 同样的检查但不带 --fix，与 CI 一致
yarn test:unit     # 单元测试
yarn ui:test       # Playwright 端到端测试；会自行启动后端和开发服务器
```

组件约定，以及 Playwright 测试所依赖的选择器钩子，见
[`web/FRONTEND.md`](web/FRONTEND.md)。

### 构建发布用二进制

后端会从磁盘读取 `web/build/`，所以开发时只重新构建前端就够了。要生成发布版所提供的那个
自包含的单文件，请先构建前端，再用 `-tags embed` 编译：

```bash
cd web && yarn build && cd ..
CGO_ENABLED=0 go build -trimpath -tags embed -o casos .
```

每个发布版本都会为 x86_64 架构的 Linux、macOS 和 Windows 提供该二进制，打包为
`.tar.gz`（Linux、macOS）或 `.zip`（Windows），其中只包含这一个可执行文件。

### 项目结构

```
casos/
├── main.go          # 入口
├── conf/app.conf    # 后端配置
├── controllers/     # HTTP 控制器（Beego 路由）
├── object/          # 业务逻辑与数据模型
├── routers/         # 路由配置与过滤器
├── server/          # 内嵌的 Kubernetes 控制平面
├── deploy/          # 节点部署与本地节点引导
├── store/           # Helm 与应用商店逻辑
├── proxy/           # kube-proxy 相关逻辑
├── i18n/            # 后端翻译
└── web/             # React 前端
    └── src/
        ├── pages/       # 每个页面一个文件
        ├── components/  # shadcn/ui 组件与共享控件
        ├── backend/     # API 客户端辅助方法
        ├── hooks/ lib/ locales/
        └── routes.jsx
```

| 层次 | 技术 |
|---|---|
| 后端 | Go 1.26+、Beego、内嵌 Kubernetes、通过 kine 使用 SQLite（可选 MySQL） |
| 前端 | React 18、shadcn/ui（Radix + Tailwind v4）、Vite、recharts、i18next |
| 认证 | 内置账号，或 Casdoor（OAuth2 / OIDC） |

## 许可证

[Apache 2.0](https://github.com/casosorg/casos/blob/master/LICENSE)
