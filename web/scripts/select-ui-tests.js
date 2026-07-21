const fs = require("fs");
const path = require("path");

// Registry of non-smoke regression specs selectable from changed paths.
const ALL_REGRESSION_TESTS = [
  "tests/ui/site-e2e.spec.js",
  "tests/ui/worker-node.spec.js",
  "tests/ui/worker-node-ready.spec.js",
  "tests/ui/app-store.spec.js",
];

const WORKER_NODE_PATTERNS = [
  /^controllers\/machine(_node_deploy)?\.go$/,
  /^controllers\/node\.go$/,
  /^object\/machine(_node_deploy)?\.go$/,
  /^object\/node\.go$/,
  /^web\/src\/Machine(ListPage|EditPage|NodeDeployPanel)\.js$/,
  /^web\/src\/NodeListPage\.js$/,
  /^web\/src\/backend\/(Machine(NodeDeploy)?|Node)Backend\.js$/,
  /^web\/tests\/ui\/worker-node(-ready)?\.spec\.js$/,
  /^web\/tests\/ui\/worker-node-helpers\.js$/,
];

const PLATFORM_READINESS_PATTERNS = [
  /^deploy\//,
  /^server\/(bootstrap|.*_bootstrap|apiserver|controllermanager|scheduler)\.go$/,
];

const SMOKE_COVERED_PATTERNS = [
  /^web\/src\/SiteEditPage\.js$/,
];

const SITE_PATTERNS = [
  /^controllers\/site\.go$/,
  /^object\/site\.go$/,
  /^web\/src\/SiteListPage\.js$/,
  /^web\/src\/backend\/SiteBackend\.js$/,
  /^web\/tests\/ui\/site-e2e\.spec\.js$/,
];

const APP_STORE_PATTERNS = [
  /^controllers\/helm\.go$/,
  /^object\/helm_repo\.go$/,
  /^store\/helm\.go$/,
  /^web\/src\/AppStorePage\.js$/,
  /^web\/src\/HelmInstallModal\.js$/,
  /^web\/src\/HelmReleasePage\.js$/,
  /^web\/src\/DeploymentListPage\.js$/,
  /^web\/src\/ServiceListPage\.js$/,
  /^web\/src\/backend\/HelmBackend\.js$/,
  /^web\/tests\/ui\/app-store\.spec\.js$/,
  /^web\/tests\/ui\/app-store-helpers\.js$/,
];

const FULL_REGRESSION_PATTERNS = [
  /^\.github\/workflows\//,
  /^conf\/app\.conf$/,
  /^routers\/router\.go$/,
  /^web\/package\.json$/,
  /^web\/playwright\.config\.js$/,
  /^web\/src\/(Conf|Setting)\.js$/,
  /^web\/src\/locales\//,
  /^web\/tests\/ui\/e2e-helpers\.js$/,
  /^web\/yarn\.lock$/,
];

const DOCS_ONLY_PATTERNS = [
  /(^|\/)(README|CHANGELOG|LICENSE)(\.[^/]*)?$/i,
  /\.md$/i,
  /^docs\//,
];

const CODE_ROOT_PATTERNS = [
  /^conf\//,
  /^controllers\//,
  /^main\.go$/,
  /^object\//,
  /^proxy\//,
  /^routers\//,
  /^web\/scripts\//,
  /^web\/src\//,
];

