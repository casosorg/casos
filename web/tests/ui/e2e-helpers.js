import {randomUUID} from "crypto";
import {expect} from "@playwright/test";

const API_E2E_SIGNIN = "/api/e2e/signin";
const e2eToken = process.env.E2E_TEST_TOKEN;
const e2eSshPassword = process.env.E2E_SSH_PASSWORD || randomUUID();

async function expectOkJson(response) {
  expect(response.ok()).toBeTruthy();
  const body = await response.json();
  expect(body.status).toBe("ok");
  return body;
}

async function signInAsCiUser(page) {
  expect(e2eToken).toBeTruthy();

  // These specs drive the full Kubernetes surface, which is advanced mode. A
  // fresh browser starts in simple mode, so the opt-in is explicit rather than
  // whatever the default happens to be.
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

export {
  dataTable,
  e2eSshPassword,
  expectOkJson,
  expectTableIdle,
  expectToast,
  signInAsCiUser,
  tableRow,
  toast,
};
