import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";

const mocks = vi.hoisted(() => ({
  publishHtmlPreview: vi.fn(),
  buildHtmlPreviewProxyUrl: vi.fn(),
}));

vi.mock("@/lib/api/domains/process-api", () => ({
  publishHtmlPreview: (...args: unknown[]) => mocks.publishHtmlPreview(...args),
  buildHtmlPreviewProxyUrl: (...args: unknown[]) => mocks.buildHtmlPreviewProxyUrl(...args),
}));

import {
  getHtmlPreviewPublishErrorCode,
  getHtmlPreviewPublishErrorKey,
  useHtmlPreviewPublisher,
} from "./use-html-preview-publisher";

describe("useHtmlPreviewPublisher", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.publishHtmlPreview.mockResolvedValue({ port: 43127, path: "/index.html", version: 1 });
    mocks.buildHtmlPreviewProxyUrl.mockReturnValue("http://api.test/preview/index.html?v=1");
  });

  it("publishes a buffer and exposes the versioned URL", async () => {
    const { result } = renderHook(() => useHtmlPreviewPublisher("session-1"));
    const payload = { path: "index.html", repo: undefined, content: "<h1>current</h1>" };

    await act(async () => {
      await expect(result.current.publish(payload)).resolves.toBe(
        "http://api.test/preview/index.html?v=1",
      );
    });

    expect(mocks.publishHtmlPreview).toHaveBeenCalledWith("session-1", payload);
    expect(result.current.status).toBe("ready");
    expect(result.current.url).toBe("http://api.test/preview/index.html?v=1");
  });

  it("reports a missing session without making a request", async () => {
    const { result } = renderHook(() => useHtmlPreviewPublisher(null));

    await act(async () => {
      await expect(
        result.current.publish({ path: "index.html", content: "<h1>current</h1>" }),
      ).resolves.toBeNull();
    });

    expect(mocks.publishHtmlPreview).not.toHaveBeenCalled();
    expect(result.current.error).toBe("session-unavailable");
  });
});

describe("HTML preview publish error mapping", () => {
  it("maps API limits and availability failures to localized keys", () => {
    expect(getHtmlPreviewPublishErrorCode(new ApiError("too large", 413, null))).toBe("too-large");
    expect(getHtmlPreviewPublishErrorCode(new ApiError("gone", 503, null))).toBe(
      "session-unavailable",
    );
    expect(getHtmlPreviewPublishErrorKey("publish-failed")).toBe("task:htmlPreviewPublishFailed");
  });
});
