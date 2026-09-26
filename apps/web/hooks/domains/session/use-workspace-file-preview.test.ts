import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useWorkspaceFilePreview } from "./use-workspace-file-preview";

const mocks = vi.hoisted(() => ({
  getWebSocketClient: vi.fn(),
  requestFileContent: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: mocks.getWebSocketClient,
}));

vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: mocks.requestFileContent,
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
}

describe("useWorkspaceFilePreview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getWebSocketClient.mockReturnValue({});
  });

  it("reports loading and then the loaded workspace response", async () => {
    const request = deferred<{ content: string; is_binary: boolean }>();
    mocks.requestFileContent.mockReturnValue(request.promise);
    const { result } = renderHook(() =>
      useWorkspaceFilePreview("session-1", "reports/summary.txt", "repo-a"),
    );

    let load!: Promise<void>;
    act(() => {
      load = result.current.load();
    });
    expect(result.current.state).toEqual({ kind: "loading" });

    request.resolve({ content: "ready", is_binary: false });
    await act(() => load);

    expect(result.current.state).toEqual({
      kind: "loaded",
      response: { content: "ready", is_binary: false },
    });
    expect(mocks.requestFileContent).toHaveBeenCalledWith(
      expect.anything(),
      "session-1",
      "reports/summary.txt",
      "repo-a",
    );
  });

  it("reports an unavailable client as an error", async () => {
    mocks.getWebSocketClient.mockReturnValue(null);
    const { result } = renderHook(() =>
      useWorkspaceFilePreview("session-2", "reports/missing.txt"),
    );

    await act(() => result.current.load());

    expect(result.current.state).toEqual({ kind: "error" });
    expect(mocks.requestFileContent).not.toHaveBeenCalled();
  });

  it("reports request failures as an error", async () => {
    mocks.requestFileContent.mockRejectedValue(new Error("request failed"));
    const { result } = renderHook(() => useWorkspaceFilePreview("session-3", "reports/error.txt"));

    await act(() => result.current.load());

    expect(result.current.state).toEqual({ kind: "error" });
  });

  it("ignores a stale response after a newer load starts", async () => {
    const staleRequest = deferred<{ content: string; is_binary: boolean }>();
    const currentRequest = deferred<{ content: string; is_binary: boolean }>();
    mocks.requestFileContent
      .mockReturnValueOnce(staleRequest.promise)
      .mockReturnValueOnce(currentRequest.promise);
    const { result } = renderHook(() =>
      useWorkspaceFilePreview("session-4", "reports/current.txt"),
    );

    let staleLoad!: Promise<void>;
    let currentLoad!: Promise<void>;
    act(() => {
      staleLoad = result.current.load();
      currentLoad = result.current.load();
    });
    staleRequest.resolve({ content: "stale", is_binary: false });
    await act(() => staleLoad);
    expect(result.current.state).toEqual({ kind: "loading" });

    currentRequest.resolve({ content: "current", is_binary: false });
    await act(() => currentLoad);
    expect(result.current.state).toEqual({
      kind: "loaded",
      response: { content: "current", is_binary: false },
    });
  });
});
