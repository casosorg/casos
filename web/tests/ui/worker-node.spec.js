const {expect, test} = require("@playwright/test");
const {expectOkJson, signInAsCiUser} = require("./e2e-helpers");
const {
  API_DEPLOY_MACHINE_NODE,
  createdMachinesFixture,
  createMachineFromUi,
  getMachineNodeTasks,
  makeMachineName,
  openWorkerNodePanel,
  startWorkerNodeDeployment,
  startWorkerNodeRepair,
  submitWorkerNodeAction,
  workerNodeDialog,
  expectWorkerNodeTaskVisible,
} = require("./worker-node-helpers");

const E2E_APISERVER_URL = process.env.E2E_APISERVER_URL || "https://127.0.0.1:16443";
const E2E_MACHINE_PREFIX = "a-worker-e2e";
const E2E_SLOW_SSH_IP = process.env.E2E_SLOW_SSH_IP || "192.0.2.1";

const workerNodeTest = test.extend({
  createdMachines: createdMachinesFixture,
});

async function expectErrorJson(response) {
  expect(response.ok()).toBeTruthy();
  const body = await response.json();
  expect(body.status).toBe("error");
  expect(body.msg).toBeTruthy();
  return body;
}

workerNodeTest.beforeEach(async({page}) => {
  await signInAsCiUser(page);
});

workerNodeTest("starts worker node deployment from the machines page @smoke", async({page, createdMachines}) => {
  const machineName = makeMachineName(E2E_MACHINE_PREFIX);

  await createMachineFromUi(page, machineName, createdMachines);
  await startWorkerNodeDeployment(page, machineName, E2E_APISERVER_URL);
});

workerNodeTest("reopens worker node deployment history from the machines page", async({page, createdMachines}) => {
  const machineName = makeMachineName(E2E_MACHINE_PREFIX);

  await createMachineFromUi(page, machineName, createdMachines);
  const task = await startWorkerNodeDeployment(page, machineName, E2E_APISERVER_URL);

  await workerNodeDialog(page, machineName).getByRole("button", {name: "Close"}).click();
  await expect(workerNodeDialog(page, machineName)).toBeHidden();

  const tasksBody = await openWorkerNodePanel(page, machineName);
  expect(tasksBody.data).toEqual(expect.arrayContaining([
    expect.objectContaining({
      id: task.id,
      machineName,
      nodeName: machineName,
      apiserverUrl: E2E_APISERVER_URL,
    }),
  ]));
  await expectWorkerNodeTaskVisible(page, machineName, task);
});

workerNodeTest("rejects a second active worker node deployment from the machines page", async({page, createdMachines}) => {
  const machineName = makeMachineName(E2E_MACHINE_PREFIX);

  await createMachineFromUi(page, machineName, createdMachines, {ip: E2E_SLOW_SSH_IP});
  await openWorkerNodePanel(page, machineName);
  const dialog = workerNodeDialog(page, machineName);
  await dialog.getByLabel("Apiserver URL").fill(E2E_APISERVER_URL);

  const firstDeploy = submitWorkerNodeAction(page, machineName, "Deploy Node", API_DEPLOY_MACHINE_NODE);
  const firstDeployBody = await expectOkJson(await firstDeploy);
  await expect(page.locator(".ant-message").getByText("Node deployment started", {exact: true})).toBeVisible();

  const duplicateDeploy = submitWorkerNodeAction(page, machineName, "Deploy Node", API_DEPLOY_MACHINE_NODE);
  const duplicateDeployBody = await expectErrorJson(await duplicateDeploy);
  expect(duplicateDeployBody.msg).toContain("already active");
  await expect(page.locator(".ant-message").getByText(/already active/)).toBeVisible();

  const tasksBody = await getMachineNodeTasks(page, machineName);
  expect(tasksBody.data).toHaveLength(1);
  expect(tasksBody.data[0]).toMatchObject({id: firstDeployBody.data.id, machineName});
  await expectWorkerNodeTaskVisible(page, machineName, firstDeployBody.data);
});

workerNodeTest("rejects invalid worker node deployment input from the machines page", async({page, createdMachines}) => {
  const machineName = makeMachineName(E2E_MACHINE_PREFIX);

  await createMachineFromUi(page, machineName, createdMachines);
  await openWorkerNodePanel(page, machineName);
  const dialog = workerNodeDialog(page, machineName);

  await dialog.getByLabel("Node name").fill("Bad_Node");
  await dialog.getByLabel("Apiserver URL").fill(E2E_APISERVER_URL);
  const invalidNodeName = submitWorkerNodeAction(page, machineName, "Deploy Node", API_DEPLOY_MACHINE_NODE);
  const invalidNodeNameBody = await expectErrorJson(await invalidNodeName);
  expect(invalidNodeNameBody.msg).toContain("nodeName must be a valid RFC 1123 subdomain");
  await expect(page.locator(".ant-message").getByText(/nodeName must be a valid RFC 1123 subdomain/)).toBeVisible();

  let tasksBody = await getMachineNodeTasks(page, machineName);
  expect(tasksBody.data).toHaveLength(0);

  await dialog.getByLabel("Node name").fill(machineName);
  await dialog.getByLabel("Apiserver URL").fill("http://127.0.0.1:16443");
  const invalidApiserver = submitWorkerNodeAction(page, machineName, "Deploy Node", API_DEPLOY_MACHINE_NODE);
  const invalidApiserverBody = await expectErrorJson(await invalidApiserver);
  expect(invalidApiserverBody.msg).toContain("apiserverUrl must be a valid https URL");
  await expect(page.locator(".ant-message").getByText(/apiserverUrl must be a valid https URL/)).toBeVisible();

  tasksBody = await getMachineNodeTasks(page, machineName);
  expect(tasksBody.data).toHaveLength(0);
});

workerNodeTest("starts worker node repair from the machines page", async({page, createdMachines}) => {
  const machineName = makeMachineName(E2E_MACHINE_PREFIX);

  await createMachineFromUi(page, machineName, createdMachines);
  await startWorkerNodeRepair(page, machineName, E2E_APISERVER_URL);
});
