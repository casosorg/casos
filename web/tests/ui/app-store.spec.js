const {expect, test} = require("@playwright/test");
const {signInAsCiUser} = require("./e2e-helpers");
const {
  addCustomHelmRepo,
  addedHelmReposFixture,
  getServiceAccessUrl,
  installAppFromAppStore,
  installedReleasesFixture,
  makeReleaseName,
  makeRepoName,
  waitForAppContent,
} = require("./app-store-helpers");

const appStoreTest = test.extend({
  installedReleases: installedReleasesFixture,
  addedHelmRepos: addedHelmReposFixture,
});
appStoreTest.describe.configure({mode: "serial", retries: 0});

const E2E_NAMESPACE = "default";
const APP_STORE_INSTALL_TIMEOUT_MS = Number(process.env.E2E_APP_INSTALL_TIMEOUT_MS) || 15 * 60 * 1000;
// Official OCI-hosted chart for Casdoor, the identity/SSO provider this project itself
// authenticates against (see controllers/e2e.go's casdoorsdk usage).
const CASDOOR_OCI_REPO_URL = "oci://registry-1.docker.io/casbin/casdoor-helm-charts";
const CASDOOR_CHART_NAME = "casdoor-helm-charts";

function nodePortValues(releaseName) {
  return `fullnameOverride: ${releaseName}\nservice:\n  type: NodePort\n`;
}

function casdoorValues(releaseName) {
  return `${nodePortValues(releaseName)}config: |
  appname = casdoor
  httpport = {{ .Values.service.port }}
  runmode = dev
  SessionOn = true
  copyrequestbody = true
  driverName = sqlite
  dataSourceName = /tmp/casdoor.db
  dbName = {{ include "casdoor.dbName" . }}
  redisEndpoint =
  defaultStorageProvider =
  isCloudIntranet = false
  authState = "casdoor"
  socks5Proxy = ""
  verificationCodeTimeout = 10
  initScore = 0
  logPostOnly = true
  origin =
  enableGzip = true
  ldapServerPort = 10389
  initDataFile = ""
`;
}

appStoreTest.beforeEach(async({page}) => {
  await signInAsCiUser(page);
});

appStoreTest("installs nginx from the App Store and serves its default page via the access URL", async({page, request, installedReleases}) => {
  appStoreTest.setTimeout(APP_STORE_INSTALL_TIMEOUT_MS);
  const releaseName = makeReleaseName("e2e-nginx");

  await installAppFromAppStore(page, {
    repoName: "Bitnami",
    chartName: "nginx",
    releaseName,
    namespace: E2E_NAMESPACE,
    valuesYAML: nodePortValues(releaseName),
    installedReleases,
  });

  const accessUrl = await getServiceAccessUrl(page, E2E_NAMESPACE, releaseName);
  expect(accessUrl).toMatch(/^http:\/\/[^/]+:\d+$/);

  await waitForAppContent(request, accessUrl, "Welcome to nginx!");
});

appStoreTest("installs Casdoor from the App Store and serves its login page via the access URL", async({page, request, installedReleases, addedHelmRepos}) => {
  appStoreTest.setTimeout(APP_STORE_INSTALL_TIMEOUT_MS);
  const releaseName = makeReleaseName("e2e-casdoor");
  const repoName = makeRepoName("casdoor");

  await addCustomHelmRepo(page, {
    name: repoName,
    url: CASDOOR_OCI_REPO_URL,
    addedHelmRepos,
  });

  await installAppFromAppStore(page, {
    repoName,
    chartName: CASDOOR_CHART_NAME,
    releaseName,
    namespace: E2E_NAMESPACE,
    valuesYAML: casdoorValues(releaseName),
    installedReleases,
  });

  const accessUrl = await getServiceAccessUrl(page, E2E_NAMESPACE, releaseName);
  expect(accessUrl).toMatch(/^http:\/\/[^/]+:\d+$/);

  await waitForAppContent(request, accessUrl, "<title>Casdoor</title>");
});
