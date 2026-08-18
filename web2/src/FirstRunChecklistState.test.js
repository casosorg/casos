import test from "node:test";
import assert from "node:assert/strict";
import {getFirstRunChecklist} from "./FirstRunChecklistState.js";

test("marks all setup steps from the account and cluster state", () => {
  assert.deepEqual(
    getFirstRunChecklist({
      account: {owner: "basic"},
      signinOptions: {signinAvailable: true, autoSignin: false},
      machines: [{name: "worker-1"}],
      stats: {nodesTotal: 1, nodesReady: 1, deploymentsTotal: 2},
    }).map((item) => item.done),
    [true, true, true, true]
  );
});

test("leaves incomplete steps unchecked and skips password changes for Casdoor", () => {
  assert.deepEqual(
    getFirstRunChecklist({
      account: {owner: "basic"},
      signinOptions: {signinAvailable: true, autoSignin: true},
      machines: [],
      stats: {nodesTotal: 1, nodesReady: 0, deploymentsTotal: 0},
    }).map((item) => item.done),
    [false, false, false, false]
  );
  assert.equal(
    getFirstRunChecklist({
      account: {owner: "casdoor"},
      signinOptions: {signinAvailable: false, autoSignin: false},
      machines: [],
      stats: {nodesTotal: 0, nodesReady: 0, deploymentsTotal: 0},
    })[0].done,
    true
  );
});
