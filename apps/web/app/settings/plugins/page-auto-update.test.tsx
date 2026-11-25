import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import type { PluginRecord } from "@/lib/types/plugins";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));
vi.mock("@/lib/api/domains/plugins-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/plugins-api")>()),
  listPlugins: () => Promise.resolve([]),
  setPluginAutoUpdate: vi.fn(),
}));
vi.mock("@/lib/api/domains/marketplace-api", () => ({
  getMarketplaceCatalog: () => Promise.resolve({ plugins: [], sources: [] }),
}));
vi.mock("@/lib/plugins/host", () => ({ loadPlugins: () => Promise.resolve() }));
vi.mock("@/components/theme/app-theme", () => ({ useTheme: () => ({ resolvedTheme: "light" }) }));
let storeState: Record<string, unknown> = {};
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(storeState),
  useAppStoreApi: () => ({
    getState: () => storeState,
    setState: vi.fn(),
    subscribe: vi.fn(() => () => {}),
  }),
}));

import PluginsSettingsPage from "./page";
import { setPluginAutoUpdate } from "@/lib/api/domains/plugins-api";

const PLUGIN_ID = "inherited";
const NEW_PLUGIN_ID = "override-on";
const ARIA_CHECKED = "aria-checked";
function activePlugin(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id: PLUGIN_ID,
    api_version: 1,
    version: "1.0.0",
    display_name: "Fixture plugin",
    description: "",
    author: "test",
    categories: [],
    capabilities: {},
    status: "active",
    install_path: "/fixtures/plugin",
    signed: true,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    ...overrides,
  };
}
function installedPlugin(overrides: Partial<PluginRecord>) {
  return activePlugin({ id: NEW_PLUGIN_ID, ...overrides });
}
function setStoreState(items: PluginRecord[]) {
  storeState = {
    auth: { user: null },
    features: { canvases: false },
    plugins: { items, loading: false, loaded: true, error: null },
    setPlugins: vi.fn(),
    setPluginsLoading: vi.fn(),
    setPluginsError: vi.fn(),
    upsertPlugin: vi.fn(),
    removePlugin: vi.fn(),
  };
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

// @covers AC-PLUGINS-MARKETPLACE-004.1, AC-PLUGINS-MARKETPLACE-004.2, AC-PLUGINS-MARKETPLACE-004.3
it.each([true, false])(
  "keeps real global/inherited controls truthful after rapid writes (success %s)",
  async (success) => {
    const held: Array<{ value: boolean; resolve: (response: Response) => void }> = [];
    const response = (value: boolean) =>
      new Response(JSON.stringify({ auto_update_default: value }));
    vi.stubGlobal(
      "fetch",
      vi.fn((_url: string, init: RequestInit) => {
        if (init.method !== "PUT") return Promise.resolve(response(false));
        const value = JSON.parse(String(init.body)).auto_update_default as boolean;
        return new Promise<Response>((resolve) => held.push({ value, resolve }));
      }),
    );
    setStoreState([
      activePlugin({ auto_update: null }),
      installedPlugin({ auto_update: true }),
      activePlugin({ id: "override-off", auto_update: false }),
    ]);
    render(
      <SettingsSaveProvider>
        <PluginsSettingsPage />
      </SettingsSaveProvider>,
    );
    const global = screen.getByTestId("plugins-auto-update-default");
    const inherited = screen.getByTestId(`plugin-auto-update-${PLUGIN_ID}`);
    const overridden = screen.getByTestId(`plugin-auto-update-${NEW_PLUGIN_ID}`);
    const overriddenOff = screen.getByTestId("plugin-auto-update-override-off");
    await vi.waitFor(() => expect(global.hasAttribute("disabled")).toBe(false));
    fireEvent.click(global);
    expect(inherited.getAttribute(ARIA_CHECKED)).toBe("true");
    expect(overriddenOff.getAttribute(ARIA_CHECKED)).toBe("false");
    fireEvent.click(global);
    expect(inherited.getAttribute(ARIA_CHECKED)).toBe("false");
    try {
      await vi.waitFor(() => expect(held[0]).toBeDefined());
      await act(async () =>
        held[0].resolve(success ? response(true) : new Response("", { status: 503 })),
      );
      await vi.waitFor(() => expect(held[1]).toBeDefined());
      await act(async () =>
        held[1].resolve(success ? response(false) : new Response("", { status: 503 })),
      );
      expect(global.getAttribute(ARIA_CHECKED)).toBe("false");
      expect(inherited.getAttribute(ARIA_CHECKED)).toBe("false");
      expect(overridden.getAttribute(ARIA_CHECKED)).toBe("true");
      expect(setPluginAutoUpdate).not.toHaveBeenCalled();
    } finally {
      cleanup();
      await act(async () => {
        held.forEach((request) => request.resolve(response(request.value)));
      });
    }
  },
);
