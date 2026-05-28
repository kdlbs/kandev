import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";

const fetchRepoBranchesMock = vi.fn();

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchRepoBranches: (...args: unknown[]) => fetchRepoBranchesMock(...args),
}));

// Import after mocks so the hook picks up the mocked module.
import { useBranchesByURL } from "./use-branches-by-url";

afterEach(() => {
  cleanup();
  fetchRepoBranchesMock.mockReset();
  vi.useRealTimers();
});

const REPO_A = "https://github.com/acme/site";
const REPO_B = "https://github.com/acme/api";

describe("useBranchesByURL", () => {
  it("fetches branches once per unique URL when ensure() is called", async () => {
    fetchRepoBranchesMock.mockImplementation((_owner: string, repo: string) => {
      return Promise.resolve({
        branches: [{ name: repo === "site" ? "main" : "develop" }],
      });
    });

    const { result } = renderHook(() => useBranchesByURL());

    act(() => {
      result.current.ensure(REPO_A);
      result.current.ensure(REPO_B);
    });

    await waitFor(() => {
      expect(result.current.branches(REPO_A)).toHaveLength(1);
      expect(result.current.branches(REPO_B)).toHaveLength(1);
    });

    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(2);
    expect(result.current.branches(REPO_A)[0]).toMatchObject({ name: "main", type: "remote" });
    expect(result.current.branches(REPO_B)[0]).toMatchObject({ name: "develop", type: "remote" });
  });

  it("dedupes concurrent ensure() calls for the same URL into a single fetch", async () => {
    fetchRepoBranchesMock.mockResolvedValue({ branches: [{ name: "main" }] });

    const { result } = renderHook(() => useBranchesByURL());

    act(() => {
      result.current.ensure(REPO_A);
      result.current.ensure(REPO_A);
      result.current.ensure(REPO_A);
    });

    await waitFor(() => expect(result.current.branches(REPO_A)).toHaveLength(1));
    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1);
  });

  it("reports loading(url) true during fetch and false after settle", async () => {
    let resolveFetch: ((v: { branches: { name: string }[] }) => void) | null = null;
    fetchRepoBranchesMock.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveFetch = resolve;
        }),
    );

    const { result } = renderHook(() => useBranchesByURL());

    act(() => {
      result.current.ensure(REPO_A);
    });

    await waitFor(() => expect(result.current.loading(REPO_A)).toBe(true));

    act(() => {
      resolveFetch?.({ branches: [{ name: "main" }] });
    });

    await waitFor(() => expect(result.current.loading(REPO_A)).toBe(false));
    expect(result.current.branches(REPO_A)).toHaveLength(1);
  });

  it("ignores ensure() with empty string and treats it as a clear", async () => {
    fetchRepoBranchesMock.mockResolvedValue({ branches: [{ name: "main" }] });

    const { result } = renderHook(() => useBranchesByURL());

    act(() => {
      result.current.ensure(REPO_A);
    });
    await waitFor(() => expect(result.current.branches(REPO_A)).toHaveLength(1));

    act(() => {
      result.current.ensure("");
    });

    // Passing "" should not trigger an additional fetch.
    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1);
  });

  it("returns an empty array for an unknown URL", () => {
    const { result } = renderHook(() => useBranchesByURL());
    expect(result.current.branches("https://github.com/who/what")).toEqual([]);
    expect(result.current.loading("https://github.com/who/what")).toBe(false);
  });

  it("does not re-fetch when ensure() is called for an already-loaded URL", async () => {
    fetchRepoBranchesMock.mockResolvedValue({ branches: [{ name: "main" }] });

    const { result } = renderHook(() => useBranchesByURL());

    act(() => {
      result.current.ensure(REPO_A);
    });
    await waitFor(() => expect(result.current.branches(REPO_A)).toHaveLength(1));

    act(() => {
      result.current.ensure(REPO_A);
    });

    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1);
  });
});

describe("useBranchesByURL — failure & invalidation", () => {
  it("retries on the next ensure() after a failed fetch", async () => {
    // First fetch rejects — the hook MUST NOT mark the URL as loaded, so a
    // subsequent ensure() for the same URL triggers a fresh fetch instead
    // of short-circuiting on the cached failure.
    fetchRepoBranchesMock.mockRejectedValueOnce(new Error("network boom"));

    const { result } = renderHook(() => useBranchesByURL());

    act(() => {
      result.current.ensure(REPO_A);
    });
    await waitFor(() => expect(result.current.loading(REPO_A)).toBe(false));
    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(1);
    expect(result.current.branches(REPO_A)).toEqual([]);

    fetchRepoBranchesMock.mockResolvedValueOnce({ branches: [{ name: "main" }] });
    act(() => {
      result.current.ensure(REPO_A);
    });
    await waitFor(() => expect(result.current.branches(REPO_A)).toHaveLength(1));
    expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(2);
  });

  it("clear(url) lets the next ensure() refetch a successfully loaded URL", async () => {
    fetchRepoBranchesMock.mockResolvedValue({ branches: [{ name: "main" }] });

    const { result } = renderHook(() => useBranchesByURL());

    act(() => {
      result.current.ensure(REPO_A);
    });
    await waitFor(() => expect(result.current.branches(REPO_A)).toHaveLength(1));

    act(() => {
      result.current.clear(REPO_A);
    });
    expect(result.current.branches(REPO_A)).toEqual([]);

    act(() => {
      result.current.ensure(REPO_A);
    });
    await waitFor(() => expect(fetchRepoBranchesMock).toHaveBeenCalledTimes(2));
  });
});
