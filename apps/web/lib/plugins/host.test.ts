import { describe, it, expect, vi, afterEach } from "vitest";
import type { Window as HappyDOMWindow } from "happy-dom";
import { loadPlugins, unloadPlugin } from "./host";
import { pluginRegistry } from "./registry";
import type { ActivePlugin, PluginHostApi, PluginRegistry } from "./types";

let mockApiBaseUrl = "";
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: mockApiBaseUrl }),
}));

const PLUGIN_SCOPE_A_ID = "plugin-scope-a";
const BUNDLE_JS_URL = "/bundle.js";
const PLUGIN_UNLOAD_A_ID = "plugin-unload-a";
const PLUGIN_UNLOAD_THROW_A_ID = "plugin-unload-throw-a";
const PLUGIN_UNLOAD_STYLE_A_ID = "plugin-unload-style-a";
const PLUGIN_REENABLE_A_ID = "plugin-reenable-a";
const PLUGIN_REENABLE_A_PATH = "/plugin-reenable-a";
const NAV_REENABLE_A_ID = "nav-reenable-a";
const PLUGIN_HANG_A_ID = "plugin-hang-a";
const PLUGIN_HANG_B_ID = "plugin-hang-b";

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
  };
}

type FakeWindow = Window & {
  registerKandevPlugin: (id: string, plugin: unknown) => void;
};

/** Fake importer that synchronously invokes window.registerKandevPlugin, no real dynamic import. */
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

afterEach(() => {
  mockApiBaseUrl = "";
});

describe("loadPlugins", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_SCOPE_A_ID);
    pluginRegistry.unregisterPlugin("plugin-style-a");
    pluginRegistry.unregisterPlugin("plugin-throw-a");
    pluginRegistry.unregisterPlugin("plugin-throw-b");
    pluginRegistry.unregisterPlugin("plugin-silent-a");
    document.head.querySelectorAll("link[rel='stylesheet']").forEach((el) => el.remove());
  });

  it("imports the bundle, then calls initialize(registry, host) with a registry scoped to the plugin", async () => {
    const initialize = vi.fn((registry: PluginRegistry, _host: PluginHostApi) => {
      registry.registerNavItem({ id: "nav-scope-a", label: "A", path: "/plugin-scope-a" });
    });
    const importer = fakeImporterFor({
      "/api/plugins/plugin-scope-a/bundle": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_SCOPE_A_ID, { initialize }),
    });

    await loadPlugins(
      [activePlugin({ id: PLUGIN_SCOPE_A_ID, bundleUrl: "/api/plugins/plugin-scope-a/bundle" })],
      makeHostFactory,
      importer,
    );

    expect(initialize).toHaveBeenCalledTimes(1);
    const [, host] = initialize.mock.calls[0];
    expect(host.pluginId).toBe(PLUGIN_SCOPE_A_ID);
    expect(pluginRegistry.getNavItems()).toContainEqual({
      id: "nav-scope-a",
      label: "A",
      path: "/plugin-scope-a",
    });
  });

  it("injects styleUrls as <link> elements before importing the bundle", async () => {
    // happy-dom eagerly loads real <link rel="stylesheet"> hrefs over the network;
    // disable that for this test so it doesn't attempt (and 404-log) a real fetch.
    const happyDOMWindow = window as unknown as HappyDOMWindow;
    happyDOMWindow.happyDOM.settings.disableCSSFileLoading = true;
    const importer = fakeImporterFor({
      [BUNDLE_JS_URL]: (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin("plugin-style-a", {
          initialize: () => {},
        }),
    });

    await loadPlugins(
      [
        activePlugin({
          id: "plugin-style-a",
          bundleUrl: BUNDLE_JS_URL,
          styleUrls: ["/plugin-a.css"],
        }),
      ],
      makeHostFactory,
      importer,
    );

    const link = document.head.querySelector("link[href='/plugin-a.css']");
    expect(link).not.toBeNull();
    happyDOMWindow.happyDOM.settings.disableCSSFileLoading = false;
  });

  it("isolates a throwing plugin: logs and does not stop other plugins from loading", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const goodInitialize = vi.fn();
    const importer = fakeImporterFor({
      "/bad-bundle.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin("plugin-throw-a", {
          initialize: () => {
            throw new Error("boom");
          },
        }),
      "/good-bundle.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin("plugin-throw-b", {
          initialize: goodInitialize,
        }),
    });

    await loadPlugins(
      [
        activePlugin({ id: "plugin-throw-a", bundleUrl: "/bad-bundle.js" }),
        activePlugin({ id: "plugin-throw-b", bundleUrl: "/good-bundle.js" }),
      ],
      makeHostFactory,
      importer,
    );

    expect(errorSpy).toHaveBeenCalled();
    expect(goodInitialize).toHaveBeenCalledTimes(1);
    errorSpy.mockRestore();
  });

  it("logs and continues when a bundle never calls registerKandevPlugin", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const importer = fakeImporterFor({ "/silent-bundle.js": () => {} });

    await loadPlugins(
      [activePlugin({ id: "plugin-silent-a", bundleUrl: "/silent-bundle.js" })],
      makeHostFactory,
      importer,
    );

    expect(errorSpy).toHaveBeenCalled();
    errorSpy.mockRestore();
  });
});

