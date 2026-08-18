import { describe, it, expect, vi, afterEach } from "vitest";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { pluginDestinations } from "@/lib/navigation/plugin-destinations";
import { loadPlugins } from "./host";
import { pluginRegistry } from "./registry";
import type { ActivePlugin, PluginHostApi, PluginRegistry } from "./types";

/** No-op `host.toast`; these specs exercise lifecycle, never notifications. */
const NOOP_TOAST = new Proxy(() => 0, {
  get: () => () => 0,
}) as unknown as PluginHostApi["toast"];

const PLUGIN_LIFECYCLE_A_ID = "plugin-lifecycle-a";
const PLUGIN_TIMEOUT_ID = "plugin-timeout-a";
const PLUGIN_THROW_ID = "plugin-throw-a";
const PLUGIN_PARTIAL_TIMEOUT_ID = "plugin-partial-timeout-a";
const SIDEBAR_FOOTER_SECTION = "sidebar-footer" as const;

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "" }),
}));

type FakeWindow = Window & {
  registerKandevPlugin: (id: string, plugin: unknown) => void;
};

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
    useSettingsSaveContributor: () => {},
    setIntegrationEnabled: () => {},
    utils: {
      cn: () => "",
      generateUUID: () => "uuid",
      formatRelativeTime: () => "",
      integrationStatusRefreshMs: 90000,
    },
    storage: {
      get: async () => undefined,
      set: async () => ({ updatedAt: "" }),
      delete: async () => {},
      list: async () => [],
      subscribe: () => () => {},
    },
  };
}

function activePlugin(overrides: Partial<ActivePlugin> = {}): ActivePlugin {
  return {
    id: "plugin-a",
    name: "Plugin A",
    bundleUrl: "/api/plugins/plugin-a/bundle",
    ...overrides,
  };
}

function fakeImporterFor(
  bundles: Record<string, (win: Window) => void>,
): (url: string) => Promise<unknown> {
  return async (url: string) => {
    const run = bundles[url];
    if (!run) throw new Error(`no fake bundle for ${url}`);
    run(window);
    return {};
  };
}

function deferred<T = void>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

afterEach(() => {
  pluginRegistry.unregisterPlugin(PLUGIN_LIFECYCLE_A_ID);
  pluginRegistry.unregisterPlugin(PLUGIN_TIMEOUT_ID);
  pluginRegistry.unregisterPlugin(PLUGIN_THROW_ID);
  pluginRegistry.unregisterPlugin(PLUGIN_PARTIAL_TIMEOUT_ID);
});

describe("authoritative plugin lifecycle", () => {
  it("keeps the current generation loading until initialize completes, then publishes ready", async () => {
    const initializeStarted = deferred<void>();
    const initializeGate = deferred<void>();
    const initialize = vi.fn(async (registry: PluginRegistry) => {
      initializeStarted.resolve();
      await initializeGate.promise;
      registry.registerNavItem({ id: "nav-lifecycle-a", label: "A", path: "/lifecycle-a" });
    });
    const importer = fakeImporterFor({
      "/lifecycle-bundle.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_LIFECYCLE_A_ID, {
          initialize,
        }),
    });

    const load = loadPlugins(
      [activePlugin({ id: PLUGIN_LIFECYCLE_A_ID, bundleUrl: "/lifecycle-bundle.js" })],
      makeHostFactory,
      importer,
    );
    await initializeStarted.promise;

    expect(pluginRegistry.getPluginLifecycle(PLUGIN_LIFECYCLE_A_ID)?.status).toBe("loading");

    initializeGate.resolve();
    await load;

    expect(pluginRegistry.getPluginLifecycle(PLUGIN_LIFECYCLE_A_ID)?.status).toBe("ready");
  });

  it("publishes failed for a timed-out initializer and fences its later registrations", async () => {
    const initializeStarted = deferred<void>();
    const initializeGate = deferred<void>();
    const initialize = vi.fn(async (registry: PluginRegistry) => {
      initializeStarted.resolve();
      await initializeGate.promise;
      registry.registerNavItem({ id: "nav-timed-out", label: "Timed out", path: "/timed-out" });
    });
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    const importer = fakeImporterFor({
      "/timed-out-bundle.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_TIMEOUT_ID, {
          initialize,
        }),
    });

    const load = loadPlugins(
      [activePlugin({ id: PLUGIN_TIMEOUT_ID, bundleUrl: "/timed-out-bundle.js" })],
      makeHostFactory,
      importer,
      window,
      1,
    );
    await initializeStarted.promise;
    await load;

    expect(pluginRegistry.getPluginLifecycle(PLUGIN_TIMEOUT_ID)?.status).toBe("failed");

    initializeGate.resolve();
    await Promise.resolve();
    expect(pluginRegistry.getNavItems()).not.toContainEqual({
      id: "nav-timed-out",
      label: "Timed out",
      path: "/timed-out",
    });
    expect(warnSpy).toHaveBeenCalledWith(expect.stringContaining("timed out"));
    warnSpy.mockRestore();
  });
});