function normalizeChangedPath(filePath) {
  return String(filePath || "")
    .trim()
    .replace(/\\/g, "/")
    .replace(/^\.\//, "");
}

function matchesAny(filePath, patterns) {
  return patterns.some(pattern => pattern.test(filePath));
}

function isCodePath(filePath) {
  return matchesAny(filePath, CODE_ROOT_PATTERNS);
}

function normalizeChangedFiles(changedFiles) {
  if (!Array.isArray(changedFiles)) {
    return [];
  }
  return Array.from(new Set(
    changedFiles.map(normalizeChangedPath).filter(Boolean)
  ));
}

function selectRegressionTestsFromNormalized(normalizedFiles) {
  if (normalizedFiles.length === 0) {
    return [...ALL_REGRESSION_TESTS];
  }

  const selectedTests = new Set();
  let runAllRegression = false;

  // Ordering matters: skip docs, honor all-regression triggers, then apply targeted and smoke-covered matches.
  for (const filePath of normalizedFiles) {
    if (matchesAny(filePath, DOCS_ONLY_PATTERNS)) {
      continue;
    }
    if (matchesAny(filePath, FULL_REGRESSION_PATTERNS)) {
      runAllRegression = true;
      continue;
    }
    if (matchesAny(filePath, PLATFORM_READINESS_PATTERNS)) {
      selectedTests.add("tests/ui/worker-node.spec.js");
      selectedTests.add("tests/ui/worker-node-ready.spec.js");
      continue;
    }
    if (matchesAny(filePath, WORKER_NODE_PATTERNS)) {
      selectedTests.add("tests/ui/worker-node.spec.js");
      selectedTests.add("tests/ui/worker-node-ready.spec.js");
      continue;
    }
    if (matchesAny(filePath, SITE_PATTERNS)) {
      selectedTests.add("tests/ui/site-e2e.spec.js");
      continue;
    }
    if (matchesAny(filePath, APP_STORE_PATTERNS)) {
      selectedTests.add("tests/ui/app-store.spec.js");
      continue;
    }
    if (matchesAny(filePath, SMOKE_COVERED_PATTERNS)) {
      continue;
    }
    if (isCodePath(filePath)) {
      runAllRegression = true;
    }
  }

  if (runAllRegression) {
    return [...ALL_REGRESSION_TESTS];
  }

  return ALL_REGRESSION_TESTS.filter(testFile => selectedTests.has(testFile));
}

// Selects non-smoke UI regression specs for repository-relative changed paths.
function selectRegressionTests(changedFiles) {
  return selectRegressionTestsFromNormalized(normalizeChangedFiles(changedFiles));
}

function main(argv) {
  const changedFilesPath = argv[2];
  if (!changedFilesPath) {
    process.stderr.write("Usage: node scripts/select-ui-tests.js <changed-files.txt>\n");
    process.exitCode = 1;
    return;
  }

  const repoRoot = path.resolve(__dirname, "..", "..");
  let resolvedChangedFilesPath;
  try {
    resolvedChangedFilesPath = fs.realpathSync(path.resolve(changedFilesPath));
  } catch (error) {
    process.stderr.write(`Error reading changed files list: ${error.message}\n`);
    process.exitCode = 1;
    return;
  }

  const changedFilesPathRelative = path.relative(repoRoot, resolvedChangedFilesPath);
  if (changedFilesPathRelative.startsWith("..")) {
    process.stderr.write(`Error: changed files path is outside the repository: ${changedFilesPath}\n`);
    process.exitCode = 1;
    return;
  }

  let rawChangedFiles;
  try {
    rawChangedFiles = fs.readFileSync(resolvedChangedFilesPath, "utf8");
  } catch (error) {
    process.stderr.write(`Error reading changed files list: ${error.message}\n`);
    process.exitCode = 1;
    return;
  }

  const changedFiles = rawChangedFiles.split(/\r?\n/);
  const normalizedFiles = normalizeChangedFiles(changedFiles);
  if (normalizedFiles.length === 0) {
    process.stderr.write("Warning: no changed files detected; falling back to all regression tests.\n");
  }
  const tests = selectRegressionTestsFromNormalized(normalizedFiles);
  process.stdout.write(tests.length > 0 ? `${tests.join("\n")}\n` : "");
}

if (require.main === module) {
  main(process.argv);
}

module.exports = {
  ALL_REGRESSION_TESTS,
  selectRegressionTests,
};
