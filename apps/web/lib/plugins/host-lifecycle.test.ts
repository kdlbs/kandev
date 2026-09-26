import { describe, it, expect, vi, afterEach } from "vitest";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { loadPlugins, unloadPlugin } from "./host";
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
    conversation: {} as PluginHostApi["conversation"],
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

function registerFake(id: string, plugin: unknown) {
  (window as unknown as FakeWindow).registerKandevPlugin(id, plugin);
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

describe("authoritative plugin lifecycle staging", () => {
  it("discards a sidebar-footer registration when initialize() throws", async () => {
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
    expect(pluginRegistry.getNavItems()).not.toContainEqual(
      expect.objectContaining({ id: "nav-throw-a" }),
    );
    errorSpy.mockRestore();
  });

  it("discards a sidebar-footer registration when initialize() times out", async () => {
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
    expect(pluginRegistry.getNavItems()).not.toContainEqual(
      expect.objectContaining({ id: "nav-partial-timeout" }),
    );

    initializeGate.resolve();
    warnSpy.mockRestore();
  });
});
describe("plugin reload staging", () => {
  const PLUGIN_RELOAD_ID = "plugin-reload-staging";

  afterEach(() => {
    unloadPlugin(PLUGIN_RELOAD_ID, { evictCache: true });
  });

  it("keeps the published runtime and contributions when a replacement fails", async () => {
    const oldDestroy = vi.fn();
    const importer = async (url: string) => {
      if (url === "/old-bundle.js") {
        registerFake(PLUGIN_RELOAD_ID, {
          initialize: (registry: PluginRegistry) =>
            registry.registerNavItem({ id: "old-nav", label: "Old", path: "/old" }),
          destroy: oldDestroy,
        });
        return {};
      }
      registerFake(PLUGIN_RELOAD_ID, {
        initialize: () => {
          throw new Error("replacement failed");
        },
      });
      return {};
    };

    await loadPlugins(
      [activePlugin({ id: PLUGIN_RELOAD_ID, bundleUrl: "/old-bundle.js" })],
      makeHostFactory,
      importer,
    );
    await loadPlugins(
      [activePlugin({ id: PLUGIN_RELOAD_ID, bundleUrl: "/new-bundle.js" })],
      makeHostFactory,
      importer,
    );

    expect(pluginRegistry.getNavItems()).toContainEqual(expect.objectContaining({ id: "old-nav" }));
    expect(oldDestroy).not.toHaveBeenCalled();
    expect(pluginRegistry.getPluginLifecycle(PLUGIN_RELOAD_ID)?.status).toBe("ready");
  });
});

describe("plugin registration staging", () => {
  const PLUGIN_STAGE_ID = "plugin-registration-staging";

  afterEach(() => {
    unloadPlugin(PLUGIN_STAGE_ID, { evictCache: true });
    pluginRegistry.unregisterPlugin("foreign-registration");
  });

  it("ignores foreign and late registrations without changing the matching bundle cache", async () => {
    const importer = vi.fn(async (_url: string) => {
      const register = (window as unknown as FakeWindow).registerKandevPlugin;
      register("foreign-registration", {
        initialize: (registry: PluginRegistry) =>
          registry.registerNavItem({ id: "foreign-nav", label: "Foreign", path: "/foreign" }),
      });
      register(PLUGIN_STAGE_ID, {
        initialize: (registry: PluginRegistry) =>
          registry.registerNavItem({ id: "stage-nav", label: "Stage", path: "/stage" }),
      });
      (window as unknown as { lateRegister?: typeof register }).lateRegister = register;
      return {};
    });

    await loadPlugins(
      [activePlugin({ id: PLUGIN_STAGE_ID, bundleUrl: "/stage-bundle.js" })],
      makeHostFactory,
      importer,
    );
    (window as unknown as { lateRegister?: typeof registerFake }).lateRegister?.(PLUGIN_STAGE_ID, {
      initialize: (registry: PluginRegistry) =>
        registry.registerNavItem({ id: "late-nav", label: "Late", path: "/late" }),
    });
    unloadPlugin(PLUGIN_STAGE_ID);
    await loadPlugins(
      [activePlugin({ id: PLUGIN_STAGE_ID, bundleUrl: "/stage-bundle.js" })],
      makeHostFactory,
      importer,
    );

    expect(importer).toHaveBeenCalledTimes(1);
    expect(pluginRegistry.getNavItems()).toContainEqual(
      expect.objectContaining({ id: "stage-nav" }),
    );
    expect(pluginRegistry.getNavItems()).not.toContainEqual(
      expect.objectContaining({ id: "foreign-nav" }),
    );
    expect(pluginRegistry.getNavItems()).not.toContainEqual(
      expect.objectContaining({ id: "late-nav" }),
    );
  });

  it("imports a changed bundle URL instead of reusing the prior registration", async () => {
    const importer = vi.fn(async (url: string) => {
      registerFake(PLUGIN_STAGE_ID, {
        initialize: (registry: PluginRegistry) =>
          registry.registerNavItem({
            id: url === "/stage-v1.js" ? "stage-v1-nav" : "stage-v2-nav",
            label: "Stage",
            path: "/stage",
          }),
      });
      return {};
    });

    await loadPlugins(
      [activePlugin({ id: PLUGIN_STAGE_ID, bundleUrl: "/stage-v1.js" })],
      makeHostFactory,
      importer,
    );
    unloadPlugin(PLUGIN_STAGE_ID);
    await loadPlugins(
      [activePlugin({ id: PLUGIN_STAGE_ID, bundleUrl: "/stage-v2.js" })],
      makeHostFactory,
      importer,
    );

    expect(importer).toHaveBeenCalledTimes(2);
    expect(pluginRegistry.getNavItems()).toContainEqual(
      expect.objectContaining({ id: "stage-v2-nav" }),
    );
    expect(pluginRegistry.getNavItems()).not.toContainEqual(
      expect.objectContaining({ id: "stage-v1-nav" }),
    );
  });
});
