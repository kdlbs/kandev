import { beforeEach, describe, expect, it, vi } from "vitest";

const fetchJsonMock = vi.hoisted(() => vi.fn());

vi.mock("../client", () => ({
  fetchJson: fetchJsonMock,
}));

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  buildHtmlPreviewProxyUrl,
  publishHtmlPreview,
  type HtmlPreviewPublishResponse,
} from "./process-api";

describe("HTML preview process API", () => {
  beforeEach(() => fetchJsonMock.mockReset());

  it("publishes the current buffer through the session endpoint", async () => {
    const response: HtmlPreviewPublishResponse = {
      port: 43127,
      path: "/site/index.html",
      version: 4,
    };
    fetchJsonMock.mockResolvedValueOnce(response);

    await expect(
      publishHtmlPreview("session-1", {
        repo: "frontend",
        path: "site/index.html",
        content: "<body>unsaved</body>",
      }),
    ).resolves.toEqual(response);

    expect(fetchJsonMock).toHaveBeenCalledWith(
      "/api/v1/task-sessions/session-1/html-previews",
      expect.objectContaining({
        init: {
          method: "POST",
          body: JSON.stringify({
            repo: "frontend",
            path: "site/index.html",
            content: "<body>unsaved</body>",
          }),
        },
      }),
    );
  });

  it("builds a versioned session port-proxy URL", () => {
    expect(
      buildHtmlPreviewProxyUrl("session/1", {
        port: 43127,
        path: "/site/index.html",
        version: 4,
      }),
    ).toBe("http://api.test/port-proxy/session%2F1/43127/site/index.html?v=4");
  });
});
