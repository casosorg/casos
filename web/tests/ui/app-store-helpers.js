const {randomUUID} = require("crypto");
const {expect} = require("@playwright/test");
const {expectOkJson} = require("./e2e-helpers");

const API_UNINSTALL_HELM_RELEASE = "/api/uninstall-helm-release";
const API_ADD_HELM_REPO = "/api/add-helm-repo";
const API_GET_HELM_REPOS = "/api/get-helm-repos";
const API_DELETE_HELM_REPO = "/api/delete-helm-repo";
const INSTALL_DONE_TIMEOUT_MS = Number(process.env.E2E_APP_INSTALL_DONE_TIMEOUT_MS) || 11 * 60 * 1000;
const HTTP_CHECK_TIMEOUT_MS = Number(process.env.E2E_APP_HTTP_TIMEOUT_MS) || 180_000;
const HTTP_CHECK_INTERVAL_MS = 3_000;

const installedReleasesFixture = async({page}, use) => {
  const installedReleases = [];
  await use(installedReleases);

  const cleanupErrors = [];
  for (const release of [...installedReleases].reverse()) {
    try {
      const uninstall = await page.context().request.post(API_UNINSTALL_HELM_RELEASE, {
        data: {releaseName: release.name, namespace: release.namespace},
      });
      await expectOkJson(uninstall);
    } catch (error) {
      cleanupErrors.push(`${release.namespace}/${release.name}: ${error.message}`);
    }
  }

  expect(cleanupErrors).toEqual([]);
};

// addedHelmReposFixture deletes any custom Helm repos (see addCustomHelmRepo) added during a
// test, so OCI-hosted charts added just for a test don't linger in "My Repos" afterward.
const addedHelmReposFixture = async({page}, use) => {
  const addedHelmRepos = [];
  await use(addedHelmRepos);

  const cleanupErrors = [];
  for (const repo of [...addedHelmRepos].reverse()) {
    try {
      const deleteRepo = await page.context().request.post(`${API_DELETE_HELM_REPO}?id=${repo.id}`);
      await expectOkJson(deleteRepo);
    } catch (error) {
      cleanupErrors.push(`${repo.name}: ${error.message}`);
    }
  }

  expect(cleanupErrors).toEqual([]);
};

function makeReleaseName(prefix) {
  return `${prefix}-${randomUUID().slice(0, 8)}`;
}

function makeRepoName(prefix) {
  return `${prefix}-${randomUUID().slice(0, 8)}`;
}

function addRepoDialog(page) {
  return page.getByRole("dialog").filter({hasText: "Add Helm Repo"});
}

// addCustomHelmRepo adds a custom repo through the App Store's "Add Repo" flow (the same
// path a user would take to point the App Store at an OCI-hosted chart, e.g.
// "oci://registry-1.docker.io/casbin/casdoor-helm-charts") and records it on addedHelmRepos
// so addedHelmReposFixture can remove it afterward. Returns the persisted repo (with id).
async function addCustomHelmRepo(page, {name, url, addedHelmRepos}) {
  await page.goto("/app-store");
  await expect(page).toHaveURL(/\/app-store$/);

  await page.getByRole("button", {name: "Add Repo"}).click();
  const dialog = addRepoDialog(page);
  await expect(dialog).toBeVisible();
  await dialog.getByLabel("Repo name").fill(name);
  await dialog.getByLabel("Repo URL").fill(url);

  const addRepo = page.waitForResponse(response =>
    response.url().includes(API_ADD_HELM_REPO) && response.request().method() === "POST"
  );
  await dialog.getByRole("button", {name: "OK"}).click();
  await expectOkJson(await addRepo);
  await expect(dialog).toBeHidden();

  const reposResponse = await page.context().request.get(API_GET_HELM_REPOS);
  const reposBody = await expectOkJson(reposResponse);
  const repo = (reposBody.data ?? []).find(r => r.name === name);
  if (!repo) {
    throw new Error(`Custom Helm repo "${name}" was not found after adding it`);
  }

  addedHelmRepos.push(repo);
  return repo;
}

function installModal(page) {
  return page.getByRole("dialog").filter({hasText: "Install chart"});
}

function compactInstallDialogText(text) {
  const trimmed = text.trim();
  if (trimmed.length <= 4000) {
    return trimmed || "No install dialog text was available.";
  }
  return `${trimmed.slice(0, 1800)}\n...\n${trimmed.slice(-1800)}`;
}