describe("loadPlugins — asset URL prefixing", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin("plugin-prefix-a");
    pluginRegistry.unregisterPlugin("plugin-bare-a");
    document.head.querySelectorAll("link[rel='stylesheet']").forEach((el) => el.remove());
  });

  it("prefixes a root-relative bundleUrl and style href with the backend apiBaseUrl when set (split-origin dev, Tauri)", async () => {
    mockApiBaseUrl = "http://localhost:38429";
    const happyDOMWindow = window as unknown as HappyDOMWindow;
    happyDOMWindow.happyDOM.settings.disableCSSFileLoading = true;
    const importedUrls: string[] = [];
    const importer = async (url: string) => {
      importedUrls.push(url);
      registerFake("plugin-prefix-a", { initialize: () => {} });
      return {};
    };

    await loadPlugins(
      [
        activePlugin({
          id: "plugin-prefix-a",
          bundleUrl: "/api/plugins/plugin-prefix-a/bundle",
          styleUrls: ["/api/plugins/plugin-prefix-a/ui/style.css"],
        }),
      ],
      makeHostFactory,
      importer,
    );

    expect(importedUrls).toEqual(["http://localhost:38429/api/plugins/plugin-prefix-a/bundle"]);
    const link = document.head.querySelector(
      "link[href='http://localhost:38429/api/plugins/plugin-prefix-a/ui/style.css']",
    );
    expect(link).not.toBeNull();
    happyDOMWindow.happyDOM.settings.disableCSSFileLoading = false;
  });

  it("leaves a root-relative bundleUrl unprefixed when apiBaseUrl is empty (same-origin production)", async () => {
    mockApiBaseUrl = "";
    const importedUrls: string[] = [];
    const importer = async (url: string) => {
      importedUrls.push(url);
      registerFake("plugin-bare-a", { initialize: () => {} });
      return {};
    };

    await loadPlugins(
      [
        activePlugin({
          id: "plugin-bare-a",
          bundleUrl: "/api/plugins/plugin-bare-a/bundle",
        }),
      ],
      makeHostFactory,
      importer,
    );

    expect(importedUrls).toEqual(["/api/plugins/plugin-bare-a/bundle"]);
  });
});

