import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";

const getMarketplaceCatalog = vi.fn();
const refreshMarketplace = vi.fn();

vi.mock("@/lib/api/domains/marketplace-api", () => ({
  getMarketplaceCatalog: (...args: unknown[]) => getMarketplaceCatalog(...args),
  refreshMarketplace: (...args: unknown[]) => refreshMarketplace(...args),
}));

import { useMarketplace } from "./use-marketplace";

const CATALOG = { plugins: [], sources: [] };

beforeEach(() => {
  getMarketplaceCatalog.mockReset().mockResolvedValue(CATALOG);
  refreshMarketplace.mockReset().mockResolvedValue({ refreshed: true });
});

afterEach(() => cleanup());

describe("useMarketplace", () => {
  it("fetches the catalog on mount with the query", async () => {
    const { result } = renderHook(() => useMarketplace({ q: "stats", sort: "stars" }));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(getMarketplaceCatalog).toHaveBeenCalledWith({
      q: "stats",
      category: undefined,
      sort: "stars",
    });
    expect(result.current.catalog).toEqual(CATALOG);
    expect(result.current.error).toBeNull();
  });

  it("re-fetches when the query changes", async () => {
    const { result, rerender } = renderHook((query) => useMarketplace(query), {
      initialProps: { q: "a" } as { q?: string },
    });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(getMarketplaceCatalog).toHaveBeenCalledTimes(1);

    rerender({ q: "b" });
    await waitFor(() => expect(getMarketplaceCatalog).toHaveBeenCalledTimes(2));
    expect(getMarketplaceCatalog).toHaveBeenLastCalledWith({
      q: "b",
      category: undefined,
      sort: undefined,
    });
  });

  it("reload refreshes the backend cache then re-fetches", async () => {
    const { result } = renderHook(() => useMarketplace({}));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.reload();
    });

    expect(refreshMarketplace).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(getMarketplaceCatalog).toHaveBeenCalledTimes(2));
  });

  it("surfaces a fetch error", async () => {
    getMarketplaceCatalog.mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useMarketplace({}));
    await waitFor(() => expect(result.current.error).toBe("boom"));
  });
});
