# Frontend notes

`web/` is the React frontend: shadcn/ui (Radix + Tailwind v4) on Vite, talking to
the Beego backend. It replaced an Ant Design frontend that used to live in the
same directory; the antd mentions left in the source are comments naming what
each component replaces.

## Stack

| | |
|---|---|
| Build | Vite 6 |
| UI kit | shadcn/ui (Radix + Tailwind v4) |
| Icons | lucide-react |
| Tables | `@tanstack/react-table` via `DataTable` |
| Toasts | sonner, behind `Setting.showMessage` |
| Router | react-router-dom 5 |
| Language | JavaScript (JSX) |

## API clients

`src/backend/*.js` are 32 plain `fetch` wrappers with no UI dependency. They
carry the whole backend contract, so treat a change there as a backend change.
`src/i18n.js` and `src/locales/{en,zh}/data.json` hold the translations; the Go
side reads the same JSON (see `i18n/`).

## Dev loop

```bash
cd web && yarn install && yarn start
```

Vite serves on **8002** and proxies `/api`, `/k8s` and `/.well-known` to the Go
backend on `:20080`, websockets included (the pod terminal needs that). Because
the proxy handles it, `Setting.ServerUrl` stays empty and every request is
relative — the same code path used in production, where the backend serves the
built bundle itself.

## Component contract

Written against these, not against raw Radix. Page code should rarely import
from `components/ui/` directly except for `Button`, `Input`, `Badge`.

### `DataTable` — `components/shared/data-table.jsx`

The list-page workhorse. Columns are declared as:

```js
{key, title, dataIndex, render(value, record, index), width, minWidth, align,
 sortable, ellipsis, className, headerClassName}
```

`dataIndex` may be a dotted path; a column without one is a display column
(actions) and cannot sort. `width` pins a column; `minWidth` only stops it
being squeezed, which is what an `ellipsis` column needs so it clips at a
readable size instead of collapsing between fixed-width neighbours. An
`ellipsis` cell also exposes its raw value as a hover tooltip.

Table props: `dataSource`, `rowKey` (string or function), `loading`, `title`,
`description`, `toolbar`, `searchable`, `pageSize` (0 disables pagination),
`emptyText`, `onRowClick`, `expandable`, `dense`. Pagination only renders once
the rows overflow a page.

### Dialogs

- `FormDialog` + `Field` — create/edit modals. `FormDialog` owns the chrome and
  submit state; `Field` is label + control + error text.
- `ConfirmDialog` — replaces antd `Popconfirm`. Trigger is passed as children;
  the confirm button is destructive by default.
- `ResourceSheet` — replaces antd `Drawer` (logs, terminal, files, history).

### Data fetching — `hooks/use-resource.js`

```js
const {data, loading, error, refresh} = useResource(
  () => PodBackend.getPods(), [], {initialData: []});
```

Handles the `{status, data, msg}` envelope, toasts failures, and drops responses
that arrive after unmount or after a newer request. `runAction(promise, {...})`
is the mutation counterpart and resolves to a boolean so callers close a dialog
only on success.

### Other shared pieces

`StatusBadge` / `ReadyBadge` / `ColorTag` (`tagVariant()` maps antd colour names
to badge variants), `KeyValueEditor` + `StringListEditor` (+ `toEntries` /
`fromEntries`), `EnvVarEditor`, `RestartDeploymentsDialog`, `SimpleSelect` /
`SearchSelect`, `NumberInput`, `PasswordInput`, `StatCard`, `EmptyState`,
`Loading` / `AiDots`, `CodeText` / `CodeBlock` / `CopyButton`,
`DescriptionList`, `ResultScreen`, `Space`, `PageContainer` / `PageHeader`.

### Navigation

`src/nav.js` is the single description of the sidebar tree, read by both
`AppSidebar` and `BreadcrumbBar`. The old UI kept that mapping in two places and
they drifted. `src/routes.jsx` holds the route table; every page is lazy-loaded.

Simple mode's pages live under `/simple`, advanced mode's at the plain paths, so
no URL means two different screens. `useUiMode` reads the mode off the URL rather
than off storage: the stored `uiMode` is only a preference deciding where `/`
lands. A page both modes render links through `resolvePath` so a simple-mode
reader is not bounced into the advanced surface; `nav.js` holds the pairing that
also decides where the header's mode switch lands.

## App Launchpad — `/launchpad`

The launchpad turns one container image into a running application: how much it
may use, what it runs, the files and credentials it needs, how it is reached and
how it scales. It manages the same objects the App Store's Docker Hub install
creates (everything labelled `app.kubernetes.io/managed-by: casos`), so an app
installed from the store can be edited here, and an app deployed here shows up
under Installed Apps.

