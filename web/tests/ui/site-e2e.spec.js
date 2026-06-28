const {expect, test} = require("@playwright/test");
const {signInAsCiUser} = require("./e2e-helpers");

test.beforeEach(async({page}) => {
  await signInAsCiUser(page);
});

test("renders the built-in site editor through the real backend @smoke", async({page}) => {
  await page.goto("/sites/site-built-in");

  await expect(page).toHaveURL(/\/sites\/site-built-in$/);
  await expect(page.locator("#parent-area")).toBeVisible();
  await expect(page.getByText("CI User")).toBeVisible();
  await expect(page.getByText("Edit Site")).toBeVisible();
  await expect(page.locator("input[disabled]").first()).toHaveValue("site-built-in");
  await expect(page.getByRole("button", {name: "Save"}).first()).toBeVisible();
});

test("renders the sites list through the real backend", async({page}) => {
  await page.goto("/sites");

  await expect(page).toHaveURL(/\/sites$/);
  const sitesTable = page.locator(".ant-table-wrapper").filter({hasText: "Sites"});
  await expect(sitesTable).toBeVisible();
  await expect(sitesTable.getByRole("link", {name: "site-built-in"})).toBeVisible();
  await expect(sitesTable.getByRole("cell", {name: "CasOS", exact: true})).toBeVisible();
  await expect(sitesTable.getByRole("button", {name: "Add"})).toBeDisabled();
});
