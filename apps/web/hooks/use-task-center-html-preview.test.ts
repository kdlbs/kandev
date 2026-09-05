import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const { publishHtmlPreviewUrl } = vi.hoisted(() => ({
  publishHtmlPreviewUrl: vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("./use-html-preview-publisher", () => ({
  getHtmlPreviewPublishErrorCode: () => "publish-failed",
  getHtmlPreviewPublishErrorKey: () => "task:htmlPreviewPublishFailed",
  publishHtmlPreviewUrl,
}));

import { useTaskCenterHtmlPreview } from "./use-task-center-html-preview";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

const previewRequest = {
  path: "index.html",
  repo: undefined,
  content: "<body>current</body>",
};

describe("useTaskCenterHtmlPreview", () => {
  it("ignores a successful publish that belongs to a replaced session", async () => {
    const oldRequest = deferred<string>();
    const currentRequest = deferred<string>();
    publishHtmlPreviewUrl
      .mockReturnValueOnce(oldRequest.promise)
      .mockReturnValueOnce(currentRequest.promise);
    const openBrowserPanel = vi.fn();
    const toast = vi.fn();
    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string | null }) =>
        useTaskCenterHtmlPreview({
          activeSessionId: sessionId,
          openBrowserPanel,
          toast,
        }),
      { initialProps: { sessionId: "old-session" } },
    );

    let oldPublish!: Promise<void>;
    act(() => {
      oldPublish = result.current.handlePreviewHtml(
        previewRequest.path,
        previewRequest.repo,
        previewRequest.content,
      );
    });
    rerender({ sessionId: "current-session" });
    expect(result.current.isPublishingHtmlPreview).toBe(false);

    let currentPublish!: Promise<void>;
    act(() => {
      currentPublish = result.current.handlePreviewHtml(
        previewRequest.path,
        previewRequest.repo,
        previewRequest.content,
      );
    });
    expect(result.current.isPublishingHtmlPreview).toBe(true);

    await act(async () => {
      oldRequest.resolve("/old-session-preview");
      await oldPublish;
    });

    expect(openBrowserPanel).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
    expect(result.current.isPublishingHtmlPreview).toBe(true);

    await act(async () => {
      currentRequest.resolve("/current-session-preview");
      await currentPublish;
    });

    expect(openBrowserPanel).toHaveBeenCalledWith("/current-session-preview");
    expect(openBrowserPanel).toHaveBeenCalledTimes(1);
    expect(result.current.isPublishingHtmlPreview).toBe(false);
  });

  it("ignores a stale publish error after the active session changes", async () => {
    const oldRequest = deferred<string>();
    const currentRequest = deferred<string>();
    publishHtmlPreviewUrl
      .mockReturnValueOnce(oldRequest.promise)
      .mockReturnValueOnce(currentRequest.promise);
    const openBrowserPanel = vi.fn();
    const toast = vi.fn();
    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string | null }) =>
        useTaskCenterHtmlPreview({
          activeSessionId: sessionId,
          openBrowserPanel,
          toast,
        }),
      { initialProps: { sessionId: "old-session" } },
    );

    let oldPublish!: Promise<void>;
    act(() => {
      oldPublish = result.current.handlePreviewHtml(
        previewRequest.path,
        previewRequest.repo,
        previewRequest.content,
      );
    });
    rerender({ sessionId: "current-session" });
    let currentPublish!: Promise<void>;
    act(() => {
      currentPublish = result.current.handlePreviewHtml(
        previewRequest.path,
        previewRequest.repo,
        previewRequest.content,
      );
    });

    await act(async () => {
      oldRequest.reject(new Error("old session failed"));
      await oldPublish;
    });

    expect(openBrowserPanel).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
    expect(result.current.isPublishingHtmlPreview).toBe(true);

    await act(async () => {
      currentRequest.resolve("/current-session-preview");
      await currentPublish;
    });
  });
});