describe("unloadPlugin", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_UNLOAD_A_ID);
    pluginRegistry.unregisterPlugin(PLUGIN_UNLOAD_THROW_A_ID);
    pluginRegistry.unregisterPlugin(PLUGIN_UNLOAD_STYLE_A_ID);
    document.head.querySelectorAll("link[rel='stylesheet']").forEach((el) => el.remove());
  });

  it("calls destroy() and bulk-revokes the plugin's registrations", async () => {
    const destroy = vi.fn();
    const importer = fakeImporterFor({
      [BUNDLE_JS_URL]: (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_UNLOAD_A_ID, {
          initialize: (registry: { registerNavItem: (item: unknown) => void }) => {
            registry.registerNavItem({ id: "nav-a", label: "A", path: "/plugin-a" });
          },
          destroy,
        }),
    });
    await loadPlugins(
      [activePlugin({ id: PLUGIN_UNLOAD_A_ID, bundleUrl: BUNDLE_JS_URL })],
      makeHostFactory,
      importer,
    );
    expect(pluginRegistry.getNavItems()).toContainEqual({
      id: "nav-a",
      label: "A",
      path: "/plugin-a",
    });

    unloadPlugin(PLUGIN_UNLOAD_A_ID);

    expect(destroy).toHaveBeenCalledTimes(1);
    expect(pluginRegistry.getNavItems().find((item) => item.id === "nav-a")).toBeUndefined();
  });

  it("swallows a throwing destroy() and still bulk-revokes registrations", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const importer = fakeImporterFor({
      [BUNDLE_JS_URL]: (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_UNLOAD_THROW_A_ID, {
          initialize: (registry: { registerNavItem: (item: unknown) => void }) => {
            registry.registerNavItem({ id: "nav-a", label: "A", path: "/plugin-a" });
          },
          destroy: () => {
            throw new Error("destroy boom");
          },
        }),
    });
    await loadPlugins(
      [activePlugin({ id: PLUGIN_UNLOAD_THROW_A_ID, bundleUrl: BUNDLE_JS_URL })],
      makeHostFactory,
      importer,
    );

    expect(() => unloadPlugin(PLUGIN_UNLOAD_THROW_A_ID)).not.toThrow();
    expect(errorSpy).toHaveBeenCalled();
    expect(pluginRegistry.getNavItems().find((item) => item.id === "nav-a")).toBeUndefined();
    errorSpy.mockRestore();
  });

  it("removes the plugin's injected <link> stylesheet tags so disable/enable cycles don't accumulate duplicates", async () => {
    const happyDOMWindow = window as unknown as HappyDOMWindow;
    happyDOMWindow.happyDOM.settings.disableCSSFileLoading = true;
    const importer = fakeImporterFor({
      [BUNDLE_JS_URL]: (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_UNLOAD_STYLE_A_ID, {
          initialize: () => {},
        }),
    });
    await loadPlugins(
      [
        activePlugin({
          id: PLUGIN_UNLOAD_STYLE_A_ID,
          bundleUrl: BUNDLE_JS_URL,
          styleUrls: ["/plugin-unload-style-a.css"],
        }),
      ],
      makeHostFactory,
      importer,
    );
    expect(document.head.querySelector("link[href='/plugin-unload-style-a.css']")).not.toBeNull();

    unloadPlugin(PLUGIN_UNLOAD_STYLE_A_ID);

    expect(document.head.querySelector("link[href='/plugin-unload-style-a.css']")).toBeNull();
    happyDOMWindow.happyDOM.settings.disableCSSFileLoading = false;
  });
});

describe("loadPlugins — initialize() timeout isolation", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_HANG_A_ID);
    pluginRegistry.unregisterPlugin(PLUGIN_HANG_B_ID);
  });

  it("does not let a plugin whose initialize() never resolves block a subsequent plugin", async () => {
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    const secondInitialize = vi.fn((registry: PluginRegistry) => {
      registry.registerNavItem({ id: "nav-hang-b", label: "B", path: "/plugin-hang-b" });
    });
    const importer = fakeImporterFor({
      "/hang-bundle.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_HANG_A_ID, {
          // Never resolves — simulates a hung plugin initialize().
          initialize: () => new Promise<void>(() => {}),
        }),
      "/second-bundle.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_HANG_B_ID, {
          initialize: secondInitialize,
        }),
    });

    await loadPlugins(
      [
        activePlugin({ id: PLUGIN_HANG_A_ID, bundleUrl: "/hang-bundle.js" }),
        activePlugin({ id: PLUGIN_HANG_B_ID, bundleUrl: "/second-bundle.js" }),
      ],
      makeHostFactory,
      importer,
      window,
      10, // short per-test timeout instead of the 10s default
    );

    expect(secondInitialize).toHaveBeenCalledTimes(1);
    expect(pluginRegistry.getNavItems()).toContainEqual({
      id: "nav-hang-b",
      label: "B",
      path: "/plugin-hang-b",
    });
    expect(warnSpy).toHaveBeenCalledWith(expect.stringContaining(PLUGIN_HANG_A_ID));
    warnSpy.mockRestore();
  });
});

