/* eslint-env jest */

jest.mock("../Setting", () => ({
  ServerUrl: "http://localhost:9000",
  getAcceptLanguage: () => "en",
}));

import {TextDecoder} from "util";
import {installHelmChartStream} from "./HelmBackend";

global.TextDecoder = TextDecoder;

function makeChunk(text) {
  return Uint8Array.from(Buffer.from(text, "utf8"));
}

function mockStreamResponse(chunks) {
  let index = 0;
  return {
    body: {
      getReader() {
        return {
          async read() {
            if (index >= chunks.length) {
              return {done: true};
            }
            return {done: false, value: makeChunk(chunks[index++])};
          },
        };
      },
    },
  };
}

describe("installHelmChartStream", () => {
  afterEach(() => {
    jest.resetAllMocks();
  });

  test("rejects when the server reports an install failure", async() => {
    global.fetch = jest.fn().mockResolvedValue(mockStreamResponse([
      "data: creating 1 resource(s)\n\n",
      "data: ERROR: install failed\n\n",
    ]));

    const onLine = jest.fn();
    await expect(installHelmChartStream({releaseName: "demo"}, onLine))
      .rejects.toThrow("install failed");

    expect(onLine).toHaveBeenCalledTimes(2);
    expect(onLine).toHaveBeenNthCalledWith(1, "creating 1 resource(s)");
    expect(onLine).toHaveBeenNthCalledWith(2, "ERROR: install failed");
  });

  test("returns DONE when the server completes the install stream", async() => {
    global.fetch = jest.fn().mockResolvedValue(mockStreamResponse([
      "data: creating 1 resource(s)\n\n",
      "data: DONE\n\n",
    ]));

    const onLine = jest.fn();
    const status = await installHelmChartStream({releaseName: "demo"}, onLine);

    expect(status).toBe("DONE");
    expect(onLine).toHaveBeenCalledTimes(2);
    expect(onLine).toHaveBeenNthCalledWith(2, "DONE");
  });

  test("forwards the task id and abort signal", async() => {
    global.fetch = jest.fn().mockResolvedValue(mockStreamResponse([
      "data: TASK_ID:42\n\n",
      "data: DONE\n\n",
    ]));
    const signal = {aborted: false};
    const onLine = jest.fn();

    await installHelmChartStream({releaseName: "demo"}, onLine, signal);

    expect(global.fetch.mock.calls[0][1].signal).toBe(signal);
    expect(onLine).toHaveBeenNthCalledWith(1, "TASK_ID:42");
    expect(onLine).toHaveBeenNthCalledWith(2, "DONE");
  });
});
