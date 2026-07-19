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

function makeRepo(name: string, owner: string = "acme") {
  return {
    provider: "github" as const,
    owner,
    name,
    full_name: `${owner}/${name}`,
    default_branch: "main",
    private: false,
  };
}

describe("useAccessibleRepos — initial fetch", () => {
  it("fetches once on mount with limit=100 and an empty query", async () => {
    fetchAccessibleReposMock.mockResolvedValueOnce([makeRepo("site"), makeRepo("api")]);

    const { result } = renderHook(() => useAccessibleRepos("ws-1"));

    await waitFor(() => expect(result.current.repos).toHaveLength(2));
    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1);
    const args = fetchAccessibleReposMock.mock.calls[0]![0] as { q?: string; limit?: number };
    expect(args.q).toBeUndefined();
    expect(args.limit).toBe(100);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
    expect(result.current.unavailable).toBe(false);
  });

  it("reports loading=true while the initial fetch is in flight", async () => {
    let resolve: ((repos: ReturnType<typeof makeRepo>[]) => void) | undefined;
    fetchAccessibleReposMock.mockImplementationOnce(
      () =>
        new Promise((res) => {
          resolve = res;
        }),
    );

    const { result } = renderHook(() => useAccessibleRepos("ws-1"));

    expect(result.current.loading).toBe(true);
    expect(result.current.repos).toEqual([]);

    await act(async () => {
      resolve!([makeRepo("foo")]);
      await Promise.resolve();
    });

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.repos).toEqual([makeRepo("foo")]);
  });
});

describe("useAccessibleRepos — client-side filtering", () => {
  it("filters by case-insensitive substring against full_name without re-fetching", async () => {
    fetchAccessibleReposMock.mockResolvedValueOnce([
      makeRepo("kandev", "kdlbs"),
      makeRepo("widget", "acme"),
      makeRepo("api", "acme"),
    ]);

    const { result } = renderHook(() => useAccessibleRepos("ws-1"));
    await waitFor(() => expect(result.current.repos).toHaveLength(3));
    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1);

    act(() => result.current.search("kandev"));
    await waitFor(() =>
      expect(result.current.repos.map((r) => r.full_name)).toEqual(["kdlbs/kandev"]),
    );

    // Case-insensitive.
    act(() => result.current.search("KDLBS"));
    await waitFor(() =>
      expect(result.current.repos.map((r) => r.full_name)).toEqual(["kdlbs/kandev"]),
    );

    // Matches against owner OR name (substring against full_name).
    act(() => result.current.search("acme/"));
    await waitFor(() =>
      expect(result.current.repos.map((r) => r.full_name).sort()).toEqual([
        "acme/api",
        "acme/widget",
      ]),
    );

    // Subsequent search() calls trigger ZERO additional fetches.
    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1);
  });

  it("empty / whitespace query returns the full list", async () => {
    fetchAccessibleReposMock.mockResolvedValueOnce([makeRepo("a"), makeRepo("b")]);

    const { result } = renderHook(() => useAccessibleRepos("ws-1"));
    await waitFor(() => expect(result.current.repos).toHaveLength(2));

    act(() => result.current.search("foo"));
    await waitFor(() => expect(result.current.repos).toHaveLength(0));

    act(() => result.current.search("   "));
    await waitFor(() => expect(result.current.repos).toHaveLength(2));
    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1);
  });

  it("repeated identical search() is idempotent (no extra fetches, stable result)", async () => {
    fetchAccessibleReposMock.mockResolvedValueOnce([makeRepo("foo"), makeRepo("bar")]);

    const { result } = renderHook(() => useAccessibleRepos("ws-1"));
    await waitFor(() => expect(result.current.repos).toHaveLength(2));

    act(() => result.current.search("foo"));
    await waitFor(() => expect(result.current.repos).toHaveLength(1));
    act(() => result.current.search("foo"));
    act(() => result.current.search("foo"));
    expect(result.current.repos.map((r) => r.full_name)).toEqual(["acme/foo"]);
    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1);
  });

  it("rapid search() calls do NOT trigger any backend requests", async () => {
    fetchAccessibleReposMock.mockResolvedValueOnce([makeRepo("foo")]);

    const { result } = renderHook(() => useAccessibleRepos("ws-1"));
    await waitFor(() => expect(result.current.repos).toHaveLength(1));

    act(() => {
      result.current.search("f");
      result.current.search("fo");
      result.current.search("foo");
      result.current.search("fooo");
      result.current.search("");
    });

    // Only the original mount fetch ever happened.
    expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1);
  });
});

describe("useAccessibleRepos — errors & unmount", () => {
  it("surfaces GitHubUnavailableError as unavailable: true (and not error)", async () => {
    fetchAccessibleReposMock.mockRejectedValueOnce(new GitHubUnavailableError("nope"));

    const { result } = renderHook(() => useAccessibleRepos("ws-1"));

    await waitFor(() => expect(result.current.unavailable).toBe(true));
    expect(result.current.error).toBeNull();
    expect(result.current.loading).toBe(false);
    expect(result.current.repos).toEqual([]);
  });

  it("surfaces non-unavailable errors via the error field", async () => {
    fetchAccessibleReposMock.mockRejectedValueOnce(new Error("boom"));

    const { result } = renderHook(() => useAccessibleRepos("ws-1"));

    await waitFor(() => expect(result.current.error).toBeInstanceOf(Error));
    expect(result.current.unavailable).toBe(false);
    expect(result.current.loading).toBe(false);
    expect(result.current.repos).toEqual([]);
  });

  it("aborts the in-flight initial fetch on unmount", async () => {
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

    const { unmount } = renderHook(() => useAccessibleRepos("ws-1"));

    await waitFor(() => expect(fetchAccessibleReposMock).toHaveBeenCalledTimes(1));
    unmount();
    await waitFor(() => expect(aborted).toBe(true));
  });
});
