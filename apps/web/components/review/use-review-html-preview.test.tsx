import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReviewFile } from "./types";

const mocks = vi.hoisted(() => ({
  requestFileContent: vi.fn(),
  getWebSocketClient: vi.fn(() => ({ id: "ws" })),
}));

const publisher = vi.hoisted(() => {
  const state = {
    url: null as string | null,
    error: null as "publish-failed" | "session-unavailable" | "too-large" | null,
    isPublishing: false,
  };
  return {
    state,
    publish: vi.fn(),
    reset: vi.fn(() => {
      state.url = null;
      state.error = null;
    }),
  };
});

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: mocks.getWebSocketClient,
}));

vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: mocks.requestFileContent,
}));

vi.mock("@/hooks/use-html-preview-publisher", () => ({
  useHtmlPreviewPublisher: () => ({
    url: publisher.state.url,
    error: publisher.state.error,
    isPublishing: publisher.state.isPublishing,
    publish: publisher.publish,
    reset: publisher.reset,
  }),
}));

import { useReviewHtmlPreview } from "./use-review-html-preview";

type PreviewFile = Pick<
  ReviewFile,
  "path" | "status" | "diff_skip_reason" | "source" | "repository_name"
>;

const SESSION_ID = "session-1";
const HTML_PATH = "site/index.html";
const REPO_NAME = "frontend";
const PREVIEW_URL = "http://api.test/port-proxy/session-1/43127/site/index.html?v=1";

function htmlFile(overrides: Partial<PreviewFile> = {}): PreviewFile {
  return {
    path: HTML_PATH,
    status: "modified",
    source: "uncommitted",
    repository_name: REPO_NAME,
    ...overrides,
  };
}

