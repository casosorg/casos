import {expect, test} from "@playwright/test";
import {expectOkJson, signInAsCiUser, uniqueName} from "./e2e-helpers.js";

/**
 * Engine settings and their history.
 *
 * A tuned setting is not stored anywhere casos can simply read back: it becomes
 * the engine's own command line, and the record of intent lives on the
 * StatefulSet beside it. Driving this against a real API server is what proves
 * the two stay in step — and that a value the catalogue does not allow never
 * reaches a command line at all.
 */

const NAMESPACE = "default";

function dbName(testInfo) {
  return uniqueName("e2e-params", testInfo);
}

async function createDatabase(page, name) {
  const response = await page.context().request.post("/api/create-database", {
    data: {
      namespace: NAMESPACE,
      name,
      engine: "postgresql",
      replicas: 1,
      storage: "1Gi",
      cpuLimit: "200m",
      memoryLimit: "256Mi",
    },
  });
  return expectOkJson(response);
}

async function params(page, name) {
  const response = await page.context().request.get(
    `/api/get-database-params?namespace=${NAMESPACE}&name=${name}`
  );
  const body = await expectOkJson(response);
  return body.data;
}

test.afterEach(async({page}, testInfo) => {
  await page.context().request.post("/api/delete-database", {
    data: {namespace: NAMESPACE, name: dbName(testInfo), deleteData: true},
  });
});

test("engine settings are applied, read back, and remembered", async({page}, testInfo) => {
  test.setTimeout(90_000);
  await signInAsCiUser(page);
  const name = dbName(testInfo);

  await createDatabase(page, name);

  // An untouched database reports the engine's own defaults and no history.
  const initial = await params(page, name);
  expect(initial.engine).toBe("postgresql");
  expect(initial.values.max_connections).toBe("100");
  expect(initial.history).toEqual([]);

  const configure = await page.context().request.post("/api/configure-database", {
    data: {namespace: NAMESPACE, name, params: {max_connections: "250", shared_buffers: "256MB"}},
  });
  const configured = await expectOkJson(configure);
  expect(configured.data.changed).toBe(2);

  const after = await params(page, name);
  expect(after.values.max_connections).toBe("250");
  expect(after.values.shared_buffers).toBe("256MB");
  // Untouched settings still read as the engine's default rather than blank.
  expect(after.values.work_mem).toBe("4MB");

  expect(after.history).toHaveLength(1);
  expect(after.history[0].changes).toEqual(
    expect.arrayContaining([{key: "max_connections", from: "100", to: "250"}])
  );

  // Saying the same thing again is not a change, so it does not restart the
  // database or add a line to the history.
  const again = await page.context().request.post("/api/configure-database", {
    data: {namespace: NAMESPACE, name, params: {max_connections: "250", shared_buffers: "256MB"}},
  });
  expect((await expectOkJson(again)).data.changed).toBe(0);
  expect((await params(page, name)).history).toHaveLength(1);
});

test("a value the setting cannot take is refused", async({page}, testInfo) => {
  test.setTimeout(90_000);
  await signInAsCiUser(page);
  const name = dbName(testInfo);

  await createDatabase(page, name);

  for (const value of ["not-a-number", "100; rm -rf /", "$(whoami)"]) {
    const response = await page.context().request.post("/api/configure-database", {
      data: {namespace: NAMESPACE, name, params: {max_connections: value}},
    });
    const body = await response.json();
    expect(body.status, `max_connections=${value} should have been refused`).toBe("error");
  }

  // Nothing was applied, so the database is still on the engine's default.
  expect((await params(page, name)).values.max_connections).toBe("100");
});

test("the database page offers engine settings with their history", async({page}, testInfo) => {
  test.setTimeout(90_000);
  await signInAsCiUser(page);
  const name = dbName(testInfo);

  await createDatabase(page, name);
  await page.context().request.post("/api/configure-database", {
    data: {namespace: NAMESPACE, name, params: {max_connections: "250"}},
  });

  await page.goto(`/databases/${NAMESPACE}/${name}`);
  await page.getByTestId("database-params").click();

  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("Maximum connections")).toBeVisible();
  await expect(dialog.locator("input").first()).toHaveValue("250");

  await dialog.getByRole("tab", {name: /History/}).click();
  await expect(dialog.getByText("max_connections:")).toBeVisible();
  await expect(dialog.getByText("250", {exact: false}).first()).toBeVisible();
});
