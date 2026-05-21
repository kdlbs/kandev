import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";

const getRepoMergeMethodsMock = vi.fn();

vi.mock("@/lib/api/domains/github-api", () => ({
  getRepoMergeMethods: (...args: unknown[]) => getRepoMergeMethodsMock(...args),
}));

// Module-level cache survives across tests in the same module — give each
// test a unique owner so the cache state from one doesn't leak into the next.
let repoCounter = 0;
function uniqueRepo() {
  repoCounter += 1;
  return { owner: `owner-${repoCounter}`, repo: `repo-${repoCounter}` };
}

// Import after mocks so the hook picks up the mocked module.
import { useRepoMergeMethods } from "./use-repo-merge-methods";

afterEach(() => {
  cleanup();
  getRepoMergeMethodsMock.mockReset();
  vi.useRealTimers();
});

describe("useRepoMergeMethods", () => {
  it("returns null when owner or repo are missing", () => {
    const { result } = renderHook(() => useRepoMergeMethods(null, null));
    expect(result.current).toBeNull();
    expect(getRepoMergeMethodsMock).not.toHaveBeenCalled();
  });

  it("returns null while in flight, then the resolved value after settling", async () => {
    const { owner, repo } = uniqueRepo();
    const methods = { merge: false, squash: true, rebase: false };
    getRepoMergeMethodsMock.mockResolvedValueOnce(methods);

    const { result } = renderHook(() => useRepoMergeMethods(owner, repo));
    expect(result.current).toBeNull();

    await waitFor(() => expect(result.current).toEqual(methods));
    expect(getRepoMergeMethodsMock).toHaveBeenCalledTimes(1);
    expect(getRepoMergeMethodsMock).toHaveBeenCalledWith(owner, repo);
  });

  it("returns the cached value on subsequent mounts without refetching", async () => {
    const { owner, repo } = uniqueRepo();
    const methods = { merge: true, squash: false, rebase: false };
    getRepoMergeMethodsMock.mockResolvedValueOnce(methods);

    const first = renderHook(() => useRepoMergeMethods(owner, repo));
    await waitFor(() => expect(first.result.current).toEqual(methods));
    first.unmount();

    const second = renderHook(() => useRepoMergeMethods(owner, repo));
    // Cache hit: synchronously returns the stored value on first render.
    expect(second.result.current).toEqual(methods);
    expect(getRepoMergeMethodsMock).toHaveBeenCalledTimes(1);
  });

  it("coalesces concurrent fetches for the same repo into a single request", async () => {
    const { owner, repo } = uniqueRepo();
    const methods = { merge: true, squash: true, rebase: true };
    getRepoMergeMethodsMock.mockResolvedValueOnce(methods);

    const first = renderHook(() => useRepoMergeMethods(owner, repo));
    const second = renderHook(() => useRepoMergeMethods(owner, repo));

    await waitFor(() => {
      expect(first.result.current).toEqual(methods);
      expect(second.result.current).toEqual(methods);
    });
    // Only one upstream call despite two consumers mounting back-to-back.
    expect(getRepoMergeMethodsMock).toHaveBeenCalledTimes(1);
  });

  it("returns null on fetch failure without throwing or poisoning the cache", async () => {
    const { owner, repo } = uniqueRepo();
    getRepoMergeMethodsMock
      .mockRejectedValueOnce(new Error("network down"))
      .mockResolvedValueOnce({ merge: false, squash: true, rebase: false });

    // Catch unhandled rejections so a regression here (rejection escaping
    // the hook) fails this test instead of leaking into the test runner.
    const unhandled: unknown[] = [];
    const onRejection = (event: PromiseRejectionEvent) => {
      unhandled.push(event.reason);
      event.preventDefault();
    };
    window.addEventListener("unhandledrejection", onRejection);

    const first = renderHook(() => useRepoMergeMethods(owner, repo));
    await waitFor(() => expect(getRepoMergeMethodsMock).toHaveBeenCalledTimes(1));
    expect(first.result.current).toBeNull();
    first.unmount();

    // Failed fetch must NOT have cached null — the next mount retries.
    const second = renderHook(() => useRepoMergeMethods(owner, repo));
    await waitFor(() => expect(getRepoMergeMethodsMock).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(second.result.current).toEqual({ merge: false, squash: true, rebase: false }),
    );

    window.removeEventListener("unhandledrejection", onRejection);
    expect(unhandled).toEqual([]);
  });

  it("refetches after the cache entry expires", async () => {
    const { owner, repo } = uniqueRepo();
    const first = { merge: false, squash: true, rebase: false };
    const second = { merge: true, squash: false, rebase: true };
    getRepoMergeMethodsMock.mockResolvedValueOnce(first).mockResolvedValueOnce(second);

    vi.useFakeTimers({ shouldAdvanceTime: true });
    const start = new Date("2026-01-01T00:00:00Z");
    vi.setSystemTime(start);

    const firstMount = renderHook(() => useRepoMergeMethods(owner, repo));
    await waitFor(() => expect(firstMount.result.current).toEqual(first));
    firstMount.unmount();

    // Move past the 5-minute TTL.
    act(() => {
      vi.setSystemTime(new Date(start.getTime() + 6 * 60 * 1000));
    });

    const secondMount = renderHook(() => useRepoMergeMethods(owner, repo));
    // Expired entry should be evicted on read → null while refetch is in flight.
    expect(secondMount.result.current).toBeNull();
    await waitFor(() => expect(secondMount.result.current).toEqual(second));
    expect(getRepoMergeMethodsMock).toHaveBeenCalledTimes(2);
  });
});
