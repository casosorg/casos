import {expect, test} from "@playwright/test";
import {expectOkJson, expectTableIdle, signInAsCiUser, uniqueName} from "./e2e-helpers.js";

/**
 * Version history and rollback.
 *
 * Rolling back is the one launchpad action that reaches for something casos did
 * not write: the ReplicaSets Kubernetes keeps behind a Deployment. That makes it
 * worth driving against a real API server rather than a fixture — the revision
 * numbering, the ownership filter and the pod-template-hash stripping are all
 * things only the cluster can confirm.
 */

const NAMESPACE = "default";

// Each test owns its own app: the suite runs its files in parallel, and two
// tests editing one Deployment would each see the other's revisions.
function appName(testInfo) {
  return uniqueName("e2e-rollback", testInfo);
}

async function deploy(page, APP, image) {
  const response = await page.context().request.post("/api/deploy-app", {
    data: {
      namespace: NAMESPACE,
      name: APP,
      image,
      replicas: 1,
      cpuLimit: "100m",
      memoryLimit: "64Mi",
      ports: [{containerPort: 80, protocol: "TCP", name: "http"}],
      serviceType: "ClusterIP",
      domains: [],
      configFiles: [],
      volumes: [],
      envVars: [],
      hpa: {enabled: false},
      registry: {enabled: false},
    },
  });
  return expectOkJson(response);
}

async function upgrade(page, APP, image) {
  const response = await page.context().request.post("/api/upgrade-image-app", {
    data: {namespace: NAMESPACE, name: APP, image, replicas: 1, cpuLimit: "100m", memoryLimit: "64Mi"},
  });
  return expectOkJson(response);
}

async function revisions(page, APP) {
  const response = await page.context().request.get(
    `/api/get-deployment-revisions?namespace=${NAMESPACE}&name=${APP}`
  );
  const body = await expectOkJson(response);
  return body.data ?? [];
}

// A ReplicaSet is made by the Deployment controller a moment after the
// Deployment is written, so two edits in quick succession can produce one
// ReplicaSet rather than two. Waiting for each to land is what a person editing
// an app does without noticing — generously, because the whole suite shares one
// small API server and the controller queues behind everyone else.
const CONTROLLER_TIMEOUT_MS = 90_000;

async function waitForRevisions(page, APP, count) {
  await expect(async() => {
    expect((await revisions(page, APP)).length).toBeGreaterThanOrEqual(count);
  }).toPass({timeout: CONTROLLER_TIMEOUT_MS});
}

test.afterEach(async({page}, testInfo) => {
  await page.context().request.post("/api/uninstall-image-app", {
    data: {namespace: NAMESPACE, name: appName(testInfo), deleteData: true},
  });
});

test("an app can be rolled back to the image it ran before", async({page}, testInfo) => {
  test.setTimeout(240_000);
  await signInAsCiUser(page);
  const APP = appName(testInfo);

  await deploy(page, APP, "nginx:1.27");
  await waitForRevisions(page, APP, 1);
  await upgrade(page, APP, "nginx:1.26");

  // Two edits, two revisions, newest first.
  await expect(async() => {
    const list = await revisions(page, APP);
    expect(list.length).toBeGreaterThanOrEqual(2);
    expect(list[0].image).toBe("nginx:1.26");
    expect(list[0].current).toBe(true);
    expect(list[1].image).toBe("nginx:1.27");
  }).toPass({timeout: CONTROLLER_TIMEOUT_MS});

  const before = await revisions(page, APP);
  const target = before.find((item) => item.image === "nginx:1.27");

  const rollback = await page.context().request.post("/api/rollback-deployment", {
    data: {namespace: NAMESPACE, name: APP, revision: target.revision},
  });
  await expectOkJson(rollback);

  // Rolling back rolls forward: the old image is running again, under a
  // revision number higher than the one it was rolled back from.
  await expect(async() => {
    const list = await revisions(page, APP);
    const current = list.find((item) => item.current);
    expect(current.image).toBe("nginx:1.27");
    expect(current.revision).toBeGreaterThan(before[0].revision);
  }).toPass({timeout: CONTROLLER_TIMEOUT_MS});
});

test("the version history is on the app's page, with the running one marked", async({page}, testInfo) => {
  test.setTimeout(240_000);
  await signInAsCiUser(page);
  const APP = appName(testInfo);

  await deploy(page, APP, "nginx:1.27");
  await waitForRevisions(page, APP, 1);
  await upgrade(page, APP, "nginx:1.26");
  await waitForRevisions(page, APP, 2);

  await page.goto(`/launchpad/${NAMESPACE}/${APP}`);
  const table = page.getByTestId("launchpad-revisions-table");
  await expectTableIdle(page, "launchpad-revisions-table");

  await expect(table.getByText("nginx:1.26")).toBeVisible();
  await expect(table.getByText("nginx:1.27")).toBeVisible();
  await expect(table.getByText("Running now")).toBeVisible();

  // Only the versions that are not running offer a way back to them.
  await expect(table.getByRole("button", {name: "Roll back"})).toHaveCount(1);
});
