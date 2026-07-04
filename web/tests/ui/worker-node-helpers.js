const {randomUUID} = require("crypto");
const {expect} = require("@playwright/test");
const {e2eSshPassword, expectOkJson} = require("./e2e-helpers");

const API_ADD_MACHINE = "/api/add-machine";
const API_DELETE_MACHINE = "/api/delete-machine";
const API_DEPLOY_MACHINE_NODE = "/api/deploy-machine-node";
const API_GET_MACHINE_NODE_TASKS = "/api/get-machine-node-tasks";
const API_REPAIR_MACHINE_NODE = "/api/repair-machine-node";
// MachineListPage currently submits new machines with owner "admin".
const E2E_MACHINE_OWNER = process.env.E2E_MACHINE_OWNER || "admin";
const MAX_MACHINE_PAGES_TO_SCAN = Number(process.env.E2E_MAX_MACHINE_PAGES_TO_SCAN) || 50;

const createdMachinesFixture = async({page}, use) => {
  const createdMachines = [];
  await use(createdMachines);

  const cleanupErrors = [];
  for (const machine of [...createdMachines].reverse()) {
    try {
      const deleteMachine = await page.context().request.post(API_DELETE_MACHINE, {
        data: machine,
      });
      await expectOkJson(deleteMachine);
    } catch (error) {
      cleanupErrors.push(`${machine.name}: ${error.message}`);
    }
  }

  expect(cleanupErrors).toEqual([]);
};

function makeMachineName(prefix) {
  return `${prefix}-${randomUUID().slice(0, 8)}`;
}

function machineTable(page) {
  return page.locator(".ant-table-wrapper").filter({hasText: "Machines"});
}

function machineRowFor(page, machineName) {
  return machineTable(page).locator(`tr[data-row-key="${machineName}"]`);
}

function machineTableTitle(page) {
  return machineTable(page).locator(".ant-table-title");
}

function activeMachinePage(page) {
  return machineTable(page).locator(".ant-pagination-item-active");
}

async function waitForMachineTableIdle(page) {
  await expect(machineTable(page).locator(".ant-spin-spinning")).toHaveCount(0);
}

async function activeMachinePageNumber(page) {
  const activePage = activeMachinePage(page);
  const pageText = (await activePage.getAttribute("data-page")) || (await activePage.textContent())?.trim();
  const pageNumber = Number(pageText);
  if (!Number.isInteger(pageNumber) || pageNumber < 1) {
    throw new Error(`Could not read active Machines page number from "${pageText || "<empty>"}"`);
  }
  return pageNumber;
}

async function expectActiveMachinePage(page, pageNumber) {
  const pageItem = machineTable(page).locator(`.ant-pagination-item-${pageNumber}`);
  await expect(pageItem).toHaveClass(/ant-pagination-item-active/);
}

async function clickMachinePagination(page, button, step) {
  const currentPage = await activeMachinePageNumber(page);
  await button.click();
  await expectActiveMachinePage(page, currentPage + step);
  await waitForMachineTableIdle(page);
}

async function goToFirstMachinePage(page) {
  const firstPage = machineTable(page).locator(".ant-pagination-item-1");
  if (await firstPage.count() > 0) {
    const firstPageClass = await firstPage.getAttribute("class");
    if (!firstPageClass?.includes("ant-pagination-item-active")) {
      await firstPage.click();
    }
    await expectActiveMachinePage(page, 1);
    return;
  }

  const previousPage = machineTable(page).locator(".ant-pagination-prev button");
  let pageTurnCount = 0;
  while (await previousPage.count() > 0 && await previousPage.isEnabled()) {
    if (pageTurnCount >= MAX_MACHINE_PAGES_TO_SCAN) {
      throw new Error("Could not reach the first Machines page; pagination appears stuck");
    }
    await clickMachinePagination(page, previousPage, -1);
    pageTurnCount += 1;
  }
  if (await activeMachinePage(page).count() === 0) {
    const previousPageCount = await previousPage.count();
    if (previousPageCount > 0) {
      await expect(previousPage).toBeDisabled();
    }
    const nextPage = machineTable(page).locator(".ant-pagination-next button");
    const nextPageCount = await nextPage.count();
    if (nextPageCount > 0) {
      await expect(nextPage).toBeDisabled();
    }
    await expect(machineTable(page)).toBeVisible();
    await expect(machineTable(page).locator("tbody tr").first()).toBeAttached();
    return;
  }
  await expectActiveMachinePage(page, 1);
}

