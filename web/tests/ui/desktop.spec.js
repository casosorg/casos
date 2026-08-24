import {expect, test} from "@playwright/test";
import {signInAsCiUser} from "./e2e-helpers.js";

/**
 * The desktop is a second shell, not another page: its windows, dock and
 * launcher are code no route walk exercises. These are the four gestures that
 * make it an operating system rather than a wallpaper — launch, stack,
 * minimize, find — so a regression in any of them is worth a red run.
 */

async function openDesktop(page) {
  await page.goto("/desktop");
  await page.getByTestId("desktop-icons").waitFor();
}

test("the desktop launches apps into windows @smoke", async({page}) => {
  await signInAsCiUser(page);

  const crashes = [];
  page.on("pageerror", (error) => crashes.push(`pageerror: ${error.message}`));

  await openDesktop(page);
  await expect(page.getByTestId("desktop-dock")).toBeVisible();
  await expect(page.getByTestId("desktop-clock")).toBeVisible();

  await page.getByTestId("desktop-icon-system-pods").click();
  const podsWindow = page.getByTestId("desktop-window-system-pods");
  await expect(podsWindow).toHaveAttribute("data-size", "maximize");
  // The window hosts the same page the sidebar UI serves at /pods.
  await expect(podsWindow.locator("[data-slot=data-table]")).toBeVisible();

  // A second app stacks above the first rather than replacing it.
  await page.getByTestId("dock-icon-system-app-store").click();
  await expect(page.getByTestId("desktop-window-system-app-store")).toBeVisible();
  await expect(podsWindow).toBeAttached();

  // Clicking the dock icon of the window in front puts it away, and clicking
  // again brings it back.
  await page.getByTestId("dock-icon-system-app-store").click();
  await expect(page.getByTestId("desktop-window-system-app-store")).toHaveAttribute("data-size", "minimize");
  await page.getByTestId("dock-icon-system-app-store").click();
  await expect(page.getByTestId("desktop-window-system-app-store")).not.toHaveAttribute("data-size", "minimize");

  expect(crashes, `React crashed on the desktop:\n${crashes.join("\n")}`).toEqual([]);
});

test("the launcher finds an app that is not on the desktop @smoke", async({page}) => {
  await signInAsCiUser(page);
  await openDesktop(page);

  await page.getByTestId("desktop-folder").click();
  const launcher = page.getByTestId("desktop-launcher");
  await expect(launcher).toBeVisible();

  await launcher.locator("input").fill("secret");
  await expect(launcher.getByTestId("desktop-icon-system-secrets")).toBeVisible();

  await launcher.getByTestId("desktop-icon-system-secrets").click();
  await expect(page.getByTestId("desktop-window-system-secrets")).toBeVisible();
});