function fileContent(content: string, overrides: Record<string, unknown> = {}) {
  return { path: HTML_PATH, content, size: content.length, is_binary: false, ...overrides };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function renderPreview(file: PreviewFile = htmlFile()) {
  return renderHook(
    ({ currentFile }: { currentFile: PreviewFile }) =>
      useReviewHtmlPreview({ file: currentFile, sessionId: SESSION_ID }),
    { initialProps: { currentFile: file } },
  );
}

beforeEach(() => {
  publisher.publish.mockReset();
  publisher.publish.mockImplementation(async () => {
    publisher.state.url = PREVIEW_URL;
    return PREVIEW_URL;
  });
  publisher.reset.mockClear();
  publisher.state.url = null;
  publisher.state.error = null;
  publisher.state.isPublishing = false;
  mocks.requestFileContent.mockReset();
  mocks.getWebSocketClient.mockClear();
});

afterEach(() => {
  cleanup();
});

describe("useReviewHtmlPreview", () => {
  it("does not read until explicit activation, then publishes the complete workspace file", async () => {
    mocks.requestFileContent.mockResolvedValue(fileContent("<html>Complete workspace file</html>"));
    const { result } = renderPreview();

    expect(mocks.requestFileContent).not.toHaveBeenCalled();
    expect(result.current.isActive).toBe(false);
    expect(result.current.canPreview).toBe(true);

    await act(async () => {
      result.current.toggle();
    });

    await waitFor(() =>
      expect(publisher.publish).toHaveBeenCalledWith({
        path: HTML_PATH,
        content: "<html>Complete workspace file</html>",
        repo: REPO_NAME,
      }),
    );
    expect(mocks.requestFileContent).toHaveBeenCalledWith(
      { id: "ws" },
      SESSION_ID,
      HTML_PATH,
      REPO_NAME,
    );
    expect(result.current.isActive).toBe(true);
    expect(result.current.previewUrl).toBe(PREVIEW_URL);
  });
});

describe("useReviewHtmlPreview reads", () => {
  it("omits the repository scope for workspace-root files", async () => {
    mocks.requestFileContent.mockResolvedValue(fileContent("<html>Root file</html>"));
    const { result } = renderPreview(htmlFile({ repository_name: undefined }));

    await act(async () => {
      result.current.toggle();
    });

    await waitFor(() =>
      expect(publisher.publish).toHaveBeenCalledWith({
        path: HTML_PATH,
        content: "<html>Root file</html>",
      }),
    );
  });

  it("rejects a binary read response without publishing", async () => {
    mocks.requestFileContent.mockResolvedValue(fileContent("<html>", { is_binary: true }));
    const { result } = renderPreview();

    await act(async () => {
      result.current.toggle();
    });

    await waitFor(() => expect(result.current.readError).toBe(true));
    expect(publisher.publish).not.toHaveBeenCalled();
  });

  it("reports a workspace read failure and rereads the current file on retry", async () => {
    mocks.requestFileContent.mockRejectedValueOnce(new Error("workspace unavailable"));
    mocks.requestFileContent.mockResolvedValueOnce(fileContent("<html>Fresh file</html>"));
    const { result } = renderPreview();

    await act(async () => {
      result.current.toggle();
    });
    await waitFor(() => expect(result.current.readError).toBe(true));
    expect(publisher.publish).not.toHaveBeenCalled();

    await act(async () => {
      result.current.retry();
    });
    await waitFor(() =>
      expect(publisher.publish).toHaveBeenCalledWith({
        path: HTML_PATH,
        content: "<html>Fresh file</html>",
        repo: REPO_NAME,
      }),
    );
    expect(result.current.readError).toBe(false);
  });

  it("rereads the workspace file on refresh", async () => {
    mocks.requestFileContent
      .mockResolvedValueOnce(fileContent("<html>First</html>"))
      .mockResolvedValueOnce(fileContent("<html>Second</html>"));
    const { result } = renderPreview();

    await act(async () => {
      result.current.toggle();
    });
    await waitFor(() => expect(result.current.previewUrl).toBe(PREVIEW_URL));

    await act(async () => {
      result.current.refresh();
    });

    await waitFor(() =>
      expect(publisher.publish).toHaveBeenLastCalledWith({
        path: HTML_PATH,
        content: "<html>Second</html>",
        repo: REPO_NAME,
      }),
    );
  });
});

describe("useReviewHtmlPreview retirement", () => {
  it("retires a pending read when the user returns to the diff", async () => {
    const read = deferred<ReturnType<typeof fileContent>>();
    mocks.requestFileContent.mockReturnValue(read.promise);
    const { result } = renderPreview();

    await act(async () => {
      result.current.toggle();
    });
    act(() => {
      result.current.showDiff();
    });

    await act(async () => {
      read.resolve(fileContent("<html>Late content</html>"));
      await read.promise;
    });

    expect(publisher.publish).not.toHaveBeenCalled();
    expect(result.current.isActive).toBe(false);
    expect(publisher.reset).toHaveBeenCalled();
  });

  it("retires a pending read when the row identity changes", async () => {
    const read = deferred<ReturnType<typeof fileContent>>();
    mocks.requestFileContent.mockReturnValue(read.promise);
    const { result, rerender } = renderPreview();

    await act(async () => {
      result.current.toggle();
    });
    rerender({ currentFile: htmlFile({ path: "site/other.html" }) });

    await act(async () => {
      read.resolve(fileContent("<html>Previous file</html>"));
      await read.promise;
    });

    expect(publisher.publish).not.toHaveBeenCalled();
    expect(result.current.isActive).toBe(false);
  });

  it("retires a pending read on unmount", async () => {
    const read = deferred<ReturnType<typeof fileContent>>();
    mocks.requestFileContent.mockReturnValue(read.promise);
    const { result, unmount } = renderPreview();

    await act(async () => {
      result.current.toggle();
    });
    unmount();

    await act(async () => {
      read.resolve(fileContent("<html>Unmounted content</html>"));
      await read.promise;
    });

    expect(publisher.publish).not.toHaveBeenCalled();
  });

  it("ignores a publication that resolves after the preview closes", async () => {
    mocks.requestFileContent.mockResolvedValue(fileContent("<html>Slow publish</html>"));
    const publication = deferred<string>();
    publisher.publish.mockReturnValueOnce(publication.promise);
    const { result } = renderPreview();

    await act(async () => {
      result.current.toggle();
    });
    await waitFor(() => expect(publisher.publish).toHaveBeenCalledOnce());

    act(() => {
      result.current.showDiff();
    });
    await act(async () => {
      publication.resolve(PREVIEW_URL);
      await publication.promise;
    });

    expect(result.current.isActive).toBe(false);
    expect(result.current.previewUrl).toBeNull();
    expect(publisher.reset).toHaveBeenCalled();
  });
});

describe("useReviewHtmlPreview eligibility", () => {
  it("suppresses automatic review marking when the row geometry changes", async () => {
    mocks.requestFileContent.mockResolvedValue(fileContent("<html>Workspace page</html>"));
    const suppressAutoMark = { current: false };
    const { result } = renderHook(() =>
      useReviewHtmlPreview({
        file: htmlFile(),
        sessionId: SESSION_ID,
        suppressAutoMark,
      }),
    );

    await act(async () => {
      result.current.toggle();
    });
    expect(suppressAutoMark.current).toBe(true);

    suppressAutoMark.current = false;
    act(() => {
      result.current.showDiff();
    });
    expect(suppressAutoMark.current).toBe(true);
  });

  it("exposes no preview action for deleted, binary, or non-HTML rows", async () => {
    for (const file of [
      htmlFile({ status: "deleted" }),
      htmlFile({ diff_skip_reason: "binary" }),
      htmlFile({ path: "src/app.ts" }),
    ]) {
      mocks.requestFileContent.mockReset();
      const { result, unmount } = renderPreview(file);
      expect(result.current.canPreview).toBe(false);
      act(() => {
        result.current.toggle();
      });
      expect(mocks.requestFileContent).not.toHaveBeenCalled();
      unmount();
    }
  });
});
