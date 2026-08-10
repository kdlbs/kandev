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

describe("usePluginUpdateAction", () => {
  let marketplaceInstall: ReturnType<typeof vi.fn<MarketplaceInstall>>;
  let reloadUpdates: ReturnType<typeof vi.fn<() => void>>;

  beforeEach(() => {
    marketplaceInstall = vi.fn<MarketplaceInstall>();
    reloadUpdates = vi.fn<() => void>();
  });

  it("tracks updatingId while the install is in flight, then clears it and re-checks the catalog", async () => {
    let resolveInstall!: (v: { ok: boolean }) => void;
    marketplaceInstall.mockReturnValue(
      new Promise((resolve) => {
        resolveInstall = resolve;
      }),
    );
    const { result } = renderHook(() => usePluginUpdateAction(marketplaceInstall, reloadUpdates));

    let runPromise!: Promise<void>;
    act(() => {
      runPromise = result.current.runUpdate(entry());
    });
    expect(result.current.updatingId).toBe(ENTRY_ID);

    await act(async () => {
      resolveInstall({ ok: true });
      await runPromise;
    });

    expect(result.current.updatingId).toBeNull();
    expect(marketplaceInstall).toHaveBeenCalledWith("https://ex/acme-2.0.0.tar.gz");
    expect(reloadUpdates).toHaveBeenCalledTimes(1);
  });

  it("records a per-row error on failure without throwing, and still re-checks the catalog", async () => {
    marketplaceInstall.mockResolvedValue({ ok: false, error: CHECKSUM_ERROR });
    const { result } = renderHook(() => usePluginUpdateAction(marketplaceInstall, reloadUpdates));

    await act(async () => {
      await result.current.runUpdate(entry());
    });

    expect(result.current.errorsById.get(ENTRY_ID)).toBe(CHECKSUM_ERROR);
    expect(result.current.updatingId).toBeNull();
    expect(reloadUpdates).toHaveBeenCalledTimes(1);
  });

  it("clears a previous error at the start of a retry", async () => {
    marketplaceInstall.mockResolvedValueOnce({ ok: false, error: CHECKSUM_ERROR });
    const { result } = renderHook(() => usePluginUpdateAction(marketplaceInstall, reloadUpdates));

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
    const { result } = renderHook(() => usePluginUpdateAction(marketplaceInstall, reloadUpdates));

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
