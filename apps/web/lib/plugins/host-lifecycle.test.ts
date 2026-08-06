import { describe, it, expect, vi, afterEach } from "vitest";
import { loadPlugins } from "./host";
import { pluginRegistry } from "./registry";
import type { ActivePlugin, PluginHostApi, PluginRegistry } from "./types";

const PLUGIN_LIFECYCLE_A_ID = "plugin-lifecycle-a";
const PLUGIN_TIMEOUT_ID = "plugin-timeout-a";

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
    api: { fetch: async () => new Response(), baseUrl: "" },
    ui: {},
    theme: "light",
    navigate: () => {},
    openModal: () => ({ close: () => {} }),
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
