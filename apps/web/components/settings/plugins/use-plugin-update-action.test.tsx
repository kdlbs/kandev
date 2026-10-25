import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { MarketplaceEntry } from "@/lib/types/plugins";
import { usePluginUpdateAction } from "./use-plugin-update-action";

const ENTRY_ID = "acme";
const CHECKSUM_ERROR = "bad checksum";

function entry(overrides: Partial<MarketplaceEntry> = {}): MarketplaceEntry {
  return {
    id: ENTRY_ID,
    name: "Acme",
    description: "",
    author: "acme",
    categories: [],
    icon_url: "",
    repo_url: "",
    version: "2.0.0",
    min_kandev_version: "",
    package_url: "https://ex/acme-2.0.0.tar.gz",
    package_sha256: "",
    stars: 0,
    updated_at: "",
    install_state: "update_available",
    source_id: "official",
    source_name: "Kandev Official",
    ...overrides,
  };
}

type MarketplaceInstall = (url: string) => Promise<{ ok: boolean; error?: string }>;

// Every id these tests act on is installed unless a case says otherwise.
const INSTALLED: ReadonlySet<string> = new Set([ENTRY_ID, "a", "b"]);

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

describe("usePluginUpdateAction", () => {
  let marketplaceInstall: ReturnType<typeof vi.fn<MarketplaceInstall>>;
  let reloadUpdates: ReturnType<typeof vi.fn<() => Promise<void>>>;

  beforeEach(() => {
    marketplaceInstall = vi.fn<MarketplaceInstall>();
    reloadUpdates = vi.fn<() => Promise<void>>().mockResolvedValue(undefined);
  });

  it("tracks the updating id while the install is in flight, then clears it and re-checks the catalog", async () => {
    let resolveInstall!: (v: { ok: boolean }) => void;
    marketplaceInstall.mockReturnValue(
      new Promise((resolve) => {
        resolveInstall = resolve;
      }),
    );
    const { result } = renderHook(() =>
      usePluginUpdateAction(marketplaceInstall, reloadUpdates, INSTALLED),
    );

    let runPromise!: Promise<void>;
    act(() => {
      runPromise = result.current.runUpdate(entry());
    });
    expect(result.current.updatingIds.has(ENTRY_ID)).toBe(true);

    await act(async () => {
      resolveInstall({ ok: true });
      await runPromise;
    });

    expect(result.current.updatingIds.has(ENTRY_ID)).toBe(false);
    expect(marketplaceInstall).toHaveBeenCalledWith("https://ex/acme-2.0.0.tar.gz");
    expect(reloadUpdates).toHaveBeenCalledTimes(1);
  });

  it("marks a successful install as current before rechecking the catalog", async () => {
    marketplaceInstall.mockResolvedValue({ ok: true });
    const markUpdated = vi.fn();
    const { result } = renderHook(() =>
      usePluginUpdateAction(marketplaceInstall, reloadUpdates, INSTALLED, markUpdated),
    );

    await act(async () => {
      await result.current.runUpdate(entry());
    });

    expect(markUpdated).toHaveBeenCalledWith(ENTRY_ID);
  });

  it("records a per-row error on failure without throwing, and still re-checks the catalog", async () => {
    marketplaceInstall.mockResolvedValue({ ok: false, error: CHECKSUM_ERROR });
    const { result } = renderHook(() =>
      usePluginUpdateAction(marketplaceInstall, reloadUpdates, INSTALLED),
    );

    await act(async () => {
      await result.current.runUpdate(entry());
    });

    expect(result.current.errorsById.get(ENTRY_ID)).toBe(CHECKSUM_ERROR);
    expect(result.current.updatingIds.has(ENTRY_ID)).toBe(false);
    expect(reloadUpdates).toHaveBeenCalledTimes(1);
  });

  it("keeps the row busy until the post-install catalog reload settles", async () => {
    marketplaceInstall.mockResolvedValue({ ok: true });
    const catalogReload = deferred<void>();
    reloadUpdates.mockReturnValueOnce(catalogReload.promise);
    const { result } = renderHook(() =>
      usePluginUpdateAction(marketplaceInstall, reloadUpdates, INSTALLED),
    );

    let runPromise!: Promise<void>;
    act(() => {
      runPromise = result.current.runUpdate(entry());
    });
    await act(async () => {
      await marketplaceInstall.mock.results[0].value;
    });

    expect(result.current.updatingIds.has(ENTRY_ID)).toBe(true);

    await act(async () => {
      catalogReload.resolve();
      await runPromise;
    });
    expect(result.current.updatingIds.has(ENTRY_ID)).toBe(false);
  });
});

