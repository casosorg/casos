/* eslint-env jest */

import {
  findStoredHelmTask,
  helmTaskMatchesIdentity,
  helmTaskPollRetryDelay,
  helmTaskStorageKey
} from "./helmTaskStorage";

describe("Helm install task recovery", () => {
  const now = 1_800_000_000_000;

  beforeEach(() => {
    window.localStorage.clear();
    jest.spyOn(Date, "now").mockReturnValue(now);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  test("restores a fresh task with complete chart identity", () => {
    const key = helmTaskStorageKey("demo", "apps", "demo-release");
    window.localStorage.setItem(key, JSON.stringify({
      schemaVersion: 1,
      taskId: 42,
      createdAt: now - 1000,
      chartName: "demo",
      namespace: "apps",
      releaseName: "demo-release",
    }));

    expect(findStoredHelmTask("demo")).toMatchObject({
      key,
      taskId: "42",
      chartName: "demo",
      namespace: "apps",
      releaseName: "demo-release",
    });
  });

  test("migrates the previous JSON format but rejects identity-free task ids", () => {
    const legacyKey = helmTaskStorageKey("demo", "apps", "legacy-release");
    const rawKey = helmTaskStorageKey("demo", "apps", "raw-release");
    window.localStorage.setItem(legacyKey, JSON.stringify({
      taskId: 43,
      createdAt: now - 1000,
      namespace: "apps",
      releaseName: "legacy-release",
    }));
    window.localStorage.setItem(rawKey, "44");

    expect(findStoredHelmTask("demo")).toMatchObject({
      key: legacyKey,
      taskId: "43",
      chartName: "demo",
    });
    expect(window.localStorage.getItem(rawKey)).toBeNull();
  });

  test("discards expired tasks before attempting recovery", () => {
    const key = helmTaskStorageKey("demo", "apps", "expired-release");
    window.localStorage.setItem(key, JSON.stringify({
      schemaVersion: 1,
      taskId: 45,
      createdAt: now - (25 * 60 * 60 * 1000),
      chartName: "demo",
      namespace: "apps",
      releaseName: "expired-release",
    }));

    expect(findStoredHelmTask("demo")).toBeNull();
    expect(window.localStorage.getItem(key)).toBeNull();
  });

  test("matches a recovered task against its complete identity", () => {
    const task = {id: 46, chartName: "demo", namespace: "apps", releaseName: "demo-release"};
    const identity = {chartName: "demo", namespace: "apps", releaseName: "demo-release"};

    expect(helmTaskMatchesIdentity(task, "46", identity)).toBe(true);
    expect(helmTaskMatchesIdentity({...task, namespace: "other"}, "46", identity)).toBe(false);
    expect(helmTaskMatchesIdentity(task, "47", identity)).toBe(false);
  });

  test("backs off failed status polls with a bounded delay", () => {
    expect(helmTaskPollRetryDelay(1)).toBe(2000);
    expect(helmTaskPollRetryDelay(2)).toBe(4000);
    expect(helmTaskPollRetryDelay(10)).toBe(30000);
  });
});
