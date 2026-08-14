/* eslint-env jest */

jest.mock("../Setting", () => ({
  ServerUrl: "http://localhost:9000",
  handleFetchResponse: (response) => response.json(),
}));

import {getSigninOptions, setup, signinWithPassword, updateAccount} from "./AccountBackend";

describe("local account API", () => {
  beforeEach(() => {
    global.fetch = jest.fn().mockResolvedValue({
      json: jest.fn().mockResolvedValue({status: "ok"}),
    });
  });

  afterEach(() => {
    jest.resetAllMocks();
  });

  test("loads public sign-in options", async() => {
    await getSigninOptions();

    expect(global.fetch).toHaveBeenCalledWith("http://localhost:9000/api/get-signin-options", expect.objectContaining({
      method: "GET",
      credentials: "include",
    }));
  });

  test("submits the first-run password", async() => {
    await setup("one-time-setup-token", "secure password");

    expect(global.fetch).toHaveBeenCalledWith("http://localhost:9000/api/setup", expect.objectContaining({
      method: "POST",
      credentials: "include",
      body: JSON.stringify({setupToken: "one-time-setup-token", password: "secure password"}),
    }));
  });

  test("submits local administrator credentials", async() => {
    await signinWithPassword("admin", "secure password");

    expect(global.fetch).toHaveBeenCalledWith("http://localhost:9000/api/signin", expect.objectContaining({
      method: "POST",
      credentials: "include",
      body: JSON.stringify({username: "admin", password: "secure password"}),
    }));
  });

  test("submits current and replacement passwords", async() => {
    await updateAccount("old password", "new password");

    expect(global.fetch).toHaveBeenCalledWith("http://localhost:9000/api/update-account", expect.objectContaining({
      method: "POST",
      credentials: "include",
      body: JSON.stringify({currentPassword: "old password", newPassword: "new password"}),
    }));
  });
});
