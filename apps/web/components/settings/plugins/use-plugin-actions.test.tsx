import { renderHook, act, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import type { PluginRecord } from "@/lib/types/plugins";
import type { InstallResult } from "@/lib/api/domains/plugins-api";

const loadPlugins = vi.fn(async () => {});
const unloadPlugin = vi.fn();
const installPluginFromUrl = vi.fn<() => Promise<InstallResult>>();
const installPluginUpload = vi.fn<() => Promise<InstallResult>>();

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
    expect(unloadPlugin).toHaveBeenCalledWith(plugin.id, { evictCache: true });

    const unloadOrder = unloadPlugin.mock.invocationCallOrder[0];
    const loadOrder = loadPlugins.mock.invocationCallOrder[0];
    expect(unloadOrder).toBeLessThan(loadOrder);
  });
});