describe("authoritative plugin lifecycle — partial-initialization-failure retention", () => {
  it("keeps a sidebar-footer registration made before initialize() throws, and marks the plugin failed", async () => {
    const initialize = vi.fn(async (registry: PluginRegistry) => {
      registry.registerNavItem({
        id: "nav-throw-a",
        label: "Throw",
        path: "/throw-a",
        section: SIDEBAR_FOOTER_SECTION,
      });
      throw new Error("boom");
    });
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const importer = fakeImporterFor({
      "/throw-bundle.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_THROW_ID, { initialize }),
    });

    await loadPlugins(
      [activePlugin({ id: PLUGIN_THROW_ID, bundleUrl: "/throw-bundle.js" })],
      makeHostFactory,
      importer,
    );

    expect(pluginRegistry.getPluginLifecycle(PLUGIN_THROW_ID)?.status).toBe("failed");
    expect(pluginRegistry.getNavItems()).toContainEqual({
      id: "nav-throw-a",
      label: "Throw",
      path: "/throw-a",
      section: SIDEBAR_FOOTER_SECTION,
    });
    // The footer's manifest is built from getNavRegistrations() through
    // pluginDestinations() (see plugin-destinations.ts), which has no concept
    // of plugin lifecycle status — a "failed" plugin's already-registered item
    // maps to the "insights" section exactly like a "ready" plugin's would,
    // which is what makes it occupy a footer budget slot (spec.md:816's AC).
    const destinations = pluginDestinations(pluginRegistry.getNavRegistrations());
    expect(destinations).toContainEqual(
      expect.objectContaining({
        pluginItemId: "nav-throw-a",
        section: "insights",
        source: "plugin",
      }),
    );
    errorSpy.mockRestore();
  });

  it("keeps a sidebar-footer registration made before initialize() times out, and marks the plugin failed", async () => {
    const initializeStarted = deferred<void>();
    const initializeGate = deferred<void>();
    const initialize = vi.fn(async (registry: PluginRegistry) => {
      registry.registerNavItem({
        id: "nav-partial-timeout",
        label: "Partial",
        path: "/partial-timeout",
        section: SIDEBAR_FOOTER_SECTION,
      });
      initializeStarted.resolve();
      await initializeGate.promise;
    });
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    const importer = fakeImporterFor({
      "/partial-timeout-bundle.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_PARTIAL_TIMEOUT_ID, {
          initialize,
        }),
    });

    const load = loadPlugins(
      [activePlugin({ id: PLUGIN_PARTIAL_TIMEOUT_ID, bundleUrl: "/partial-timeout-bundle.js" })],
      makeHostFactory,
      importer,
      window,
      1,
    );
    await initializeStarted.promise;
    await load;

    expect(pluginRegistry.getPluginLifecycle(PLUGIN_PARTIAL_TIMEOUT_ID)?.status).toBe("failed");
    expect(pluginRegistry.getNavItems()).toContainEqual({
      id: "nav-partial-timeout",
      label: "Partial",
      path: "/partial-timeout",
      section: SIDEBAR_FOOTER_SECTION,
    });
    // See the throw case above: pluginDestinations() has no lifecycle concept,
    // so a timed-out plugin's already-registered item still maps to "insights"
    // and occupies a footer budget slot exactly like a ready plugin's would.
    const destinations = pluginDestinations(pluginRegistry.getNavRegistrations());
    expect(destinations).toContainEqual(
      expect.objectContaining({
        pluginItemId: "nav-partial-timeout",
        section: "insights",
        source: "plugin",
      }),
    );

    initializeGate.resolve();
    warnSpy.mockRestore();
  });
});
