import { describe, it, expect, vi, afterEach } from "vitest";
import type { Window as HappyDOMWindow } from "happy-dom";
import { loadPlugins, unloadPlugin } from "./host";
import { pluginRegistry } from "./registry";
import type { ActivePlugin, PluginHostApi, PluginRegistry } from "./types";

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
    api: { fetch: async () => new Response() },
    ui: {},
    theme: "light",
  };
}

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

describe("loadPlugins", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin("plugin-a");
    pluginRegistry.unregisterPlugin("plugin-b");
    document.head.querySelectorAll("link[rel='stylesheet']").forEach((el) => el.remove());
  });

  it("imports the bundle, then calls initialize(registry, host) with a registry scoped to the plugin", async () => {
    const initialize = vi.fn((registry: PluginRegistry, _host: PluginHostApi) => {
      registry.registerNavItem({ id: "nav-a", label: "A", path: "/plugin-a" });
    });
    const importer = fakeImporterFor({
      "/api/plugins/plugin-a/bundle": (win) => {
        (
          win as unknown as { registerKandevPlugin: (id: string, plugin: unknown) => void }
        ).registerKandevPlugin("plugin-a", { initialize });
      },
    });

    await loadPlugins([activePlugin()], makeHostFactory, importer);

    expect(initialize).toHaveBeenCalledTimes(1);
    const [, host] = initialize.mock.calls[0];
    expect(host.pluginId).toBe("plugin-a");
    expect(pluginRegistry.getNavItems()).toContainEqual({
      id: "nav-a",
      label: "A",
      path: "/plugin-a",
    });
  });

  it("injects styleUrls as <link> elements before importing the bundle", async () => {
    // happy-dom eagerly loads real <link rel="stylesheet"> hrefs over the network;
    // disable that for this test so it doesn't attempt (and 404-log) a real fetch.
    const happyDOMWindow = window as unknown as HappyDOMWindow;
    happyDOMWindow.happyDOM.settings.disableCSSFileLoading = true;
    const importer = fakeImporterFor({
      "/bundle.js": (win) => {
        (
          win as unknown as { registerKandevPlugin: (id: string, plugin: unknown) => void }
        ).registerKandevPlugin("plugin-a", { initialize: () => {} });
      },
    });

    await loadPlugins(
      [activePlugin({ bundleUrl: "/bundle.js", styleUrls: ["/plugin-a.css"] })],
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
      "/bad-bundle.js": (win) => {
        (
          win as unknown as { registerKandevPlugin: (id: string, plugin: unknown) => void }
        ).registerKandevPlugin("plugin-a", {
          initialize: () => {
            throw new Error("boom");
          },
        });
      },
      "/good-bundle.js": (win) => {
        (
          win as unknown as { registerKandevPlugin: (id: string, plugin: unknown) => void }
        ).registerKandevPlugin("plugin-b", { initialize: goodInitialize });
      },
    });

    await loadPlugins(
      [
        activePlugin({ id: "plugin-a", bundleUrl: "/bad-bundle.js" }),
        activePlugin({ id: "plugin-b", bundleUrl: "/good-bundle.js" }),
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
      [activePlugin({ bundleUrl: "/silent-bundle.js" })],
      makeHostFactory,
      importer,
    );

    expect(errorSpy).toHaveBeenCalled();
    errorSpy.mockRestore();
  });
});

describe("unloadPlugin", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin("plugin-a");
  });

  it("calls destroy() and bulk-revokes the plugin's registrations", async () => {
    const destroy = vi.fn();
    const importer = fakeImporterFor({
      "/bundle.js": (win) => {
        (
          win as unknown as { registerKandevPlugin: (id: string, plugin: unknown) => void }
        ).registerKandevPlugin("plugin-a", {
          initialize: (registry: { registerNavItem: (item: unknown) => void }) => {
            registry.registerNavItem({ id: "nav-a", label: "A", path: "/plugin-a" });
          },
          destroy,
        });
      },
    });
    await loadPlugins([activePlugin({ bundleUrl: "/bundle.js" })], makeHostFactory, importer);
    expect(pluginRegistry.getNavItems()).toContainEqual({
      id: "nav-a",
      label: "A",
      path: "/plugin-a",
    });

    unloadPlugin("plugin-a");

    expect(destroy).toHaveBeenCalledTimes(1);
    expect(pluginRegistry.getNavItems().find((item) => item.id === "nav-a")).toBeUndefined();
  });

  it("swallows a throwing destroy() and still bulk-revokes registrations", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const importer = fakeImporterFor({
      "/bundle.js": (win) => {
        (
          win as unknown as { registerKandevPlugin: (id: string, plugin: unknown) => void }
        ).registerKandevPlugin("plugin-a", {
          initialize: (registry: { registerNavItem: (item: unknown) => void }) => {
            registry.registerNavItem({ id: "nav-a", label: "A", path: "/plugin-a" });
          },
          destroy: () => {
            throw new Error("destroy boom");
          },
        });
      },
    });
    await loadPlugins([activePlugin({ bundleUrl: "/bundle.js" })], makeHostFactory, importer);

    expect(() => unloadPlugin("plugin-a")).not.toThrow();
    expect(errorSpy).toHaveBeenCalled();
    expect(pluginRegistry.getNavItems().find((item) => item.id === "nav-a")).toBeUndefined();
    errorSpy.mockRestore();
  });
});
