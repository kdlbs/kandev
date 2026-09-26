import { renderHook, act, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { PluginRecord } from "@/lib/types/plugins";
import type { InstallResult } from "@/lib/api/domains/plugins-api";

const loadPlugins = vi.fn(async () => {});
const unloadPlugin = vi.fn();
const installPluginFromUrl = vi.fn<() => Promise<InstallResult>>();
const installPluginUpload = vi.fn<() => Promise<InstallResult>>();
const enablePlugin = vi.fn(async () => ({ enabled: true }));
const getPlugin = vi.fn<() => Promise<PluginRecord>>();
const syncPlugins = vi.fn();
const listPlugins = vi.fn(async () => []);
const uninstallPlugin = vi.fn<() => Promise<{ deleted: boolean }>>();
const { toastError, toastSuccess, toastWarning } = vi.hoisted(() => ({
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
  toastWarning: vi.fn(),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: {
    error: (...args: unknown[]) => toastError(...args),
    success: (...args: unknown[]) => toastSuccess(...args),
    warning: (...args: unknown[]) => toastWarning(...args),
  },
}));

vi.mock("@/lib/plugins/host", () => ({
  loadPlugins: (...args: unknown[]) => loadPlugins(...(args as [])),
  unloadPlugin: (...args: unknown[]) => unloadPlugin(...(args as [])),
}));

vi.mock("@/lib/api/domains/plugins-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/plugins-api")>(
    "@/lib/api/domains/plugins-api",
  );
  return {
    ...actual,
    installPluginFromUrl: (...args: unknown[]) => installPluginFromUrl(...(args as [])),
    installPluginUpload: (...args: unknown[]) => installPluginUpload(...(args as [])),
    enablePlugin: (...args: unknown[]) => enablePlugin(...(args as [])),
    getPlugin: (...args: unknown[]) => getPlugin(...(args as [])),
    syncPlugins: (...args: unknown[]) => syncPlugins(...(args as [])),
    listPlugins: (...args: unknown[]) => listPlugins(...(args as [])),
    uninstallPlugin: (...args: unknown[]) => uninstallPlugin(...(args as [])),
  };
});

import { usePluginActions } from "./use-plugin-actions";

function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

function activeRecord(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id: "acme-tools",
    api_version: 1,
    version: "2.0.0",
    display_name: "Acme Tools",
    description: "",
    author: "",
    categories: [],
    capabilities: {},
    status: "active",
    install_path: "/home/user/.kandev/plugins/acme-tools/2.0.0",
    signed: true,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    ui: { bundle: "/ui/bundle.js" },
    ...overrides,
  };
}

beforeEach(() => {
  loadPlugins.mockClear();
  unloadPlugin.mockClear();
  installPluginFromUrl.mockReset();
  installPluginUpload.mockReset();
  enablePlugin.mockClear();
  getPlugin.mockReset();
  syncPlugins.mockReset();
  listPlugins.mockClear();
  listPlugins.mockResolvedValue([]);
  uninstallPlugin.mockReset();
  toastError.mockReset();
  toastSuccess.mockReset();
  toastWarning.mockReset();
});

describe("usePluginActions — install/update", () => {
  it("unloads the plugin's previous registrations before reloading it, so an update doesn't leave duplicate slot registrations", async () => {
    const plugin = activeRecord();
    installPluginFromUrl.mockResolvedValue({ plugin });

    const { result } = renderHook(() => usePluginActions(), { wrapper });

    await act(async () => {
      await result.current.submitInstallUrl("https://example.com/acme-tools.tar.gz");
    });

    await waitFor(() => expect(loadPlugins).toHaveBeenCalledTimes(1));
    expect(unloadPlugin).toHaveBeenCalledWith(plugin.id, {
      evictCache: true,
      transition: "reload",
    });

    const unloadOrder = unloadPlugin.mock.invocationCallOrder[0];
    const loadOrder = loadPlugins.mock.invocationCallOrder[0];
    expect(unloadOrder).toBeLessThan(loadOrder);
  });
});

