const os = require("os");
const path = require("path");
const {defineConfig, devices} = require("@playwright/test");

const localE2ETestToken = "local-e2e-token";
const backendPort = Number(process.env.E2E_BACKEND_PORT || 9000);
const frontendPort = Number(process.env.E2E_FRONTEND_PORT || 8001);
const baseURL = `http://127.0.0.1:${frontendPort}`;
const backendURL = `http://127.0.0.1:${backendPort}`;
const backendHealthPath = process.env.E2E_HEALTH_CHECK_PATH || "/api/get-built-in-site";
const e2eToken = process.env.E2E_TEST_TOKEN || localE2ETestToken;
const e2eDataDir = process.env.E2E_DATA_DIR || path.join(os.tmpdir(), `casos-e2e-${process.pid}`);
const backendDir = path.resolve(__dirname, "..");

module.exports = defineConfig({
  testDir: "./tests/ui",
  outputDir: "test-results",
  timeout: 30 * 1000,
  expect: {
    timeout: 10 * 1000,
  },
  fullyParallel: true,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [["github"], ["html", {open: "never"}]] : "list",
  use: {
    baseURL,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    video: "retain-on-failure",
    viewport: {width: 1280, height: 720},
  },
  projects: [
    {
      name: "chromium",
      use: {...devices["Desktop Chrome"]},
    },
  ],
  webServer: [
    {
      command: "go run main.go -createDatabase=true",
      cwd: backendDir,
      url: `${backendURL}${backendHealthPath}`,
      reuseExistingServer: !process.env.CI,
      timeout: 180 * 1000,
      env: {
        ...process.env,
        httpport: String(backendPort),
        dataDir: e2eDataDir,
        apiserverPort: process.env.E2E_APISERVER_PORT || "16443",
        webhookPort: process.env.E2E_WEBHOOK_PORT || "19443",
        socks5Proxy: process.env.socks5Proxy || "",
        e2eTestMode: "true",
        e2eTestToken: e2eToken,
      },
    },
    {
      command: "yarn start",
      url: baseURL,
      reuseExistingServer: !process.env.CI,
      timeout: 120 * 1000,
      env: {
        ...process.env,
        BROWSER: "none",
        CI: "false",
        PORT: String(frontendPort),
      },
    },
  ],
});
