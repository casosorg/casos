import {randomUUID} from "crypto";
import {expect} from "@playwright/test";

const API_E2E_SIGNIN = "/api/e2e/signin";
const e2eToken = process.env.E2E_TEST_TOKEN;
const e2eSshPassword = process.env.E2E_SSH_PASSWORD || randomUUID();

// The web server answers well before the cluster it manages does, so a spec
// that creates Kubernetes objects has to wait for the API server rather than
// for the page. Everything before this point answers "apiserver not ready".
async function waitForApiServer(page) {
  await expect(async() => {
    const response = await page.context().request.get("/api/get-namespaces");
    const body = await response.json();
    expect(body.status, `the cluster is not up yet: ${body.msg ?? ""}`).toBe("ok");
  }).toPass({timeout: 120_000, intervals: [500, 1000, 2000]});
}

async function expectOkJson(response) {
  expect(response.ok()).toBeTruthy();
  const body = await response.json();
  // The backend's own message is the only thing that says why; without it a
  // failed call is just "expected ok, got error".
  expect(body.status, `${response.url()} answered: ${body.msg ?? "(no message)"}`).toBe("ok");
  return body;
}

async function signInAsCiUser(page) {
  expect(e2eToken).toBeTruthy();

  // Every screen is decided by its URL, so this only settles where "/" lands:
  // these specs drive the full Kubernetes surface, and a fresh browser would
  // otherwise be sent to simple mode's home.
  await page.addInitScript(() => {
    localStorage.setItem("language", "en");
    localStorage.setItem("uiMode", "advanced");
  });

  const signin = await page.context().request.post(API_E2E_SIGNIN, {
    headers: {
      "X-Casos-E2E-Token": e2eToken,
    },
  });
  const signinBody = await expectOkJson(signin);
  expect(signinBody.data).toMatchObject({
    name: "ci-user",
    displayName: "CI User",
  });

  // Signed in, but not yet able to do anything: the cluster is asked for after
  // the session exists, because the check itself needs one.
  await waitForApiServer(page);
}

// Toasts come from sonner, which marks each one with data-sonner-toast. That
// attribute is the addressable part: the surrounding classes are Tailwind
// output and must never be selected on.
function toast(page) {
  return page.locator("[data-sonner-toast]");
}

// A string matches the toast body exactly; a RegExp matches loosely, which is
// what the backend-error assertions need.
function expectToast(page, text) {
  const body = typeof text === "string" ? toast(page).getByText(text, {exact: true}) : toast(page).getByText(text);
  return expect(body).toBeVisible();
}

// A DataTable renders skeleton rows while loading and stamps data-loading on its
// root, so "the table has settled" is an assertion rather than a sleep.
function dataTable(page, testId) {
  return testId ? page.getByTestId(testId) : page.locator("[data-slot=data-table]");
}

async function expectTableIdle(page, testId) {
  await expect(dataTable(page, testId)).toHaveAttribute("data-loading", "false");
}

function tableRow(scope, rowKey) {
  return scope.locator(`tr[data-row-key="${rowKey}"]`);
}

// A name of its own for each test, derived from its title.
//
// Naming by worker index is not enough: a worker is handed the next test as
// soon as one finishes, and the object that test just deleted can still be
// terminating when the next one tries to create it again.
function uniqueName(prefix, testInfo) {
  const slug = testInfo.title.toLowerCase().replace(/[^a-z0-9]+/g, "-");
  // Trimmed after truncating, not before: a name cut mid-word can end on the
  // dash, and Kubernetes rejects that in both names and label values.
  return `${prefix}-${slug}`.slice(0, 40).replace(/^-+|-+$/g, "");
}

export {
  dataTable,
  e2eSshPassword,
  expectOkJson,
  expectTableIdle,
  expectToast,
  signInAsCiUser,
  tableRow,
  toast,
  uniqueName,
  waitForApiServer,
};