async function findMachineRow(page, machineName) {
  await goToFirstMachinePage(page);
  await waitForMachineTableIdle(page);
  const machineRow = machineRowFor(page, machineName);
  let pageTurnCount = 0;

  while (await machineRow.count() === 0) {
    if (pageTurnCount >= MAX_MACHINE_PAGES_TO_SCAN) {
      throw new Error(`Could not find machine ${machineName} within ${MAX_MACHINE_PAGES_TO_SCAN} Machines pages`);
    }
    const nextPage = machineTable(page).locator(".ant-pagination-next button");
    if (await nextPage.count() === 0 || !(await nextPage.isEnabled())) {
      throw new Error(`Machine ${machineName} was not found after scanning all available Machines pages`);
    }
    await clickMachinePagination(page, nextPage, 1);
    pageTurnCount += 1;
  }

  await machineRow.scrollIntoViewIfNeeded();
  await expect(machineRow).toBeVisible();
  return machineRow;
}

async function getMachineNodeTasks(page, machineName) {
  const tasks = await page.context().request.get(
    `${API_GET_MACHINE_NODE_TASKS}?owner=${encodeURIComponent(E2E_MACHINE_OWNER)}&machineName=${encodeURIComponent(machineName)}`
  );
  return expectOkJson(tasks);
}

function workerNodeDialog(page, machineName) {
  return page.getByRole("dialog", {name: `Worker Node - ${machineName}`});
}

function workerNodeTaskTable(page, machineName) {
  return workerNodeDialog(page, machineName).getByRole("table");
}

async function expectWorkerNodeTaskVisible(page, machineName, task) {
  const taskRow = workerNodeTaskTable(page, machineName).locator(`tr[data-row-key="${task.id}"]`);
  await expect(taskRow.getByRole("cell", {name: String(task.id), exact: true})).toBeVisible();
  await expect(taskRow.getByRole("cell", {name: task.nodeName || machineName, exact: true})).toBeVisible();
}

async function expectWorkerNodeLogVisible(page, machineName, message) {
  await expect(workerNodeDialog(page, machineName).getByText(message)).toBeVisible();
}

async function createMachineFromUi(page, machineName, createdMachines, options = {}) {
  await page.goto("/machines");
  await page.waitForLoadState("networkidle");
  await expect(page).toHaveURL(/\/machines$/);

  await machineTableTitle(page).getByRole("button", {name: "Add"}).click();
  const addDialog = page.getByRole("dialog", {name: "Add Machine"});
  await expect(addDialog).toBeVisible();
  await addDialog.getByPlaceholder("my-machine").fill(machineName);
  await addDialog.getByPlaceholder("My Machine").fill(options.displayName || "E2E Worker Node");
  await addDialog.getByPlaceholder("192.168.1.10").fill(options.ip || "127.0.0.1");
  await addDialog.getByPlaceholder("root").fill(options.username || "root");
  await addDialog.getByLabel("Password").fill(options.password || e2eSshPassword);

  const addMachine = page.waitForResponse(response =>
    response.url().includes(API_ADD_MACHINE) && response.request().method() === "POST"
  );
  await addDialog.getByRole("button", {name: "Add"}).click();
  const addMachineResponse = await addMachine;
  try {
    await expectOkJson(addMachineResponse);
  } catch (error) {
    if (addMachineResponse.ok()) {
      createdMachines.push({owner: E2E_MACHINE_OWNER, name: machineName});
    }
    throw error;
  }
  createdMachines.push({owner: E2E_MACHINE_OWNER, name: machineName});
  await expect(addDialog).toBeHidden();

  await findMachineRow(page, machineName);
}

