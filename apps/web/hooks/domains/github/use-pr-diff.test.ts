import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { resolvePRDiffView, usePRDiff, type KeyedPRDiffState } from "./use-pr-diff";

const { requestMock, getWebSocketClientMock } = vi.hoisted(() => ({
  requestMock: vi.fn(),
  getWebSocketClientMock: vi.fn(),
}));
const FIRST_PR_FILENAME = "src/first-pr.ts";
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: getWebSocketClientMock,
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  requestMock.mockReset();
  getWebSocketClientMock.mockReset();
  getWebSocketClientMock.mockReturnValue({ request: requestMock });
});

function wrapper({ children }: { children: ReactNode }) {
  const initialState = {
    workspaces: { activeId: "workspace-1" },
  } as unknown as Partial<AppState>;
  return createElement(StateProvider, { initialState, children });
}

const staleState: KeyedPRDiffState = {
  identityKey: "workspace-1/acme/app/1",
  hasResolved: true,
  files: [
    {
      filename: FIRST_PR_FILENAME,
      status: "modified",
      patch: "@@ -1 +1 @@",
      additions: 1,
      deletions: 1,
    },
  ],
  loading: false,
  error: null,
};

describe("resolvePRDiffView", () => {
  it("masks the previous PR while a newly requested source starts loading", () => {
    expect(resolvePRDiffView(staleState, "workspace-1/acme/app/2")).toEqual({
      files: [],
      loading: true,
      error: null,
    });
  });

  it("clears stale files without loading when no PR is requested", () => {
    expect(resolvePRDiffView(staleState, "")).toEqual({
      files: [],
      loading: false,
      error: null,
    });
  });
});

describe("usePRDiff refresh callback", () => {
  it("keeps the manual refresh callback stable when the automatic refresh key advances", () => {
    requestMock.mockResolvedValue({ files: staleState.files });
    const { result, rerender } = renderHook(
      ({ refreshKey }) => usePRDiff("acme", "app", 1, refreshKey),
      { initialProps: { refreshKey: "old-sync" }, wrapper },
    );
    const refresh = result.current.refresh;

    rerender({ refreshKey: "new-sync" });

    expect(result.current.refresh).toBe(refresh);
  });
});

