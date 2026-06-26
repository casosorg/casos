const {expect, test} = require("@playwright/test");

const e2eToken = process.env.E2E_TEST_TOKEN || "local-e2e-token";
const failureVideoHoldMs = Number(process.env.E2E_FAILURE_VIDEO_HOLD_MS || 8000);

async function keepFailureVisible(page, runAssertions) {
  try {
    await runAssertions();
  } catch (error) {
    await showFailureOverlay(page, error);
    await page.waitForTimeout(failureVideoHoldMs);
    throw error;
  }
}

async function showFailureOverlay(page, error) {
  const errorMessage = error instanceof Error ? error.message : String(error);
  const summary = errorMessage.split("\n").slice(0, 4).join("\n");

  await page.evaluate((message) => {
    document.getElementById("ci-ui-test-failure-overlay")?.remove();

    const overlay = document.createElement("div");
    overlay.id = "ci-ui-test-failure-overlay";
    overlay.textContent = `UI test failed:\n${message}`;
    overlay.style.cssText = [
      "position: fixed",
      "top: 16px",
      "left: 16px",
      "right: 16px",
      "z-index: 2147483647",
      "padding: 14px 16px",
      "border: 3px solid #b91c1c",
      "background: #fef2f2",
      "color: #7f1d1d",
      "font: 600 18px/1.35 Arial, sans-serif",
      "white-space: pre-wrap",
      "box-shadow: 0 8px 24px rgba(0, 0, 0, 0.24)",
    ].join(";");
    document.body.appendChild(overlay);
  }, summary).catch(() => {});
}

test("renders the built-in site editor through the real backend", async ({page}) => {
  expect(e2eToken).toBeTruthy();

  await page.addInitScript(() => {
    localStorage.setItem("language", "en");
  });

  await test.step("sign in through the CI-only backend endpoint", async () => {
    const signin = await page.context().request.post("/api/e2e/signin", {
      headers: {
        "X-Casos-E2E-Token": e2eToken,
      },
    });
    expect(signin.ok()).toBeTruthy();
    await expect(signin).toBeOK();
    await expect(signin.json()).resolves.toMatchObject({
      status: "ok",
      data: {
        name: "ci-user",
        displayName: "CI User",
      },
    });
  });

  await test.step("open the built-in site editor", async () => {
    await page.goto("/sites/site-built-in");
    await page.waitForLoadState("networkidle");
  });

  await test.step("verify the built-in site editor is rendered", async () => {
    await keepFailureVisible(page, async () => {
      await expect(page).toHaveURL(/\/sites\/site-built-in$/);
      await expect(page.locator("#parent-area")).toBeVisible();
      await expect(page.getByText("CI User")).toBeVisible();
      await expect(page.getByText("Edit Site")).toBeVisible();
      await expect(page.locator("input[disabled]").first()).toHaveValue("site-built-in");
      await expect(page.getByRole("button", {name: "Save"}).first()).toBeVisible();
      await expect(page.getByText("CI artifact failure demo marker")).toBeVisible();
    });
  });
});