async function openWorkerNodePanel(page, machineName) {
  const machineRow = await findMachineRow(page, machineName);

  const loadTasks = page.waitForResponse(response =>
    response.url().includes(API_GET_MACHINE_NODE_TASKS) && response.request().method() === "GET"
  );
  await machineRow.getByRole("button", {name: "Deploy worker node"}).click();
  await expect(workerNodeDialog(page, machineName)).toBeVisible();
  return expectOkJson(await loadTasks);
}

async function submitWorkerNodeAction(page, machineName, buttonName, apiPath) {
  const request = page.waitForResponse(response =>
    response.url().includes(apiPath) && response.request().method() === "POST"
  );
  await workerNodeDialog(page, machineName).getByRole("button", {name: buttonName}).click();
  return request;
}

async function startWorkerNodeDeployment(page, machineName, apiserverUrl) {
  await openWorkerNodePanel(page, machineName);
  const dialog = workerNodeDialog(page, machineName);
  await expect(dialog.getByLabel("Node name")).toHaveValue(machineName);
  await dialog.getByLabel("Apiserver URL").fill(apiserverUrl);

  const deployMachineNode = submitWorkerNodeAction(page, machineName, "Deploy Node", API_DEPLOY_MACHINE_NODE);
  const deployBody = await expectOkJson(await deployMachineNode);
  expect(deployBody.data).toMatchObject({
    machineName,
    nodeName: machineName,
    apiserverUrl,
    status: "pending",
    phase: "queued",
  });

  await expect(page.locator(".ant-message").getByText("Node deployment started", {exact: true})).toBeVisible();
  await expectWorkerNodeTaskVisible(page, machineName, deployBody.data);
  await expectWorkerNodeLogVisible(page, machineName, "Node deployment task created");
  return deployBody.data;
}

async function startWorkerNodeRepair(page, machineName, apiserverUrl) {
  await openWorkerNodePanel(page, machineName);
  const dialog = workerNodeDialog(page, machineName);
  await expect(dialog.getByLabel("Node name")).toHaveValue(machineName);
  await dialog.getByLabel("Apiserver URL").fill(apiserverUrl);

  const repairMachineNode = submitWorkerNodeAction(page, machineName, "Repair Node", API_REPAIR_MACHINE_NODE);
  const repairBody = await expectOkJson(await repairMachineNode);
  expect(repairBody.data).toMatchObject({
    machineName,
    nodeName: machineName,
    apiserverUrl,
    status: "pending",
    phase: "queued",
  });

  await expect(page.locator(".ant-message").getByText("Node repair started", {exact: true})).toBeVisible();
  await expectWorkerNodeTaskVisible(page, machineName, repairBody.data);
  await expectWorkerNodeLogVisible(page, machineName, "Node deployment task created");
  return repairBody.data;
}

module.exports = {
  API_ADD_MACHINE,
  API_DELETE_MACHINE,
  API_DEPLOY_MACHINE_NODE,
  API_GET_MACHINE_NODE_TASKS,
  API_REPAIR_MACHINE_NODE,
  E2E_MACHINE_OWNER,
  createdMachinesFixture,
  createMachineFromUi,
  findMachineRow,
  getMachineNodeTasks,
  makeMachineName,
  openWorkerNodePanel,
  startWorkerNodeDeployment,
  startWorkerNodeRepair,
  submitWorkerNodeAction,
  workerNodeDialog,
  workerNodeTaskTable,
  expectWorkerNodeTaskVisible,
  expectWorkerNodeLogVisible,
};