async function waitForInstallDone(dialog) {
  const doneButton = dialog.getByRole("button", {name: "Done"});
  const errorAlert = dialog.locator(".ant-alert-error").first();
  const deadline = Date.now() + INSTALL_DONE_TIMEOUT_MS;

  while (Date.now() < deadline) {
    if (await doneButton.isVisible()) {
      return;
    }
    if (await errorAlert.isVisible()) {
      const dialogText = compactInstallDialogText(await dialog.innerText());
      throw new Error(`Helm install failed before completion:\n${dialogText}`);
    }
    await new Promise(resolve => setTimeout(resolve, 1000));
  }

  const dialogText = compactInstallDialogText(await dialog.innerText());
  throw new Error(`Timed out waiting for Helm install to complete:\n${dialogText}`);
}

async function openChartInstallModal(page, {repoName, chartName}) {
  await page.goto("/app-store");
  await expect(page).toHaveURL(/\/app-store$/);

  await page.getByText(repoName, {exact: true}).click();
  const chartCard = page.locator(".ant-row .ant-col .ant-card").filter({has: page.getByText(chartName, {exact: true})});
  await expect(chartCard).toBeVisible({timeout: 30_000});
  await chartCard.getByRole("button", {name: "Install"}).click();

  const dialog = installModal(page);
  await expect(dialog).toBeVisible();
  return dialog;
}

// installAppFromAppStore drives the App Store install flow to completion: opens the chart's
// install modal, overrides the release name and values, submits, and waits for the streamed
// install log to report completion (the "Done" button). The release is recorded on
// installedReleases so installedReleasesFixture can uninstall it afterward.
async function installAppFromAppStore(page, {repoName, chartName, releaseName, namespace = "default", valuesYAML, installedReleases}) {
  const dialog = await openChartInstallModal(page, {repoName, chartName});

  const textarea = dialog.locator("textarea");
  await expect(textarea).toBeVisible({timeout: 30_000});

  await dialog.getByLabel("Release name").fill(releaseName);
  await textarea.fill(valuesYAML);

  await dialog.getByRole("button", {name: "Install"}).click();
  installedReleases.push({name: releaseName, namespace});

  await waitForInstallDone(dialog);
  await dialog.getByRole("button", {name: "Done"}).click();
  await expect(dialog).toBeHidden();
}

function servicesTable(page) {
  return page.locator(".ant-table-wrapper").filter({hasText: "Services"});
}

// getServiceAccessUrl reads the "Access URL" column rendered for a NodePort service on the
// Services page (see ServiceListPage.js), which is where an installed app's reachable URL
// is computed from the cluster node IP and the service's NodePort.
async function getServiceAccessUrl(page, namespace, name) {
  await page.goto("/services");
  await expect(page).toHaveURL(/\/services$/);

  const row = servicesTable(page).locator(`tr[data-row-key="${namespace}/${name}"]`);
  await expect(row).toBeVisible({timeout: 30_000});

  const link = row.getByRole("link").first();
  await expect(link).toBeVisible({timeout: 30_000});
  return link.getAttribute("href");
}

// waitForAppContent polls the access URL until it returns a 2xx response whose body contains
// expectedText, so a 404/blank/error page fails the test instead of a bare connection check.
async function waitForAppContent(request, url, expectedText, timeoutMs = HTTP_CHECK_TIMEOUT_MS) {
  const deadline = Date.now() + timeoutMs;
  let lastError = null;

  while (Date.now() < deadline) {
    try {
      const response = await request.get(url, {timeout: 10_000});
      if (response.ok()) {
        const body = await response.text();
        if (body.includes(expectedText)) {
          return body;
        }
        lastError = new Error(`Response body from ${url} did not contain "${expectedText}": ${body.slice(0, 200)}`);
      } else {
        lastError = new Error(`Got status ${response.status()} from ${url}`);
      }
    } catch (error) {
      lastError = error;
    }
    await new Promise(resolve => setTimeout(resolve, HTTP_CHECK_INTERVAL_MS));
  }

  throw lastError || new Error(`Timed out waiting for ${url} to serve expected content`);
}

module.exports = {
  API_UNINSTALL_HELM_RELEASE,
  API_ADD_HELM_REPO,
  API_GET_HELM_REPOS,
  API_DELETE_HELM_REPO,
  addCustomHelmRepo,
  addedHelmReposFixture,
  installAppFromAppStore,
  installedReleasesFixture,
  makeReleaseName,
  makeRepoName,
  getServiceAccessUrl,
  waitForAppContent,
};
