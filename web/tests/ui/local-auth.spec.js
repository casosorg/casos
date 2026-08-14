const {expect, test} = require("@playwright/test");

const passwordA = "CasOS-local-auth-A-2026";
const passwordB = "CasOS-local-auth-B-2026";
const localSetupToken = process.env.E2E_LOCAL_SETUP_TOKEN;

test.skip(!process.env.CI && process.env.E2E_LOCAL_AUTH !== "true", "requires an isolated or explicitly approved local-auth backend");

async function tryPassword(request, password) {
  const response = await request.post("/api/signin", {
    data: {username: "admin", password},
  });
  const body = await response.json();
  return body.status === "ok";
}

test("initializes, signs in, and changes the local administrator password @smoke", async({page}) => {
  test.setTimeout(90 * 1000);
  await page.addInitScript(() => localStorage.setItem("language", "en"));
  const request = page.context().request;
  const optionsResponse = await request.get("/api/get-signin-options");
  const options = await optionsResponse.json();
  expect(options.status).toBe("ok");
  expect(options.data.authMode).toBe("local");

  let currentPassword;
  if (options.data.setupRequired) {
    await page.goto("/signin");
    await expect(page.getByRole("heading", {name: "Initialize CasOS"})).toBeVisible();
    const passwordInputs = page.locator("input[type=password]");
    if (options.data.setupTokenRequired === true) {
      expect(localSetupToken).toBeTruthy();
      await passwordInputs.nth(0).fill(localSetupToken);
      await passwordInputs.nth(1).fill(passwordA);
      await passwordInputs.nth(2).fill(passwordA);
    } else {
      await passwordInputs.nth(0).fill(passwordA);
      await passwordInputs.nth(1).fill(passwordA);
    }
    const setupResponse = page.waitForResponse(response => response.url().endsWith("/api/setup") && response.request().method() === "POST");
    await page.getByRole("button", {name: /Set up and continue/}).click({noWaitAfter: true});
    expect((await setupResponse).status()).toBe(200);
    await page.waitForURL(/\/dashboard$|\/$/, {timeout: 60000});
    currentPassword = passwordA;
  } else if (await tryPassword(request, passwordA)) {
    currentPassword = passwordA;
  } else {
    expect(await tryPassword(request, passwordB)).toBe(true);
    currentPassword = passwordB;
  }

  const nextPassword = currentPassword === passwordA ? passwordB : passwordA;
  await page.goto("/account");
  await expect(page.getByRole("heading", {name: "Local account"})).toBeVisible();
  await page.getByLabel("Current password").fill(currentPassword);
  await page.getByLabel("New password", {exact: true}).fill(nextPassword);
  await page.getByLabel("Confirm new password").fill(nextPassword);
  await page.getByRole("button", {name: "Update password"}).click();
  await expect(page.getByText("Password updated")).toBeVisible();

  const signout = await request.post("/api/signout");
  expect((await signout.json()).status).toBe("ok");
  await page.goto("/signin");
  await expect(page.getByRole("heading", {name: "Sign in to CasOS"})).toBeVisible();
  await page.getByLabel("Password").fill(nextPassword);
  await page.getByRole("button", {name: "Sign In"}).click();
  await expect(page).toHaveURL(/\/dashboard$|\/$/);
  await expect(page.locator("#parent-area")).toBeVisible();
});