describe("usePluginActions — enable", () => {
  it("does not evict the cached bundle registration on plain enable, so a disable-then-re-enable cycle reuses it", async () => {
    const plugin = activeRecord();

    const { result } = renderHook(() => usePluginActions(), { wrapper });

    await act(async () => {
      await result.current.handleEnable(plugin);
    });

    await waitFor(() => expect(loadPlugins).toHaveBeenCalledTimes(1));
    // Enable must unload without evicting the cache — eviction is reserved
    // for install/update, where the bundle content actually changed.
    expect(unloadPlugin).toHaveBeenCalledWith(plugin.id, { transition: "reload" });
    expect(unloadPlugin).not.toHaveBeenCalledWith(plugin.id, {
      evictCache: true,
      transition: "reload",
    });

    const unloadOrder = unloadPlugin.mock.invocationCallOrder[0];
    const loadOrder = loadPlugins.mock.invocationCallOrder[0];
    expect(unloadOrder).toBeLessThan(loadOrder);
  });

  it("clears a stale failure diagnostic after an errored plugin is enabled", async () => {
    const plugin = activeRecord({
      status: "error",
      last_error: "plugins/runtime: handshake failed",
      last_error_at: "2026-08-02T12:34:56Z",
    });

    const { result } = renderHook(
      () => ({ actions: usePluginActions(), stored: useAppStore((s) => s.plugins.items[0]) }),
      { wrapper },
    );

    await act(async () => {
      await result.current.actions.handleEnable(plugin);
    });

    await waitFor(() => expect(result.current.stored?.status).toBe("active"));
    expect(result.current.stored?.last_error).toBeUndefined();
    expect(result.current.stored?.last_error_at).toBeUndefined();
  });

  it("refreshes and upserts the replacement diagnostic after a failed enable", async () => {
    const plugin = activeRecord({
      status: "error",
      last_error: "old handshake failure",
      last_error_at: "2026-08-02T12:34:56Z",
    });
    const refreshed = activeRecord({
      status: "error",
      last_error: "new executable failure",
      last_error_at: "2026-08-02T12:35:56Z",
    });
    enablePlugin.mockRejectedValueOnce(new Error("enable failed"));
    getPlugin.mockResolvedValueOnce(refreshed);

    const { result } = renderHook(
      () => ({ actions: usePluginActions(), stored: useAppStore((s) => s.plugins.items[0]) }),
      { wrapper },
    );

    await act(async () => {
      await result.current.actions.handleEnable(plugin);
    });

    await waitFor(() => expect(result.current.stored?.last_error).toBe("new executable failure"));
    expect(result.current.stored?.last_error).not.toBe(plugin.last_error);
    expect(getPlugin).toHaveBeenCalledWith(plugin.id, { cache: "no-store" });
  });
});

describe("usePluginActions — marketplaceInstall result", () => {
  it("resolves ok: true on a successful install", async () => {
    const plugin = activeRecord();
    installPluginFromUrl.mockResolvedValue({ plugin });

    const { result } = renderHook(() => usePluginActions(), { wrapper });

    let outcome: { ok: boolean; error?: string; pluginId?: string } | undefined;
    await act(async () => {
      outcome = await result.current.marketplaceInstall("https://example.com/acme-tools.tar.gz");
    });

    expect(outcome).toEqual({ ok: true, pluginId: plugin.id });
  });

  it("resolves ok: false with the backend's error message on a failed install, without throwing", async () => {
    installPluginFromUrl.mockRejectedValue(new Error("bad checksum"));

    const { result } = renderHook(() => usePluginActions(), { wrapper });

    let outcome: { ok: boolean; error?: string } | undefined;
    await act(async () => {
      outcome = await result.current.marketplaceInstall("https://example.com/acme-tools.tar.gz");
    });

    expect(outcome).toEqual({ ok: false, error: "bad checksum" });
  });
});

describe("usePluginActions — handleSync result", () => {
  it("resolves ok: true after a successful sync", async () => {
    syncPlugins.mockResolvedValue({ added: [], installed: [], missing: [], errors: [] });

    const { result } = renderHook(() => usePluginActions(), { wrapper });

    let outcome: { ok: boolean } | undefined;
    await act(async () => {
      outcome = await result.current.handleSync();
    });

    expect(outcome).toEqual({ ok: true });
  });

  it("resolves ok: false when the sync call fails, without throwing", async () => {
    syncPlugins.mockRejectedValue(new Error("backend unreachable"));

    const { result } = renderHook(() => usePluginActions(), { wrapper });

    let outcome: { ok: boolean } | undefined;
    await act(async () => {
      outcome = await result.current.handleSync();
    });

    expect(outcome).toEqual({ ok: false });
  });
});

describe("usePluginActions — uninstall", () => {
  it("confirms an explicitly supplied target and keeps busy state until API cleanup settles", async () => {
    const plugin = activeRecord();
    let resolveUninstall!: (result: { deleted: boolean }) => void;
    uninstallPlugin.mockReturnValueOnce(
      new Promise<{ deleted: boolean }>((resolve) => {
        resolveUninstall = resolve;
      }),
    );

    const { result } = renderHook(() => usePluginActions(), { wrapper });
    let confirmation!: Promise<boolean>;
    act(() => {
      confirmation = result.current.confirmUninstall(plugin);
    });

    await waitFor(() => expect(result.current.uninstallBusy).toBe(true));
    expect(uninstallPlugin).toHaveBeenCalledWith(plugin.id);
    expect(unloadPlugin).not.toHaveBeenCalled();

    resolveUninstall({ deleted: true });
    await act(async () => {
      await expect(confirmation).resolves.toBe(true);
    });

    expect(unloadPlugin).toHaveBeenCalledWith(plugin.id);
    await waitFor(() => expect(result.current.uninstallBusy).toBe(false));
  });

  it("keeps localized failure feedback and releases busy state when uninstall fails", async () => {
    const plugin = activeRecord();
    uninstallPlugin.mockRejectedValueOnce(new Error("API key revoke failed"));

    const { result } = renderHook(() => usePluginActions(), { wrapper });
    let confirmation!: Promise<boolean>;
    act(() => {
      confirmation = result.current.confirmUninstall(plugin);
    });

    await act(async () => {
      await expect(confirmation).resolves.toBe(false);
    });

    expect(toastError).toHaveBeenCalledWith("API key revoke failed");
    expect(unloadPlugin).not.toHaveBeenCalled();
    expect(result.current.uninstallBusy).toBe(false);
  });
});
