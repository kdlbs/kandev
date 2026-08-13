import { describe, it, expect, vi, afterEach } from "vitest";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { loadPlugins } from "./host";
import { pluginRegistry } from "./registry";
import type { ActivePlugin, PluginHostApi, PluginRegistry } from "./types";

/** No-op `host.toast`; these specs exercise lifecycle, never notifications. */
const NOOP_TOAST = new Proxy(() => 0, {
  get: () => () => 0,
}) as unknown as PluginHostApi["toast"];

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
    context: {
      getActiveWorkspaceId: () => undefined,
      subscribeActiveWorkspace: () => () => {},
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
    utils: { cn: () => "", generateUUID: () => "uuid", formatRelativeTime: () => "" },
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
