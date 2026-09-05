import { act, cleanup, renderHook } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

const { openFiles, publishHtmlPreviewUrl } = vi.hoisted(() => ({
  openFiles: new Map<string, { content: string; repo?: string }>(),
  publishHtmlPreviewUrl: vi.fn(),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: {
    getState: () => ({ openFiles }),
  },
}));

vi.mock("@/lib/state/dockview-panel-actions", () => ({
  buildRepoScopedItemId: (path: string, repo?: string) => (repo ? `${repo}:${path}` : path),
}));

vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));

vi.mock("./use-html-preview-publisher", () => ({
  getHtmlPreviewPublishErrorCode: () => "publish-failed",
  getHtmlPreviewPublishErrorKey: () => "task:htmlPreviewPublishFailed",
  publishHtmlPreviewUrl,
}));

import { useFileHtmlPreview } from "./use-file-html-preview";

afterEach(() => {
  cleanup();
  openFiles.clear();
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

describe("useFileHtmlPreview", () => {
  it("does not toast or clear the current busy state for a stale session error", async () => {
    openFiles.set("index.html", { content: "<body>current</body>" });
    const oldRequest = deferred<string>();
    const currentRequest = deferred<string>();
    publishHtmlPreviewUrl
      .mockReturnValueOnce(oldRequest.promise)
      .mockReturnValueOnce(currentRequest.promise);
    const activeSessionIdRef = { current: "old-session" };
    const openBrowserPanel = vi.fn();
    const toast = vi.fn();
    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) => {
        activeSessionIdRef.current = sessionId;
        const [isPublishing, setPublishing] = useState(false);
        const preview = useFileHtmlPreview({
          activeSessionId: sessionId,
          activeSessionIdRef,
          setPublishingHtmlPreview: setPublishing,
          openBrowserPanel,
          toast,
        });
        return { isPublishing, preview };
      },
      { initialProps: { sessionId: "old-session" } },
    );

    let oldPublish!: Promise<void>;
    act(() => {
      oldPublish = result.current.preview("index.html");
    });
    rerender({ sessionId: "current-session" });
    let currentPublish!: Promise<void>;
    act(() => {
      currentPublish = result.current.preview("index.html");
    });

    await act(async () => {
      oldRequest.reject(new Error("old session failed"));
      await oldPublish;
    });

    expect(toast).not.toHaveBeenCalled();
    expect(openBrowserPanel).not.toHaveBeenCalled();
    expect(result.current.isPublishing).toBe(true);

    await act(async () => {
      currentRequest.resolve("/current-session-preview");
      await currentPublish;
    });
    expect(openBrowserPanel).toHaveBeenCalledWith("/current-session-preview");
    expect(result.current.isPublishing).toBe(false);
  });
});
