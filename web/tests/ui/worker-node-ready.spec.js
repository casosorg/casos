const {expect, test} = require("@playwright/test");
const {e2eSshPassword, signInAsCiUser} = require("./e2e-helpers");
const {
  createdMachinesFixture,
  createMachineFromUi,
  makeMachineName,
  startWorkerNodeDeployment,
  workerNodeDialog,
  workerNodeTaskTable,
} = require("./worker-node-helpers");

// This test only runs in CI jobs that provisioned a real worker VM
// (see "Prepare worker node VM" in .github/workflows/build.yml). It exercises
// the full path other worker-node.spec.js tests stop short of: the deployment
// task actually finishing and the node showing up as Ready on the Nodes page.
const E2E_WORKER_VM_IP = process.env.E2E_WORKER_VM_IP;
const E2E_APISERVER_URL = process.env.E2E_APISERVER_URL || "https://127.0.0.1:16443";
const E2E_MACHINE_PREFIX = "a-worker-ready-e2e";
const E2E_WORKER_DEPLOY_TIMEOUT_MS = Number(process.env.E2E_WORKER_DEPLOY_TIMEOUT_MS) || 8 * 60 * 1000;
const E2E_WORKER_READY_TIMEOUT_MS = Number(process.env.E2E_WORKER_READY_TIMEOUT_MS) || 2 * 60 * 1000;

const workerNodeReadyTest = test.extend({
  createdMachines: createdMachinesFixture,
});
// This test deploys to a real VM and can legitimately take several minutes;
// retrying it on failure would just re-run an expensive, non-flaky wait and
// risks blowing the CI job's overall time budget.
workerNodeReadyTest.describe.configure({retries: 0});

function nodeRow(page, nodeName) {
  return page.locator(".ant-table-wrapper").filter({hasText: "Nodes"}).locator(`tr[data-row-key="${nodeName}"]`);
}

async function waitForDeployTaskToSucceed(page, machineName, taskId) {
  const taskRow = workerNodeTaskTable(page, machineName).locator(`tr[data-row-key="${taskId}"]`);
  await expect(taskRow.getByRole("cell", {name: "succeeded", exact: true})).toBeVisible({
    timeout: E2E_WORKER_DEPLOY_TIMEOUT_MS,
  });
}

async function waitForNodeReady(page, nodeName) {
  await expect(async() => {
    await page.goto("/nodes");
    await expect(nodeRow(page, nodeName).getByText("Ready", {exact: true})).toBeVisible({timeout: 2000});
  }).toPass({timeout: E2E_WORKER_READY_TIMEOUT_MS});
}

workerNodeReadyTest.beforeEach(async({page}) => {
  test.skip(!E2E_WORKER_VM_IP, "E2E_WORKER_VM_IP is not set; no worker VM was provisioned for this run");
  await signInAsCiUser(page);
});

workerNodeReadyTest("deployed worker node becomes Ready on the Nodes page", async({page, createdMachines}) => {
  workerNodeReadyTest.setTimeout(E2E_WORKER_DEPLOY_TIMEOUT_MS + E2E_WORKER_READY_TIMEOUT_MS + 60 * 1000);

  const machineName = makeMachineName(E2E_MACHINE_PREFIX);

  await createMachineFromUi(page, machineName, createdMachines, {
    ip: E2E_WORKER_VM_IP,
    username: "root",
    password: e2eSshPassword,
  });

  const task = await startWorkerNodeDeployment(page, machineName, E2E_APISERVER_URL);
  await waitForDeployTaskToSucceed(page, machineName, task.id);
  await workerNodeDialog(page, machineName).getByRole("button", {name: "Close"}).click();

  await waitForNodeReady(page, machineName);
});
