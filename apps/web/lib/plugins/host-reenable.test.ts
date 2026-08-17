import { describe, it, expect, vi, afterEach } from "vitest";
import { loadPlugins, unloadPlugin } from "./host";
import { pluginRegistry } from "./registry";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { ActivePlugin, PluginHostApi, PluginRegistry } from "./types";

/** No-op `host.toast`; these specs exercise lifecycle, never notifications. */
const NOOP_TOAST = new Proxy(() => 0, {
  get: () => () => 0,
}) as unknown as PluginHostApi["toast"];

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "" }),
}));

const PLUGIN_REENABLE_A_ID = "plugin-reenable-a";
const PLUGIN_REENABLE_A_PATH = "/plugin-reenable-a";
const NAV_REENABLE_A_ID = "nav-reenable-a";
const PLUGIN_REENABLE_B_ID = "plugin-reenable-b";
const PLUGIN_REENABLE_B_PATH = "/plugin-reenable-b";
const NAV_REENABLE_B_ID = "nav-reenable-b";

function makeHostFactory(pluginId: string): PluginHostApi {
  return {
    pluginId,
    React: {} as PluginHostApi["React"],
    jsx: {} as PluginHostApi["jsx"],
    store: {
      getState: () => ({}) as never,
      setState: () => {},
      subscribe: () => () => {},
    },
    context: {
      getActiveWorkspaceId: () => undefined,
      subscribeActiveWorkspace: () => () => {},
      getWorkspaceIds: () => [],
      subscribeWorkspaces: () => () => {},
      getTaskCreationContext: () => null,
      subscribeTaskCreationContext: () => () => {},
      resolveRepositoryId: () => undefined,
    },
    api: {
      fetch: async () => new Response(),
      invokeAction: async <TResponse>() => undefined as TResponse,
      baseUrl: "",
    },
    i18n: {
      locale: "en",
      t: (key) => key,
      useTranslation: () => ({ locale: "en", t: (key) => key }),
    },
    ui: {} as PluginHostApi["ui"],
    useResponsiveBreakpoint,
    theme: "light",
    onThemeChange: () => () => {},
    navigate: () => {},
    openModal: () => ({ close: () => {} }),
    openTaskLinkDialog: () => ({ close: () => {} }),
    openTaskReview: () => {},
    toast: NOOP_TOAST,
    utils: {
      cn: () => "",
      generateUUID: () => "uuid",
      formatRelativeTime: () => "",
      integrationStatusRefreshMs: 90000,
    },
    useSettingsSaveContributor: () => {},
    setIntegrationEnabled: () => {},
    storage: {
      get: async () => undefined,
      set: async () => ({ updatedAt: "" }),
      delete: async () => {},
      list: async () => [],
      subscribe: () => () => {},
    },
  };
}

type FakeWindow = Window & {
  registerKandevPlugin: (id: string, plugin: unknown) => void;
};

function registerFake(id: string, plugin: unknown) {
  (window as unknown as FakeWindow).registerKandevPlugin(id, plugin);
}

function activePlugin(overrides: Partial<ActivePlugin> = {}): ActivePlugin {
  return {
    id: "plugin-a",
    name: "Plugin A",
    bundleUrl: "/api/plugins/plugin-a/bundle",
    ...overrides,
  };
}

describe("disable then re-enable in the same session", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_REENABLE_A_ID);
    pluginRegistry.unregisterPlugin(PLUGIN_REENABLE_B_ID);
  });

  it("re-initializes from the cached registration when the bundle's module-eval side effect only fires once (ESM import caching)", async () => {
    const initialize = vi.fn((registry: PluginRegistry) => {
      registry.registerNavItem({ id: NAV_REENABLE_A_ID, label: "A", path: PLUGIN_REENABLE_A_PATH });
    });
    let importCount = 0;
    const importer = vi.fn(async (_url: string) => {
      importCount += 1;
      // The real browser only runs a module's top-level side effect on the
      // *first* resolution of a given specifier — a second `import(url)`
      // resolves from the module cache without re-executing
      // `window.registerKandevPlugin(...)`.
      if (importCount === 1) {
        registerFake(PLUGIN_REENABLE_A_ID, { initialize });
      }
      return {};
    });

    await loadPlugins([activePlugin({ id: PLUGIN_REENABLE_A_ID })], makeHostFactory, importer);
    expect(initialize).toHaveBeenCalledTimes(1);
    expect(pluginRegistry.getNavItems()).toContainEqual({
      id: NAV_REENABLE_A_ID,
      label: "A",
      path: PLUGIN_REENABLE_A_PATH,
    });

    unloadPlugin(PLUGIN_REENABLE_A_ID);
    expect(
      pluginRegistry.getNavItems().find((item) => item.id === NAV_REENABLE_A_ID),
    ).toBeUndefined();

    await loadPlugins([activePlugin({ id: PLUGIN_REENABLE_A_ID })], makeHostFactory, importer);

    expect(initialize).toHaveBeenCalledTimes(2);
    expect(pluginRegistry.getNavItems()).toContainEqual({
      id: NAV_REENABLE_A_ID,
      label: "A",
      path: PLUGIN_REENABLE_A_PATH,
    });
  });

  it("reorders the registry to the end on re-enable, relative to a plugin that stayed enabled throughout", async () => {
    const initializeA = vi.fn((registry: PluginRegistry) => {
      registry.registerNavItem({ id: NAV_REENABLE_A_ID, label: "A", path: PLUGIN_REENABLE_A_PATH });
    });
    const initializeB = vi.fn((registry: PluginRegistry) => {
      registry.registerNavItem({ id: NAV_REENABLE_B_ID, label: "B", path: PLUGIN_REENABLE_B_PATH });
    });
    const importer = vi.fn(async (url: string) => {
      if (url === "/reenable-a-bundle.js") {
        registerFake(PLUGIN_REENABLE_A_ID, { initialize: initializeA });
      } else if (url === "/reenable-b-bundle.js") {
        registerFake(PLUGIN_REENABLE_B_ID, { initialize: initializeB });
      }
      return {};
    });

    await loadPlugins(
      [
        activePlugin({ id: PLUGIN_REENABLE_A_ID, bundleUrl: "/reenable-a-bundle.js" }),
        activePlugin({ id: PLUGIN_REENABLE_B_ID, bundleUrl: "/reenable-b-bundle.js" }),
      ],
      makeHostFactory,
      importer,
    );

    expect(
      pluginRegistry.getNavRegistrations().map((registration) => registration.pluginId),
    ).toEqual([PLUGIN_REENABLE_A_ID, PLUGIN_REENABLE_B_ID]);

    unloadPlugin(PLUGIN_REENABLE_A_ID);
    await loadPlugins(
      [activePlugin({ id: PLUGIN_REENABLE_A_ID, bundleUrl: "/reenable-a-bundle.js" })],
      makeHostFactory,
      importer,
    );

    // The registry itself reorders on re-enable — A's re-registration is
    // appended after B, not merely re-added somewhere. Proves the mechanism
    // (spec.md:797/:815's re-enable ordering guarantee), not just a manually
    // simulated post-re-enable array as app-sidebar-footer.test.tsx's
    // "partitions by current registration order" test discloses it does.
    expect(
      pluginRegistry.getNavRegistrations().map((registration) => registration.pluginId),
    ).toEqual([PLUGIN_REENABLE_B_ID, PLUGIN_REENABLE_A_ID]);
    // Count assertion: A is the sole entry after B — a looser "A comes after
    // B somewhere" check would still pass if a stray duplicate leaked in.
    expect(pluginRegistry.getNavRegistrations()).toHaveLength(2);
  });
});