describe("usePRDiff manual refresh", () => {
  it("keeps resolved files mounted while a manual refresh is pending", async () => {
    const refreshed = deferred<{ files: KeyedPRDiffState["files"] }>();
    requestMock
      .mockResolvedValueOnce({ files: staleState.files })
      .mockReturnValueOnce(refreshed.promise);
    const { result } = renderHook(() => usePRDiff("acme", "app", 1, "synced"), { wrapper });
    await waitFor(() => expect(result.current.files[0]?.filename).toBe(FIRST_PR_FILENAME));

    act(() => {
      result.current.refresh();
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.files[0]?.filename).toBe(FIRST_PR_FILENAME);

    await act(async () => {
      refreshed.resolve({ files: staleState.files });
      await refreshed.promise;
    });
  });

  it("keeps resolved files mounted when a manual refresh has no WebSocket client", async () => {
    requestMock.mockResolvedValueOnce({ files: staleState.files });
    const { result } = renderHook(() => usePRDiff("acme", "app", 1, "synced"), { wrapper });
    await waitFor(() => expect(result.current.files[0]?.filename).toBe(FIRST_PR_FILENAME));
    getWebSocketClientMock.mockReturnValue(null);

    act(() => {
      result.current.refresh();
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.files[0]?.filename).toBe(FIRST_PR_FILENAME);
  });
});

describe("usePRDiff automatic same-PR refresh", () => {
  it("keeps current files mounted without re-entering blocking loading", async () => {
    const initial = deferred<{ files: KeyedPRDiffState["files"] }>();
    const refreshed = deferred<{ files: KeyedPRDiffState["files"] }>();
    requestMock.mockReturnValueOnce(initial.promise).mockReturnValueOnce(refreshed.promise);
    const { result, rerender } = renderHook(
      ({ refreshKey }) => usePRDiff("acme", "app", 1, refreshKey),
      { initialProps: { refreshKey: "old-sync" }, wrapper },
    );

    await act(async () => {
      initial.resolve({ files: staleState.files });
      await initial.promise;
    });
    await waitFor(() => expect(result.current.files[0]?.filename).toBe(FIRST_PR_FILENAME));

    rerender({ refreshKey: "new-sync" });

    expect(result.current.loading).toBe(false);
    expect(result.current.files[0]?.filename).toBe(FIRST_PR_FILENAME);
  });

  it("atomically replaces retained files when the same PR refresh succeeds", async () => {
    const refreshed = deferred<{ files: KeyedPRDiffState["files"] }>();
    requestMock
      .mockResolvedValueOnce({ files: staleState.files })
      .mockReturnValueOnce(refreshed.promise);
    const { result, rerender } = renderHook(
      ({ refreshKey }) => usePRDiff("acme", "app", 1, refreshKey),
      { initialProps: { refreshKey: "old-sync" }, wrapper },
    );
    await waitFor(() => expect(result.current.files[0]?.filename).toBe(FIRST_PR_FILENAME));

    rerender({ refreshKey: "new-sync" });
    expect(result.current.files[0]?.filename).toBe(FIRST_PR_FILENAME);

    await act(async () => {
      refreshed.resolve({
        files: [
          {
            filename: "src/refreshed.ts",
            status: "modified",
            patch: "@@refreshed@@",
            additions: 2,
            deletions: 1,
          },
        ],
      });
      await refreshed.promise;
    });

    await waitFor(() => expect(result.current.files[0]?.filename).toBe("src/refreshed.ts"));
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("retains resolved files when a same-PR background refresh fails", async () => {
    const refreshed = deferred<{ files: KeyedPRDiffState["files"] }>();
    requestMock
      .mockResolvedValueOnce({ files: staleState.files })
      .mockReturnValueOnce(refreshed.promise);
    const { result, rerender } = renderHook(
      ({ refreshKey }) => usePRDiff("acme", "app", 1, refreshKey),
      { initialProps: { refreshKey: "old-sync" }, wrapper },
    );
    await waitFor(() => expect(result.current.files[0]?.filename).toBe(FIRST_PR_FILENAME));

    rerender({ refreshKey: "new-sync" });
    await act(async () => {
      refreshed.reject(new Error("refresh failed"));
      await refreshed.promise.catch(() => undefined);
    });

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.files[0]?.filename).toBe(FIRST_PR_FILENAME);
    expect(result.current.error).toBeNull();
  });
});

describe("usePRDiff initial failure", () => {
  it("exposes an initial request failure for retry", async () => {
    requestMock.mockRejectedValueOnce(new Error("initial failed"));
    const { result } = renderHook(() => usePRDiff("acme", "app", 1, "first-sync"), {
      wrapper,
    });

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.files).toEqual([]);
    expect(result.current.error).toBe("initial failed");
  });
});

describe("usePRDiff request ownership", () => {
  it("ignores a superseded same-PR refresh that resolves last", async () => {
    const superseded = deferred<{ files: KeyedPRDiffState["files"] }>();
    const current = deferred<{ files: KeyedPRDiffState["files"] }>();
    requestMock
      .mockResolvedValueOnce({ files: staleState.files })
      .mockReturnValueOnce(superseded.promise)
      .mockReturnValueOnce(current.promise);
    const { result, rerender } = renderHook(
      ({ refreshKey }) => usePRDiff("acme", "app", 1, refreshKey),
      { initialProps: { refreshKey: "old-sync" }, wrapper },
    );
    await waitFor(() => expect(result.current.files[0]?.filename).toBe(FIRST_PR_FILENAME));

    rerender({ refreshKey: "middle-sync" });
    rerender({ refreshKey: "new-sync" });
    await act(async () => {
      current.resolve({
        files: [
          {
            filename: "src/current.ts",
            status: "modified",
            patch: "@@current@@",
            additions: 1,
            deletions: 0,
          },
        ],
      });
      await current.promise;
    });
    await waitFor(() => expect(result.current.files[0]?.filename).toBe("src/current.ts"));

    await act(async () => {
      superseded.resolve({ files: staleState.files });
      await superseded.promise;
    });
    expect(result.current.files[0]?.filename).toBe("src/current.ts");
  });

  it("masks A immediately on B and ignores A when its request resolves late", async () => {
    const first = deferred<{ files: KeyedPRDiffState["files"] }>();
    const second = deferred<{ files: KeyedPRDiffState["files"] }>();
    requestMock.mockImplementation((_action: string, payload: { number: number }) =>
      payload.number === 1 ? first.promise : second.promise,
    );
    const { result, rerender } = renderHook(
      ({ number }) => usePRDiff("acme", "app", number, "synced"),
      { initialProps: { number: 1 }, wrapper },
    );

    rerender({ number: 2 });
    expect(result.current).toMatchObject({ files: [], loading: true, error: null });

    second.resolve({
      files: [
        {
          filename: "src/second-pr.ts",
          status: "modified",
          patch: "@@second@@",
          additions: 1,
          deletions: 0,
        },
      ],
    });
    await waitFor(() => expect(result.current.files[0]?.filename).toBe("src/second-pr.ts"));

    await act(async () => {
      first.resolve({
        files: [
          {
            filename: FIRST_PR_FILENAME,
            status: "modified",
            patch: "@@first@@",
            additions: 1,
            deletions: 0,
          },
        ],
      });
      await first.promise;
    });

    expect(result.current.files[0]?.filename).toBe("src/second-pr.ts");
  });
});