describe("update sequence: unload before reload", () => {
  const PLUGIN_UPDATE_A_ID = "plugin-update-a";
  const MAIN_TOP_BAR_SLOT = "main-top-bar";

  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_UPDATE_A_ID);
  });

  it("registers the top-bar slot component exactly once when a plugin is unloaded then reloaded (update), not twice", async () => {
    const Widget = () => null;
    const importer = fakeImporterFor({
      [BUNDLE_JS_URL]: (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_UPDATE_A_ID, {
          initialize: (registry: PluginRegistry) => {
            registry.registerComponent(MAIN_TOP_BAR_SLOT, Widget);
          },
        }),
    });

    // First install.
    await loadPlugins(
      [activePlugin({ id: PLUGIN_UPDATE_A_ID, bundleUrl: BUNDLE_JS_URL })],
      makeHostFactory,
      importer,
    );
    expect(pluginRegistry.getSlotComponents(MAIN_TOP_BAR_SLOT)).toEqual([Widget]);

    // Update: the reused install path must revoke the previous version's
    // registrations before reloading, exactly like disable/uninstall do.
    unloadPlugin(PLUGIN_UPDATE_A_ID);
    await loadPlugins(
      [activePlugin({ id: PLUGIN_UPDATE_A_ID, bundleUrl: BUNDLE_JS_URL })],
      makeHostFactory,
      importer,
    );

    expect(pluginRegistry.getSlotComponents(MAIN_TOP_BAR_SLOT)).toEqual([Widget]);
  });
});

describe("idempotent reload: no duplicate registrations without an explicit unload", () => {
  const PLUGIN_REBOOT_A_ID = "plugin-reboot-a";
  const CHAT_INPUT_SLOT = "chat-input-actions";

  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_REBOOT_A_ID);
  });

  it("registers the slot component exactly once when loadPlugins runs twice for the same plugin (boot race / HMR re-boot / new store), not twice", async () => {
    const Widget = () => null;
    // The real browser only runs a module's top-level registerKandevPlugin on
    // the first import of a specifier; a second import resolves from cache. The
    // host reuses that cached registration and re-runs initialize(), so without
    // idempotent (re)load this second pass would append a duplicate slot entry.
    let importCount = 0;
    const importer = vi.fn(async (_url: string) => {
      importCount += 1;
      if (importCount === 1) {
        registerFake(PLUGIN_REBOOT_A_ID, {
          initialize: (registry: PluginRegistry) => {
            registry.registerComponent(CHAT_INPUT_SLOT, Widget);
          },
        });
      }
      return {};
    });
    const plugin = activePlugin({ id: PLUGIN_REBOOT_A_ID });

    await loadPlugins([plugin], makeHostFactory, importer);
    // No unloadPlugin() between the two loads — this is the boot re-entry path.
    await loadPlugins([plugin], makeHostFactory, importer);

    expect(pluginRegistry.getSlotComponents(CHAT_INPUT_SLOT)).toEqual([Widget]);
  });
});

