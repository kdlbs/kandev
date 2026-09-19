import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";

const getMarketplaceCatalog = vi.fn();
const refreshMarketplace = vi.fn();
const REFRESH_UNAVAILABLE = "refresh unavailable";

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

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((next, fail) => {
    resolve = next;
    reject = fail;
  });
  return { promise, resolve, reject };
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

    await act(async () => {
      await result.current.reload();
    });
    await waitFor(() => expect(getMarketplaceCatalog).toHaveBeenCalledTimes(2));
  });

  it("marks an update entry installed locally while a catalog reload is unavailable", async () => {
    getMarketplaceCatalog.mockResolvedValue({
      plugins: [entry("acme", "update_available", "2.0.0")],
      sources: [source()],
    });
    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(result.current.updates.has("acme")).toBe(true));

    act(() => {
      result.current.markUpdated("acme");
    });

    expect(result.current.latestById.get("acme")?.install_state).toBe("installed");
    expect(result.current.updates.has("acme")).toBe(false);
  });
});

describe("usePluginUpdates — checkForUpdates", () => {
  it("busts the marketplace cache before re-fetching, and reports checking/lastCheckedAt", async () => {
    getMarketplaceCatalog.mockResolvedValue({ plugins: [], sources: [] });
    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(result.current.checked).toBe(true));

    let checkPromise!: Promise<boolean>;
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

  it("does not fetch or record a fresh catalog when cache refresh fails", async () => {
    getMarketplaceCatalog.mockResolvedValue({ plugins: [], sources: [source()] });
    refreshMarketplace.mockRejectedValueOnce(new Error(REFRESH_UNAVAILABLE));
    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(result.current.checked).toBe(true));
    const lastCheckedAt = result.current.lastCheckedAt;

    await act(async () => {
      await result.current.checkForUpdates();
    });

    expect(result.current.error).toBe(REFRESH_UNAVAILABLE);
    expect(result.current.lastCheckedAt).toBe(lastCheckedAt);
    expect(getMarketplaceCatalog).toHaveBeenCalledTimes(1);
  });

  it("ignores an older page-load response after a manual check starts", async () => {
    const pageLoad = deferred<{ plugins: unknown[]; sources: unknown[] }>();
    getMarketplaceCatalog.mockReturnValueOnce(pageLoad.promise);
    getMarketplaceCatalog.mockResolvedValueOnce({
      plugins: [entry("acme", "update_available", "2.0.0")],
      sources: [source()],
    });
    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(getMarketplaceCatalog).toHaveBeenCalledTimes(1));

    await act(async () => {
      await result.current.checkForUpdates();
    });
    expect(result.current.latestById.get("acme")?.version).toBe("2.0.0");

    await act(async () => {
      pageLoad.resolve({
        plugins: [entry("acme", "installed", "1.0.0")],
        sources: [source()],
      });
      await pageLoad.promise;
    });

    expect(result.current.latestById.get("acme")?.version).toBe("2.0.0");
  });

  it("keeps the cache-busting check alive when a reload overlaps it", async () => {
    getMarketplaceCatalog.mockResolvedValueOnce({
      plugins: [entry("acme", "installed", "1.0.0")],
      sources: [source()],
    });
    const refresh = deferred<{ refreshed: boolean }>();
    refreshMarketplace.mockReturnValueOnce(refresh.promise);
    getMarketplaceCatalog.mockResolvedValueOnce({
      plugins: [entry("acme", "update_available", "2.0.0")],
      sources: [source()],
    });
    getMarketplaceCatalog.mockResolvedValueOnce({
      plugins: [entry("acme", "installed", "2.0.0")],
      sources: [source()],
    });

    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(result.current.checked).toBe(true));

    let checkPromise!: Promise<boolean>;
    act(() => {
      checkPromise = result.current.checkForUpdates();
    });
    let reloadPromise!: Promise<void>;
    act(() => {
      reloadPromise = result.current.reload();
    });

    // The reload must wait for the refresh + catalog GET pair. If it starts
    // its GET here, it can steal the check's request generation and make the
    // cache-busting check finish without applying a fresh catalog.
    expect(getMarketplaceCatalog).toHaveBeenCalledTimes(1);

    await act(async () => {
      refresh.resolve({ refreshed: true });
      await checkPromise;
      await reloadPromise;
    });

    expect(result.current.latestById.get("acme")?.version).toBe("2.0.0");
    expect(result.current.updates.has("acme")).toBe(false);
    expect(getMarketplaceCatalog).toHaveBeenCalledTimes(3);
  });
});