describe("usePluginUpdateAction — per-plugin state", () => {
  let marketplaceInstall: ReturnType<typeof vi.fn<MarketplaceInstall>>;
  let reloadUpdates: ReturnType<typeof vi.fn<() => Promise<void>>>;

  beforeEach(() => {
    marketplaceInstall = vi.fn<MarketplaceInstall>();
    reloadUpdates = vi.fn<() => Promise<void>>().mockResolvedValue(undefined);
  });

  it("clears a previous error at the start of a retry", async () => {
    marketplaceInstall.mockResolvedValueOnce({ ok: false, error: CHECKSUM_ERROR });
    const { result } = renderHook(() =>
      usePluginUpdateAction(marketplaceInstall, reloadUpdates, INSTALLED),
    );

    await act(async () => {
      await result.current.runUpdate(entry());
    });
    expect(result.current.errorsById.get(ENTRY_ID)).toBe(CHECKSUM_ERROR);

    let resolveRetry!: (v: { ok: boolean }) => void;
    marketplaceInstall.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveRetry = resolve;
      }),
    );
    let retryPromise!: Promise<void>;
    act(() => {
      retryPromise = result.current.runUpdate(entry());
    });

    expect(result.current.errorsById.has(ENTRY_ID)).toBe(false);

    await act(async () => {
      resolveRetry({ ok: true });
      await retryPromise;
    });
    expect(result.current.errorsById.has(ENTRY_ID)).toBe(false);
  });

  it("tracks each plugin's update independently", async () => {
    marketplaceInstall.mockResolvedValue({ ok: false, error: "boom" });
    const { result } = renderHook(() =>
      usePluginUpdateAction(marketplaceInstall, reloadUpdates, INSTALLED),
    );

    await act(async () => {
      await result.current.runUpdate(entry({ id: "a", package_url: "https://ex/a.tar.gz" }));
    });
    await act(async () => {
      await result.current.runUpdate(entry({ id: "b", package_url: "https://ex/b.tar.gz" }));
    });

    expect(result.current.errorsById.get("a")).toBe("boom");
    expect(result.current.errorsById.get("b")).toBe("boom");
  });
});

describe("usePluginUpdateAction — overlapping updates", () => {
  // Regression: `updatingId` was a single slot, so when two rows updated at
  // once the first install to settle cleared the marker for the other — the
  // still-installing row lost its spinner and its Update/Uninstall controls
  // went live again mid-install, inviting a second concurrent install.
  it("keeps every in-flight row marked busy when two updates overlap", async () => {
    const resolvers: Record<string, (v: { ok: boolean }) => void> = {};
    const marketplaceInstall = vi.fn<MarketplaceInstall>(
      (url: string) =>
        new Promise((resolve) => {
          resolvers[url.includes("/a.") ? "a" : "b"] = resolve;
        }),
    );
    const { result } = renderHook(() =>
      usePluginUpdateAction(
        marketplaceInstall,
        vi.fn<() => Promise<void>>().mockResolvedValue(undefined),
        INSTALLED,
      ),
    );

    let runA!: Promise<void>;
    let runB!: Promise<void>;
    act(() => {
      runA = result.current.runUpdate(entry({ id: "a", package_url: "https://ex/a.tar.gz" }));
    });
    act(() => {
      runB = result.current.runUpdate(entry({ id: "b", package_url: "https://ex/b.tar.gz" }));
    });
    expect(result.current.updatingIds.has("a")).toBe(true);
    expect(result.current.updatingIds.has("b")).toBe(true);

    // A settles first; B is still installing and must stay busy.
    await act(async () => {
      resolvers.a({ ok: true });
      await runA;
    });
    expect(result.current.updatingIds.has("a")).toBe(false);
    expect(result.current.updatingIds.has("b")).toBe(true);

    await act(async () => {
      resolvers.b({ ok: true });
      await runB;
    });
    expect(result.current.updatingIds.size).toBe(0);
  });
});

describe("usePluginUpdateAction — error lifetime", () => {
  let marketplaceInstall: ReturnType<typeof vi.fn<MarketplaceInstall>>;
  let reloadUpdates: ReturnType<typeof vi.fn<() => Promise<void>>>;

  beforeEach(() => {
    marketplaceInstall = vi.fn<MarketplaceInstall>();
    reloadUpdates = vi.fn<() => Promise<void>>().mockResolvedValue(undefined);
  });

  // Regression: uninstalling a plugin whose update failed and installing it
  // again (same id, no page reload) used to resurface the previous copy's
  // error on the new row, reporting a failure for an install that succeeded.
  it("drops a plugin's error once it is no longer installed, and does not resurface it on reinstall", async () => {
    marketplaceInstall.mockResolvedValue({ ok: false, error: CHECKSUM_ERROR });
    const { result, rerender } = renderHook(
      ({ installed }: { installed: ReadonlySet<string> }) =>
        usePluginUpdateAction(marketplaceInstall, reloadUpdates, installed),
      { initialProps: { installed: INSTALLED } },
    );

    await act(async () => {
      await result.current.runUpdate(entry());
    });
    expect(result.current.errorsById.get(ENTRY_ID)).toBe(CHECKSUM_ERROR);

    // Uninstalled: the id leaves the installed set.
    rerender({ installed: new Set<string>() });
    expect(result.current.errorsById.has(ENTRY_ID)).toBe(false);

    // Reinstalled under the same id: still clean.
    rerender({ installed: new Set([ENTRY_ID]) });
    expect(result.current.errorsById.has(ENTRY_ID)).toBe(false);
  });

  it("keeps errors for plugins that are still installed", async () => {
    marketplaceInstall.mockResolvedValue({ ok: false, error: CHECKSUM_ERROR });
    const { result, rerender } = renderHook(
      ({ installed }: { installed: ReadonlySet<string> }) =>
        usePluginUpdateAction(marketplaceInstall, reloadUpdates, installed),
      { initialProps: { installed: INSTALLED } },
    );

    await act(async () => {
      await result.current.runUpdate(entry());
    });

    // A new Set with the same members must not drop a live error.
    rerender({ installed: new Set([ENTRY_ID, "a", "b"]) });
    expect(result.current.errorsById.get(ENTRY_ID)).toBe(CHECKSUM_ERROR);
  });
});
