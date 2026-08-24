import {expect, test} from "@playwright/test";
import {signInAsCiUser} from "./e2e-helpers.js";

/**
 * The desktop's app SDK. An installed app runs in a frame of its own and can
 * only learn who is signed in, what language the desktop speaks, or how to open
 * a sibling app by asking over postMessage. These exercise that channel from
 * the far side of a frame, which is the only side that proves it works.
 */

test("an app in a frame can ask the desktop who is signed in", async({page}) => {
  await signInAsCiUser(page);
  await page.addInitScript(() => window.localStorage.setItem("desktopTourSeen", "true"));
  await page.goto("/desktop");
  await page.getByTestId("desktop-icons").waitFor();

  const result = await page.evaluate(async() => {
    const frame = document.createElement("iframe");
    frame.srcdoc = "<!doctype html><script src='/casos-app-sdk.js'></script>";
    document.body.appendChild(frame);
    await new Promise((resolve) => frame.addEventListener("load", resolve));

    const app = frame.contentWindow.casosApp;
    const session = await app.getSession();
    const language = await app.getLanguage();
    const host = await app.getHostConfig();
    const apps = await app.getApps();
    let rejected = null;
    try {
      await app.openApp({appKey: "no-such-app"});
    } catch (error) {
      rejected = error.message;
    }
    return {session, language, host, appCount: apps.length, rejected};
  });

  expect(result.session.user.name).toBeTruthy();
  expect(result.language.lng).toBeTruthy();
  expect(result.host.cloud.domain).toBeTruthy();
  expect(result.appCount).toBeGreaterThan(0);
  expect(result.rejected).toContain("no-such-app");
});

test("an app in a frame can open another app on the desktop", async({page}) => {
  await signInAsCiUser(page);
  await page.addInitScript(() => window.localStorage.setItem("desktopTourSeen", "true"));
  await page.goto("/desktop");
  await page.getByTestId("desktop-icons").waitFor();

  await page.evaluate(async() => {
    const frame = document.createElement("iframe");
    frame.srcdoc = "<!doctype html><script src='/casos-app-sdk.js'></script>";
    document.body.appendChild(frame);
    await new Promise((resolve) => frame.addEventListener("load", resolve));
    await frame.contentWindow.casosApp.openApp({appKey: "system-secrets"});
  });

  await expect(page.getByTestId("desktop-window-system-secrets")).toBeVisible();
});