| File | What it holds |
|---|---|
| `lib/launchpad.js` | The form model: read from the cluster, edited, previewed as YAML, sent back |
| `pages/LaunchpadPage.jsx` | The list, with live CPU and memory per app |
| `pages/LaunchpadEditPage.jsx` | Create and edit — the same form, with the manifest preview beside it |
| `pages/LaunchpadDetailPage.jsx` | One app: usage over time, addresses, storage, pods with logs/terminal/events |

Backend: `POST /api/deploy-app` and `POST /api/upgrade-image-app` take the same
payload, and `GET /api/get-image-app` reads an app back as the form that
produced it. `controllers/launchpad.go` holds the reconcilers for the objects an
app owns besides its workload — its ConfigMap of config files, its registry
secret, its autoscaler and its Ingress.

**Absent means "leave alone".** `configFiles`, `domains`, `hpa` and `registry`
are pointers on the Go side: a payload that omits one keeps whatever the app
already has, and an empty list removes it. That is what lets the App Store's
upgrade — which knows nothing about domains or autoscaling — leave the
launchpad's work intact. The launchpad form always states all four.

Usage series on the detail page are collected in the page from `/api/get-metrics`
polls; the cluster only reports the present, so the chart starts empty and fills
in rather than inventing history.

## Databases — `/databases`

A managed database is a StatefulSet, a Service, a Secret holding its
credentials and two claims — one for its data, one for its backups. There is no
database operator involved: the engines are the stock images, and everything the
pages offer is done to those objects directly.

| File | What it holds |
|---|---|
| `lib/database.js` | Engine tints and the form model shared by create and edit |
| `pages/DatabasePage.jsx` | The list, with per-database state and start/stop |
| `pages/DatabaseEditPage.jsx` | Engine, version, credentials, size, public access |
| `pages/DatabaseDetailPage.jsx` | Connection details, backups, pods, and the engine console |

Backend: `controllers/database_engine.go` is the catalogue — for each engine the
image, how the Secret reaches the container, and the four shell commands that
make it a database rather than a container (console, dump, restore, connection
URI). `controllers/database.go` is the CRUD and the actions around it.

**Backups run inside the database pod.** The engine dumps itself into the backup
claim, which the pod also mounts, so listing and downloading a backup are just
the pod-file endpoints that already exist and no backup operator is needed. That
is also why backups and the console are unavailable while a database is stopped,
and the pages say so rather than failing.

**The console is the engine's own client**, not a shell: `/api/database-console`
runs `psql`, `mysql`, `mongosh` or `redis-cli` already signed in.
`PodShell` takes the endpoint as a prop, so the pod terminal and the database
console are the same component over the same two-channel protocol.

Credentials are set once. The engine stores its own user and password at first
start, so rewriting the Secret afterwards would only make it disagree with the
engine — the edit form leaves them out for that reason.

## The desktop — `src/desktop/`

`/desktop` is a third shell alongside simple and advanced mode: a wallpaper, an
icon grid, a dock and floating windows, modelled on the sealos desktop. It is
not a separate application — a window mounts `AppRoutes` under a `MemoryRouter`
of its own, so every page the sidebar UI serves is available inside a window and
the two surfaces cannot drift apart. Each window keeps its own history, which is
what lets two windows sit on different pages at once without touching the
browser address bar.

| File | What it holds |
|---|---|
| `registry.js` | The app catalogue: key, label key, icon, tint, and the route the window opens at |
| `desktop-store.jsx` | The window manager — processes, stacking order, sizes, single-instance launching |
| `app-window.jsx` | Window chrome: drag, resize, minimize/maximize/restore, focus mask |
| `app-frame.jsx` | Window contents: our routes, or an iframe for an installed app's own UI |
| `app-dock.jsx` | The dock, its magnification, and the running indicators |
| `desktop-icons.jsx` | The icon grid and the folder icons drag into |
| `app-launcher.jsx` | Launchpad: every app, searchable, with the pin that puts one on the desktop |
| `desktop-topbar.jsx` | Search, cluster vitals, notifications, account controls |
| `notification-center.jsx` | The bell: cluster warnings and standing conditions, with read state |
| `desktop-tour.jsx` | The first-run spotlight tour |
| `desktop-prefs.js` | What persists in localStorage: arrangement, wallpaper, dock mode and state, tour |
| `use-installed-apps.js` | Installed releases that have an address, as icons |
| `app-sdk.js` | The desktop half of the app SDK: the postMessage channel apps in frames talk over |
| `use-app-sdk.js` | What the desktop answers with — session, language, quota, host config, event bus |

