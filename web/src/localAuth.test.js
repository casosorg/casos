/* eslint-env jest */

import {TextEncoder} from "util";
import {getLocalPasswordError, localPasswordErrors} from "./localAuth";

global.TextEncoder = TextEncoder;

test("local password validation matches the backend character and byte limits", () => {
  expect(getLocalPasswordError("1234567")).toBe(localPasswordErrors.tooShort);
  expect(getLocalPasswordError("12345678")).toBe("");
  expect(getLocalPasswordError("密码安全测试一二")).toBe("");
  expect(getLocalPasswordError("😀😀😀😀")).toBe(localPasswordErrors.tooShort);
  expect(getLocalPasswordError("密码密码密码密码密码密码密码密码密码密码密码密码密码")).toBe(localPasswordErrors.tooLong);
});