describe("overlapping loads for the same plugin: newest-initiated load wins", () => {
  const PLUGIN_CONC_A_ID = "plugin-conc-a";
  const PLUGIN_CONC_B_ID = "plugin-conc-b";
  const SLOT = "chat-input-actions";

  function deferred<T = void>() {
    let resolve!: (value: T) => void;
    const promise = new Promise<T>((r) => {
      resolve = r;
    });
    return { promise, resolve };
  }

  afterEach(() => {
    // evictCache clears both the registry and the module-level registeredPlugins
    // cache so one test's bundle can't leak into the next (they'd otherwise reuse
    // a stale cached registration).
    unloadPlugin(PLUGIN_CONC_A_ID, { evictCache: true });
    unloadPlugin(PLUGIN_CONC_B_ID, { evictCache: true });
  });

  it("a superseded (older) load whose import resolves last does not revoke the newer load's registration (Codex P1: boot v1 vs update v2)", async () => {
    const OldWidget = () => null;
    const NewWidget = () => null;
    const oldImportGate = deferred();

    // The older boot import resolves only after the gate — i.e. last.
    const oldImporter = async (_url: string) => {
      await oldImportGate.promise;
      registerFake(PLUGIN_CONC_A_ID, {
        initialize: (registry: PluginRegistry) => registry.registerComponent(SLOT, OldWidget),
      });
      return {};
    };
    const newImporter = async (_url: string) => {
      registerFake(PLUGIN_CONC_A_ID, {
        initialize: (registry: PluginRegistry) => registry.registerComponent(SLOT, NewWidget),
      });
      return {};
    };

    // Older load starts first (claims the earlier generation) but parks on its
    // import; the newer load then runs to completion.
    const oldLoad = loadPlugins(
      [activePlugin({ id: PLUGIN_CONC_A_ID })],
      makeHostFactory,
      oldImporter,
    );
    await loadPlugins([activePlugin({ id: PLUGIN_CONC_A_ID })], makeHostFactory, newImporter);
    expect(pluginRegistry.getSlotComponents(SLOT)).toEqual([NewWidget]);

    // The stale import finally resolves — it must bail before touching the
    // registry, leaving the newer registration intact (no unregister, no OldWidget).
    oldImportGate.resolve();
    await oldLoad;

    expect(pluginRegistry.getSlotComponents(SLOT)).toEqual([NewWidget]);
  });

  it("a superseded load whose initialize() registers after an await does not append a duplicate slot entry", async () => {
    const OldWidget = () => null;
    const NewWidget = () => null;
    const oldInitGate = deferred();

    let importCount = 0;
    const importer = async (_url: string) => {
      importCount += 1;
      const isOld = importCount === 1;
      registerFake(PLUGIN_CONC_B_ID, {
        initialize: async (registry: PluginRegistry) => {
          if (isOld) {
            await oldInitGate.promise; // registers post-await, after being superseded
            registry.registerComponent(SLOT, OldWidget);
          } else {
            registry.registerComponent(SLOT, NewWidget);
          }
        },
      });
      return {};
    };

    // Older load reaches its awaiting initialize() and parks there. Flush a
    // full macrotask so it is guaranteed past the import fence and parked at the
    // gate before we supersede it — exercising the post-await register path.
    const oldLoad = loadPlugins(
      [activePlugin({ id: PLUGIN_CONC_B_ID })],
      makeHostFactory,
      importer,
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
    // The update path evicts the cached bundle then reloads — the newer load
    // registers NewWidget and becomes the current generation.
    unloadPlugin(PLUGIN_CONC_B_ID, { evictCache: true });
    await loadPlugins([activePlugin({ id: PLUGIN_CONC_B_ID })], makeHostFactory, importer);
    expect(pluginRegistry.getSlotComponents(SLOT)).toEqual([NewWidget]);

    // The stale initialize resumes and tries to register — the generation fence
    // must drop it so exactly one slot registration remains.
    oldInitGate.resolve();
    await oldLoad;

    expect(pluginRegistry.getSlotComponents(SLOT)).toEqual([NewWidget]);
  });
});

describe("unloadPlugin — evictCache option", () => {
  const PLUGIN_EVICT_A_ID = "plugin-evict-a";

  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_EVICT_A_ID);
  });

  it("re-imports the bundle on the next load when evictCache is true, unlike the default reuse-cached-registration behavior", async () => {
    const firstInitialize = vi.fn();
    const secondInitialize = vi.fn();
    let importCount = 0;
    const importer = vi.fn(async (_url: string) => {
      importCount += 1;
      registerFake(PLUGIN_EVICT_A_ID, {
        initialize: importCount === 1 ? firstInitialize : secondInitialize,
      });
      return {};
    });

    await loadPlugins([activePlugin({ id: PLUGIN_EVICT_A_ID })], makeHostFactory, importer);
    expect(importCount).toBe(1);
    expect(firstInitialize).toHaveBeenCalledTimes(1);

    unloadPlugin(PLUGIN_EVICT_A_ID, { evictCache: true });
    await loadPlugins([activePlugin({ id: PLUGIN_EVICT_A_ID })], makeHostFactory, importer);

    expect(importCount).toBe(2);
    expect(secondInitialize).toHaveBeenCalledTimes(1);
  });
});

describe("disable then re-enable in the same session", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_REENABLE_A_ID);
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
});
