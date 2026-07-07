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

  test("returns ABORTED when the server aborts the install stream", async() => {
    global.fetch = jest.fn().mockResolvedValue(mockStreamResponse([
      "data: creating 1 resource(s)\n\n",
      "data: ABORTED\n\n",
    ]));

    const onLine = jest.fn();
    const status = await installHelmChartStream({releaseName: "demo"}, onLine);

    expect(status).toBe("ABORTED");
    expect(onLine).toHaveBeenCalledTimes(2);
    expect(onLine).toHaveBeenNthCalledWith(1, "creating 1 resource(s)");
    expect(onLine).toHaveBeenNthCalledWith(2, "ABORTED");
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
});
