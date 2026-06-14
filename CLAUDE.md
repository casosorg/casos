# casos

A Kubernetes management platform with a Go backend and React frontend.

## Project Structure

- `main.go` — entry point
- `controllers/` — HTTP controllers (Beego routing)
- `object/` — business logic and data models
- `routers/` — route configuration
- `proxy/` — kube-proxy related logic
- `conf/app.conf` — backend configuration
- `web/` — React frontend (see below)

## Frontend (web/)

Stack: React 18 + Ant Design 6, bootstrapped with Create React App.

**Package manager: yarn.** The lock file is `web/yarn.lock`. Do not use npm or pnpm.

```bash
# Install dependencies
cd web && yarn install

# Start dev server (port 8001)
cd web && yarn start

# Production build
cd web && yarn build
```

Registry is set to yarnpkg. Configure it via:

```bash
yarn config set registry https://registry.yarnpkg.com
```

or add to `web/.yarnrc`:

```
registry "https://registry.yarnpkg.com"
```

The dev server proxies API requests to the backend at `http://localhost:9000` (configured via the `proxy` field in `web/package.json`).

## Backend

Language: Go 1.26, module path `github.com/casosorg/casos`.

```bash
# Run
go run main.go

# Build
go build -o casos.exe .
```

## Notes

- All yarn/frontend commands must be run inside `web/`, not the repo root.
- Do not commit `web/node_modules/`.