describe("usePluginUpdates — overlapping checks", () => {
  it("does not reload stale catalog data after an overlapping refresh fails", async () => {
    getMarketplaceCatalog.mockResolvedValue({
      plugins: [entry("acme", "update_available", "2.0.0")],
      sources: [source()],
    });
    const refresh = deferred<{ refreshed: boolean }>();
    refreshMarketplace.mockReturnValueOnce(refresh.promise);

    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(result.current.updates.has("acme")).toBe(true));
    const lastCheckedAt = result.current.lastCheckedAt;

    let checkPromise!: Promise<boolean>;
    act(() => {
      checkPromise = result.current.checkForUpdates();
    });
    let reloadPromise!: Promise<void>;
    act(() => {
      reloadPromise = result.current.reload();
    });

    await act(async () => {
      refresh.reject(new Error(REFRESH_UNAVAILABLE));
      await checkPromise;
      await reloadPromise;
    });

    expect(result.current.error).toBe(REFRESH_UNAVAILABLE);
    expect(result.current.lastCheckedAt).toBe(lastCheckedAt);
    expect(result.current.updates.has("acme")).toBe(true);
    expect(getMarketplaceCatalog).toHaveBeenCalledTimes(1);
  });

  it("marks the installed plugin current when its overlapping refresh fails", async () => {
    getMarketplaceCatalog.mockResolvedValue({
      plugins: [entry("acme", "update_available", "2.0.0")],
      sources: [source()],
    });
    const refresh = deferred<{ refreshed: boolean }>();
    refreshMarketplace.mockReturnValueOnce(refresh.promise);

    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(result.current.updates.has("acme")).toBe(true));
    const lastCheckedAt = result.current.lastCheckedAt;

    let checkPromise!: Promise<boolean>;
    act(() => {
      checkPromise = result.current.checkForUpdates();
    });
    let reloadPromise!: Promise<void>;
    act(() => {
      reloadPromise = result.current.reload("acme");
    });

    await act(async () => {
      refresh.reject(new Error(REFRESH_UNAVAILABLE));
      await checkPromise;
      await reloadPromise;
    });

    expect(result.current.error).toBe(REFRESH_UNAVAILABLE);
    expect(result.current.lastCheckedAt).toBe(lastCheckedAt);
    expect(result.current.latestById.get("acme")?.install_state).toBe("installed");
    expect(result.current.updates.has("acme")).toBe(false);
    expect(getMarketplaceCatalog).toHaveBeenCalledTimes(1);
  });

  it("does not replace a failed check with a stale catalog on a later install reload", async () => {
    getMarketplaceCatalog.mockResolvedValue({
      plugins: [entry("acme", "update_available", "2.0.0")],
      sources: [source()],
    });
    refreshMarketplace.mockRejectedValueOnce(new Error(REFRESH_UNAVAILABLE));

    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(result.current.updates.has("acme")).toBe(true));
    const lastCheckedAt = result.current.lastCheckedAt;

    await act(async () => {
      await result.current.checkForUpdates();
    });
    await act(async () => {
      await result.current.reload("acme");
    });

    expect(result.current.error).toBe(REFRESH_UNAVAILABLE);
    expect(result.current.lastCheckedAt).toBe(lastCheckedAt);
    expect(result.current.latestById.get("acme")?.install_state).toBe("installed");
    expect(result.current.updates.has("acme")).toBe(false);
    expect(getMarketplaceCatalog).toHaveBeenCalledTimes(1);
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

  // Regression: the backend omits a failing source's entries entirely, which is
  // indistinguishable from "this plugin was delisted" unless the partial result
  // is flagged. Without `sourcesDegraded` the row claimed "not in the
  // marketplace" for a plugin whose only source was merely unreachable.
  it("flags a partially degraded catalog, keeps the healthy sources' entries, and reports the reason", async () => {
    getMarketplaceCatalog.mockResolvedValue({
      plugins: [entry("a", "installed", "1.0.0")],
      sources: [source(), source({ id: "local", healthy: false, error: "connection refused" })],
    });
    const { result } = renderHook(() => usePluginUpdates());

    await waitFor(() => expect(result.current.checked).toBe(true));
    expect(result.current.sourcesDegraded).toBe(true);
    expect(result.current.error).toBe("connection refused");
    expect(result.current.latestById.has("a")).toBe(true);
    expect(result.current.lastCheckedAt).not.toBeNull();
  });

  it("does not flag degradation when every enabled source is healthy", async () => {
    getMarketplaceCatalog.mockResolvedValue({
      plugins: [],
      sources: [source(), source({ id: "local", enabled: false })],
    });
    const { result } = renderHook(() => usePluginUpdates());

    await waitFor(() => expect(result.current.checked).toBe(true));
    expect(result.current.sourcesDegraded).toBe(false);
    expect(result.current.error).toBeNull();
  });

  // Regression: with no enabled source nothing was queried, so an empty
  // catalog is evidence of nothing — not of every plugin being delisted. The
  // guard on the all-unhealthy branch deliberately skips this case, which used
  // to drop it into the fully-successful branch and flip every installed row
  // to "not in the marketplace" under a clean "last checked" line.
  it("flags a catalog with no enabled source as degraded and explains why", async () => {
    getMarketplaceCatalog.mockResolvedValue({
      plugins: [],
      sources: [source({ enabled: false }), source({ id: "local", enabled: false })],
    });
    const { result } = renderHook(() => usePluginUpdates());

    await waitFor(() => expect(result.current.checked).toBe(true));
    expect(result.current.sourcesDegraded).toBe(true);
    expect(result.current.error).toBe(
      "No marketplace sources are enabled, so versions can't be checked. Enable one under Browse > Sources.",
    );
  });

  // The early return above skipped `setSourcesDegraded`, so a total outage
  // after a healthy check left the flag stale at `false` and rows kept
  // claiming removal on data no source had confirmed this round.
  it("flags degradation when every enabled source is unhealthy", async () => {
    getMarketplaceCatalog.mockResolvedValue({
      plugins: [],
      sources: [source({ healthy: false, error: "timeout" })],
    });
    const { result } = renderHook(() => usePluginUpdates());

    await waitFor(() => expect(result.current.error).toBe("timeout"));
    expect(result.current.sourcesDegraded).toBe(true);
  });

  it("clears a previous error once a later check succeeds", async () => {
    getMarketplaceCatalog.mockRejectedValueOnce(new Error("offline"));
    getMarketplaceCatalog.mockResolvedValueOnce({ plugins: [], sources: [source()] });
    const { result } = renderHook(() => usePluginUpdates());
    await waitFor(() => expect(result.current.error).toBe("offline"));

    await act(async () => {
      await result.current.checkForUpdates();
    });

    expect(result.current.error).toBeNull();
    expect(result.current.checked).toBe(true);
  });
});
