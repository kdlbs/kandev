import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";

const getMarketplaceCatalog = vi.fn();
const refreshMarketplace = vi.fn();

vi.mock("@/lib/api/domains/marketplace-api", () => ({
  getMarketplaceCatalog: (...args: unknown[]) => getMarketplaceCatalog(...args),
  refreshMarketplace: (...args: unknown[]) => refreshMarketplace(...args),
}));

import { usePluginUpdates } from "./use-plugin-updates";

function entry(id: string, install_state: string, version = "2.0.0") {
  return { id, install_state, version, package_url: `https://ex/${id}.tar.gz` };
}

function source(overrides: Record<string, unknown> = {}) {
  return {
    id: "official",
    name: "Official",
    url: "https://ex",
    enabled: true,
    builtin: true,
    ...overrides,
  };
}

beforeEach(() => {
  getMarketplaceCatalog.mockReset();
  refreshMarketplace.mockReset();
  refreshMarketplace.mockResolvedValue({ refreshed: true });
});

afterEach(() => cleanup());

describe("usePluginUpdates — latestById", () => {
  it("keeps both installed and update_available entries, keyed by id, and derives the update_available subset", async () => {
    getMarketplaceCatalog.mockResolvedValue({
      plugins: [
        entry("a", "update_available"),
        entry("b", "installed", "1.0.0"),
        entry("c", "available"),
      ],
      sources: [source()],
    });

    const { result } = renderHook(() => usePluginUpdates());

    await waitFor(() => expect(result.current.checked).toBe(true));
    expect(result.current.latestById.has("a")).toBe(true);
    expect(result.current.latestById.has("b")).toBe(true);
    expect(result.current.latestById.has("c")).toBe(false);
    expect(result.current.updates.has("a")).toBe(true);
    expect(result.current.updates.has("b")).toBe(false);
    expect(result.current.error).toBeNull();
    expect(result.current.lastCheckedAt).not.toBeNull();
  });

  it("is unchecked and empty before the first catalog response resolves", () => {
    getMarketplaceCatalog.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => usePluginUpdates());
    expect(result.current.checked).toBe(false);
    expect(result.current.latestById.size).toBe(0);
  });

  it("reload re-fetches the catalog", async () => {
    getMarketplaceCatalog.mockResolvedValue({ plugins: [], sources: [] });
    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(getMarketplaceCatalog).toHaveBeenCalledTimes(1));

    act(() => result.current.reload());
    await waitFor(() => expect(getMarketplaceCatalog).toHaveBeenCalledTimes(2));
  });
});

describe("usePluginUpdates — checkForUpdates", () => {
  it("busts the marketplace cache before re-fetching, and reports checking/lastCheckedAt", async () => {
    getMarketplaceCatalog.mockResolvedValue({ plugins: [], sources: [] });
    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(result.current.checked).toBe(true));

    let checkPromise!: Promise<void>;
    act(() => {
      checkPromise = result.current.checkForUpdates();
    });
    expect(result.current.checking).toBe(true);

    await act(async () => {
      await checkPromise;
    });

    expect(result.current.checking).toBe(false);
    expect(refreshMarketplace).toHaveBeenCalledTimes(1);
    const refreshOrder = refreshMarketplace.mock.invocationCallOrder[0];
    const catalogOrder = getMarketplaceCatalog.mock.invocationCallOrder.at(-1);
    expect(refreshOrder).toBeLessThan(catalogOrder as number);
  });
});

describe("usePluginUpdates — failure isolation", () => {
  it("sets error (not a throw) when the catalog fetch rejects, and never breaks updates", async () => {
    getMarketplaceCatalog.mockRejectedValue(new Error("marketplace is unavailable"));
    const { result } = renderHook(() => usePluginUpdates());

    await waitFor(() => expect(result.current.error).toBe("marketplace is unavailable"));
    expect(result.current.updates.size).toBe(0);
  });

  it("sets error when every enabled source is unhealthy, even though the request itself succeeded", async () => {
    getMarketplaceCatalog.mockResolvedValue({
      plugins: [],
      sources: [
        source({ healthy: false, error: "timeout" }),
        source({ id: "extra", enabled: false }),
      ],
    });
    const { result } = renderHook(() => usePluginUpdates());

    await waitFor(() => expect(result.current.error).toBe("timeout"));
  });

  it("clears a previous error once a later check succeeds", async () => {
    getMarketplaceCatalog.mockRejectedValueOnce(new Error("offline"));
    getMarketplaceCatalog.mockResolvedValueOnce({ plugins: [], sources: [] });
    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(result.current.error).toBe("offline"));

    await act(async () => {
      await result.current.checkForUpdates();
    });

    expect(result.current.error).toBeNull();
    expect(result.current.checked).toBe(true);
  });
});
