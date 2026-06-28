const assert = require("assert");
const {execFileSync} = require("child_process");
const fs = require("fs");
const path = require("path");
const {ALL_REGRESSION_TESTS, selectRegressionTests} = require("./select-ui-tests");

function expectSelection(name, changedFiles, expectedTests) {
  assert.deepStrictEqual(selectRegressionTests(changedFiles), expectedTests, name);
}

expectSelection(
  "worker node UI changes select worker node regression",
  ["web/src/MachineNodeDeployPanel.js"],
  ["tests/ui/worker-node.spec.js"]
);

expectSelection(
  "worker node backend changes select worker node regression once",
  ["controllers/machine.go", "object/machine_node_deploy.go", "web/src/MachineListPage.js"],
  ["tests/ui/worker-node.spec.js"]
);

expectSelection(
  "site changes rely on fixed smoke coverage",
  ["web/src/SiteEditPage.js"],
  []
);

expectSelection(
  "site list and backend changes select site regression",
  ["web/src/SiteListPage.js", "web/src/backend/SiteBackend.js", "object/site.go"],
  ["tests/ui/site-e2e.spec.js"]
);

expectSelection(
  "docs-only changes do not request extra regression tests",
  ["README.md", "docs/ci.md"],
  []
);

expectSelection(
  "UI test infrastructure changes run all regression tests",
  ["web/tests/ui/e2e-helpers.js"],
  ALL_REGRESSION_TESTS
);

expectSelection(
  "unknown frontend code changes run all regression tests",
  ["web/src/DeploymentListPage.js"],
  ALL_REGRESSION_TESTS
);

expectSelection(
  "UI selector script changes run all regression tests",
  ["web/scripts/select-ui-tests.js"],
  ALL_REGRESSION_TESTS
);

expectSelection(
  "non-array inputs fall back to all regression tests",
  null,
  ALL_REGRESSION_TESTS
);

const cliInputPath = path.join(__dirname, `.select-ui-tests-${process.pid}.txt`);
fs.writeFileSync(cliInputPath, "web/src/MachineNodeDeployPanel.js\n", "utf8");
try {
  const output = execFileSync(process.execPath, [path.join(__dirname, "select-ui-tests.js"), cliInputPath], {
    encoding: "utf8",
  });
  assert.strictEqual(output, "tests/ui/worker-node.spec.js\n", "CLI prints selected regression tests");
} finally {
  fs.rmSync(cliInputPath, {force: true});
}
