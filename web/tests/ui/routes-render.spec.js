import {expect, test} from "@playwright/test";
import {signInAsCiUser} from "./e2e-helpers.js";

/**
 * Not a port of anything in web/tests — added during the port because nothing
 * else covers it.
 *
 * The other specs drive four screens. The remaining thirty are only reached by
 * a human clicking around, and a component that throws on render produces a
 * blank page that still answers 200, so neither the Go tests nor a build check
 * can see it. This walk found exactly that: `<Button asChild>` was handing
 * Radix's Slot two children and taking the App Store and the 404 page down.
 *
 * It asserts two cheap things per route — React did not throw, and something
 * actually painted — which is what makes a crash impossible to miss.
 */

// Mirrors the route table in src/routes.jsx, plus one unmatched path so the
// 404 screen is covered too.
const ROUTES = [
  "/dashboard",
  "/app-store",
  "/launchpad",
  "/launchpad/new",
  "/databases",
  "/databases/new",
  "/app-store/templates",
  "/helm-releases",
  "/pods",
  "/deployments",
  "/statefulsets",
  "/daemonsets",
  "/jobs",
  "/cronjobs",
  "/nodes",
  "/namespaces",
  "/serviceaccounts",
  "/configmaps",
  "/secrets",
  "/pvcs",
  "/storageclasses",
  "/resourcequotas",
  "/hpas",
  "/services",
  "/ingresses",
  "/networkpolicies",
  "/rolebindings",
  "/clusterrolebindings",
  "/admission-policy",
  "/authorization-policy",
  "/trivy-scans",
  "/monitor",
  "/terminal",
  "/log-search",
  "/topology",
  "/machines",
  "/sites",
  "/sites/site-built-in",
  "/no-such-page",
];

// Simple mode has URLs of its own, so its screens are a separate walk rather
// than the same paths with a flag flipped.
const SIMPLE_MODE_ROUTES = [
  "/simple",
  "/simple/app-store",
  "/simple/apps",
  "/simple/launchpad",
  "/simple/launchpad/new",
  "/simple/databases",
  "/simple/databases/new",
  "/simple/devices",
  "/simple/health",
];

// A rendered screen always carries the shell's own chrome; anything shorter
// than this means React unmounted the tree.
const MIN_RENDERED_CHARS = 20;
const SETTLE_MS = 700;

test("every route renders without a React crash @smoke", async({page}) => {
  test.setTimeout(300_000);
  await signInAsCiUser(page);

  const crashes = [];
  page.on("pageerror", (error) => crashes.push(`pageerror: ${error.message}`));
  page.on("console", (message) => {
    const text = message.text();
    // React logs this immediately before unmounting a subtree that threw.
    if (message.type() === "error" && text.includes("The above error occurred")) {
      crashes.push(text.split("\n")[0]);
    }
  });

  const blank = [];
  for (const route of ROUTES) {
    await page.goto(route);
    // The shell renders nothing until the account request resolves, and a first
    // visit also downloads that route's own chunk. Waiting for the shell keeps
    // the walk independent of how long either takes.
    await page.getByTestId("management-layout").waitFor({state: "attached"});
    await page.waitForTimeout(SETTLE_MS);
    const rendered = (await page.locator("body").innerText()).trim();
    if (rendered.length < MIN_RENDERED_CHARS) {
      blank.push(`${route} rendered ${rendered.length} chars`);
    }
  }

  expect(crashes, `React crashed on:\n${crashes.join("\n")}`).toEqual([]);
  expect(blank, `Routes that rendered nothing:\n${blank.join("\n")}`).toEqual([]);
});

test("simple mode renders its own screens without a React crash @smoke", async({page}) => {
  test.setTimeout(120_000);
  await signInAsCiUser(page);

  const crashes = [];
  page.on("pageerror", (error) => crashes.push(`pageerror: ${error.message}`));
  page.on("console", (message) => {
    const text = message.text();
    if (message.type() === "error" && text.includes("The above error occurred")) {
      crashes.push(text.split("\n")[0]);
    }
  });

  const blank = [];
  for (const route of SIMPLE_MODE_ROUTES) {
    await page.goto(route);
    // The shell renders nothing until the account request resolves, and a first
    // visit also downloads that route's own chunk. Waiting for the shell keeps
    // the walk independent of how long either takes.
    await page.getByTestId("management-layout").waitFor({state: "attached"});
    await page.waitForTimeout(SETTLE_MS);
    const rendered = (await page.locator("body").innerText()).trim();
    if (rendered.length < MIN_RENDERED_CHARS) {
      blank.push(`${route} rendered ${rendered.length} chars`);
    }
  }

  // The rail is the one thing that must differ: seven plain-language entries
  // and none of the advanced groups.
  await expect(page.getByRole("link", {name: "App Store"})).toBeVisible();
  await expect(page.getByRole("button", {name: "Workloads"})).toHaveCount(0);

  expect(crashes, `React crashed on:\n${crashes.join("\n")}`).toEqual([]);
  expect(blank, `Routes that rendered nothing:\n${blank.join("\n")}`).toEqual([]);
});
