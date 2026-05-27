import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";

const fetchAccessibleReposMock = vi.fn();

vi.mock("@/lib/api/domains/github-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/github-api")>(
    "@/lib/api/domains/github-api",
  );
  return {
    ...actual,
    fetchAccessibleRepos: (...args: unknown[]) => fetchAccessibleReposMock(...args),
  };
});

// Import AFTER mocks so the hook picks up the mocked module. `GitHubUnavailableError`
// is re-exported from the real module so `instanceof` still works inside the hook.
import { useAccessibleRepos } from "./use-accessible-repos";
import { GitHubUnavailableError } from "@/lib/api/domains/github-api";

afterEach(() => {
  cleanup();
  fetchAccessibleReposMock.mockReset();
  vi.useRealTimers();
});

function makeRepo(name: string) {
  return {
    provider: "github" as const,
    owner: "acme",
    name,
    full_name: `acme/${name}`,
    private: false,
  };
}

describe("useAccessibleRepos — fetching & debouncing", () => {
  it("fetches once on mount for the initial empty query", async () => {
    fetchAccessibleReposMock.mockResolvedValueOnce([makeRepo("site"), makeRepo("api")]);

    const { result } = renderHook(() => useAccessibleRepos());

    await waitFor(() => expect(result.current.repos).toHaveLength(2));
    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
    expect(result.current.unavailable).toBe(false);
  });

  it("debounces and coalesces rapid search() calls into a single fetch", async () => {
    fetchAccessibleReposMock.mockResolvedValue([makeRepo("first")]);
    vi.useFakeTimers({ shouldAdvanceTime: true });

    const { result } = renderHook(() => useAccessibleRepos());

    // Initial fetch settles immediately.
    await act(async () => {
      await Promise.resolve();
    });

    fetchAccessibleReposMock.mockClear();
    fetchAccessibleReposMock.mockResolvedValue([makeRepo("foo")]);

    act(() => {
      result.current.search("f");
      result.current.search("fo");
      result.current.search("foo");
    });

    // Inside the debounce window: no fetch yet.
    expect(fetchAccessibleReposMock).not.toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(260);
      await Promise.resolve();
    });

    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1);
    const lastCall = fetchAccessibleReposMock.mock.calls.at(-1)!;
    expect((lastCall[0] as { q?: string }).q).toBe("foo");
  });

  it("aborts the in-flight fetch when search() switches to a new query", async () => {
    let firstAborted = false;
    fetchAccessibleReposMock.mockImplementationOnce(
      (opts: { signal?: AbortSignal }) =>
        new Promise((_resolve, reject) => {
          opts.signal?.addEventListener("abort", () => {
            firstAborted = true;
            reject(new DOMException("Aborted", "AbortError"));
          });
        }),
    );

    const { result } = renderHook(() => useAccessibleRepos("foo"));

    // Wait for the debounced fetch to start.
    await waitFor(() => expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1));

    fetchAccessibleReposMock.mockResolvedValueOnce([makeRepo("bar")]);
    act(() => {
      result.current.search("bar");
    });

    await waitFor(() => expect(firstAborted).toBe(true));
    await waitFor(() => expect(result.current.repos).toEqual([makeRepo("bar")]));
  });
});

describe("useAccessibleRepos — errors, cache & unmount", () => {
  it("surfaces GitHubUnavailableError as unavailable: true (and not error)", async () => {
    fetchAccessibleReposMock.mockRejectedValueOnce(new GitHubUnavailableError("nope"));

    const { result } = renderHook(() => useAccessibleRepos());

    await waitFor(() => expect(result.current.unavailable).toBe(true));
    expect(result.current.error).toBeNull();
    expect(result.current.loading).toBe(false);
    expect(result.current.repos).toEqual([]);
  });

  it("surfaces non-unavailable errors via the error field", async () => {
    fetchAccessibleReposMock.mockRejectedValueOnce(new Error("boom"));

    const { result } = renderHook(() => useAccessibleRepos());

    await waitFor(() => expect(result.current.error).toBeInstanceOf(Error));
    expect(result.current.unavailable).toBe(false);
    expect(result.current.loading).toBe(false);
  });

  it("caches results by query — repeating a query does not re-fetch", async () => {
    fetchAccessibleReposMock.mockResolvedValueOnce([makeRepo("initial")]);

    const { result } = renderHook(() => useAccessibleRepos());

    await waitFor(() => expect(result.current.repos).toHaveLength(1));

    fetchAccessibleReposMock.mockResolvedValueOnce([makeRepo("foo-1")]);
    act(() => result.current.search("foo"));
    await waitFor(() => expect(result.current.repos).toEqual([makeRepo("foo-1")]));
    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(2);

    // Switch away and back — second 'foo' must reuse the cache.
    fetchAccessibleReposMock.mockResolvedValueOnce([makeRepo("bar-1")]);
    act(() => result.current.search("bar"));
    await waitFor(() => expect(result.current.repos).toEqual([makeRepo("bar-1")]));
    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(3);

    act(() => result.current.search("foo"));
    // Cache hit: no new fetch and the previous foo result is restored synchronously.
    await waitFor(() => expect(result.current.repos).toEqual([makeRepo("foo-1")]));
    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(3);
  });

  it("treats whitespace-only queries as empty and reuses the initial cache entry", async () => {
    fetchAccessibleReposMock.mockResolvedValueOnce([makeRepo("initial")]);

    const { result } = renderHook(() => useAccessibleRepos());

    // Initial empty-query fetch settles.
    await waitFor(() => expect(result.current.repos).toEqual([makeRepo("initial")]));
    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1);

    act(() => result.current.search("   "));

    // Whitespace collapses to "" — same cache entry, no extra fetch.
    await waitFor(() => expect(result.current.repos).toEqual([makeRepo("initial")]));
    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1);
  });

  it("aborts the in-flight fetch on unmount", async () => {
    let aborted = false;
    fetchAccessibleReposMock.mockImplementationOnce(
      (opts: { signal?: AbortSignal }) =>
        new Promise((_resolve, reject) => {
          opts.signal?.addEventListener("abort", () => {
            aborted = true;
            reject(new DOMException("Aborted", "AbortError"));
          });
        }),
    );

    const { unmount } = renderHook(() => useAccessibleRepos());

    await waitFor(() => expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1));
    unmount();
    await waitFor(() => expect(aborted).toBe(true));
  });
});