An installed app becomes an icon only once it has a reachable address (the
Ingress or Service its release owns), and opens in an iframe. Apps that refuse
framing still have the window header's open-in-browser button.

The dock has two shapes, switched from the desktop's right-click menu and
forced to the second below 768px: the floating dock, and a task bar across the
bottom edge that a maximized window stops above.

### The app SDK

An app hosted in a window is a separate document, so it learns about the desktop
by asking. `web/public/casos-app-sdk.js` is served at `/casos-app-sdk.js` and
needs no build step:

```html
<script src="https://<casos-host>/casos-app-sdk.js"></script>
<script>
  const session = await casosApp.getSession();
  await casosApp.openApp({appKey: "system-pods"});
</script>
```

`getSession`, `getLanguage`, `getWorkspaceQuota`, `getHostConfig`, `getApps`,
`openApp`, `closeApp`, `showMessage`, and an event bus in both directions. The
wire protocol is the one sealos's client SDK speaks and `window.sealosApp` is
aliased to the same object, so an app written for that desktop runs unchanged.

Only the origins of apps this cluster installed may use the channel — an app's
own origin is the proof the operator put it there. Nothing in the session is
cluster credentials: it carries identity and workspace, not a kubeconfig.

`useUiMode` treats the desktop as a mode: entering it stores `uiMode=desktop`, so
`/` lands there next time, and leaving it returns to whichever page mode the
reader came from (`uiPageMode`).

## End-to-end tests

```bash
cd web && yarn ui:test:selector:check   # unit-tests the changed-path selection
cd web && yarn ui:test:smoke            # @smoke only
cd web && yarn ui:test                  # everything
```

`playwright.config.js` starts both servers itself: the Go backend in
`e2eTestMode` and a Vite dev server on 8002. Override `E2E_FRONTEND_PORT`,
`E2E_BACKEND_PORT`, `E2E_APISERVER_PORT` and `E2E_WEBHOOK_PORT` to run beside a
backend you already have going — the dev server's proxy target follows
`BACKEND_URL`, which the config sets from `E2E_BACKEND_PORT`.

### Selectors are a contract

Tailwind class names are generated output and must never be selected on, so the
suite is written against roles, labels and a small set of deliberate hooks:

| Hook | Where | Replaces |
|---|---|---|
| `data-testid` on `DataTable` (`testId` prop) | machines, sites, services, nodes, worker-node tasks | `.ant-table-wrapper` filtered by text |
| `data-row-key` on every `DataTable` row | `DataTable` | antd's own `data-row-key` |
| `data-loading` on `DataTable` | `DataTable` | `.ant-spin-spinning` |
| `data-variant` on `Alert` | `ui/alert.jsx` | `.ant-alert-error` |
| `data-sonner-toast` | sonner | `.ant-message` |
| `data-testid="management-layout"` | `ManagementPage` | `#parent-area` |
| `data-testid="chart-card"` | `AppStorePage` | `.ant-row .ant-col .ant-card` |

Adding a hook is preferable to selecting on a class; adding one is cheap and
says out loud that a test depends on it.

`findMachineRow` does not walk pagination — `DataTable` filters every loaded
row, so the helper types into the search box instead.

`routes-render.spec.js` walks all 33 routes asserting React did not throw and
something painted; nothing else covers 30 of them, and that is how the
`<Button asChild>` crash described below was found.

## React 18, not 19

shadcn's current component sources assume React 19, where `ref` is an ordinary
prop. This project is on React 18, where it is not, so `Button`, `Input` and
`Textarea` are `forwardRef` components — without that, every Radix `asChild`
trigger (tooltip, dropdown, popover, dialog) silently fails to get its anchor.

`Button` also renders `asChild` as a bare `Slot` around its single child: Slot
accepts exactly one child, and injecting the loading spinner alongside it threw
and took the whole route down. `loading` is therefore only honoured on a real
`<button>`.

## Serving from the Go backend

`routers/static_filter.go` serves `web/build` from disk, so a `yarn build` is
picked up without rebuilding the backend. The directory is looked up relative to
the working directory first and then next to the executable, so a binary copied
out of the repository still finds the frontend shipped beside it.

`-tags embed` compiles the frontend into the binary instead:
`web/assets_embed.go` carries the `//go:embed all:build` directive, so a
standalone build fails to compile unless `yarn build` has run in `web/`.
