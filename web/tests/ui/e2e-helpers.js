const {randomUUID} = require("crypto");
const {expect} = require("@playwright/test");

const API_E2E_SIGNIN = "/api/e2e/signin";
const e2eToken = process.env.E2E_TEST_TOKEN;
const e2eSshPassword = process.env.E2E_SSH_PASSWORD || randomUUID();

async function expectOkJson(response) {
  expect(response.ok()).toBeTruthy();
  const body = await response.json();
  expect(body.status).toBe("ok");
  return body;
}

async function signInAsCiUser(page) {
  expect(e2eToken).toBeTruthy();

  await page.addInitScript(() => {
    localStorage.setItem("language", "en");
  });

  const signin = await page.context().request.post(API_E2E_SIGNIN, {
    headers: {
      "X-Casos-E2E-Token": e2eToken,
    },
  });
  const signinBody = await expectOkJson(signin);
  expect(signinBody.data).toMatchObject({
    name: "ci-user",
    displayName: "CI User",
  });
}

module.exports = {
  e2eSshPassword,
  expectOkJson,
  signInAsCiUser,
};
