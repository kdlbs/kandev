import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";

const getMarketplaceCatalog = vi.fn();

vi.mock("@/lib/api/domains/marketplace-api", () => ({
  getMarketplaceCatalog: (...args: unknown[]) => getMarketplaceCatalog(...args),
}));

import { usePluginUpdates } from "./use-plugin-updates";

function entry(id: string, install_state: string, version = "2.0.0") {
  return { id, install_state, version, package_url: `https://ex/${id}.tar.gz` };
}

beforeEach(() => {
  getMarketplaceCatalog.mockReset();
});

afterEach(() => cleanup());

describe("usePluginUpdates", () => {
  it("keeps only update_available entries, keyed by id", async () => {
    getMarketplaceCatalog.mockResolvedValue({
      plugins: [entry("a", "update_available"), entry("b", "installed"), entry("c", "available")],
      sources: [],
    });

    const { result } = renderHook(() => usePluginUpdates());

    await waitFor(() => expect(result.current.updates.size).toBe(1));
    expect(result.current.updates.has("a")).toBe(true);
    expect(result.current.updates.get("a")?.version).toBe("2.0.0");
    expect(result.current.updates.has("b")).toBe(false);
  });

  it("reload re-fetches the catalog", async () => {
    getMarketplaceCatalog.mockResolvedValue({ plugins: [], sources: [] });
    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(getMarketplaceCatalog).toHaveBeenCalledTimes(1));

    act(() => result.current.reload());
    await waitFor(() => expect(getMarketplaceCatalog).toHaveBeenCalledTimes(2));
  });

  it("swallows a catalog fetch error (no updates, no throw)", async () => {
    getMarketplaceCatalog.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(getMarketplaceCatalog).toHaveBeenCalled());
    expect(result.current.updates.size).toBe(0);
  });
});
