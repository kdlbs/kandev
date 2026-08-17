import { afterEach, describe, expect, it, vi } from "vitest";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { loadPlugins, unloadPlugin } from "./host";
import { pluginRegistry } from "./registry";
import type { ActivePlugin, PluginHostApi, PluginRegistry } from "./types";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "" }),
}));

const DECLARED_PROVIDER_PLUGIN_ID = "plugin-declared-provider";
const LEGACY_PROVIDER_PLUGIN_ID = "plugin-legacy-provider";

const NOOP_TOAST = new Proxy(() => 0, {
  get: () => () => 0,
}) as unknown as PluginHostApi["toast"];

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

function importerFor(
  pluginId: string,
  bundleUrl: string,
  initialize: (registry: PluginRegistry) => void,
) {
  return async (url: string) => {
    if (url !== bundleUrl) throw new Error(`unexpected bundle URL: ${url}`);
    (
      window as unknown as { registerKandevPlugin: (id: string, plugin: unknown) => void }
    ).registerKandevPlugin(pluginId, { initialize });
    return {};
  };
}

function repositoryProvider(id: string) {
  return {
    id,
    label: "Source Control",
    listRepositories: async () => [],
    matchesURL: () => false,
    listBranches: async () => [],
    inspectURL: async () => null,
  };
}

afterEach(() => {
  pluginRegistry.unregisterPlugin(DECLARED_PROVIDER_PLUGIN_ID);
  pluginRegistry.unregisterPlugin(LEGACY_PROVIDER_PLUGIN_ID);
});

describe("loadPlugins — repository provider declarations", () => {
  it("installs declared repository provider IDs before initialize and restores them on reload", async () => {
    const providerId = "source-control";
    const declarationSpy = vi.spyOn(pluginRegistry, "setDeclaredRepositoryProviderIds");
    const initialize = vi.fn((registry: PluginRegistry) => {
      expect(declarationSpy).toHaveBeenLastCalledWith(DECLARED_PROVIDER_PLUGIN_ID, [providerId]);
      registry.registerRepositoryProvider(repositoryProvider(providerId));
    });
    const bundleUrl = "/declared-provider.js";
    const plugin = activePlugin({
      id: DECLARED_PROVIDER_PLUGIN_ID,
      bundleUrl,
      repositoryProviderIds: [providerId],
    });

    await loadPlugins(
      [plugin],
      makeHostFactory,
      importerFor(DECLARED_PROVIDER_PLUGIN_ID, bundleUrl, initialize),
    );
    expect(pluginRegistry.getRepositoryProvider(providerId)?.pluginId).toBe(
      DECLARED_PROVIDER_PLUGIN_ID,
    );

    unloadPlugin(DECLARED_PROVIDER_PLUGIN_ID);
    await loadPlugins(
      [plugin],
      makeHostFactory,
      importerFor(DECLARED_PROVIDER_PLUGIN_ID, bundleUrl, initialize),
    );

    expect(initialize).toHaveBeenCalledTimes(2);
    expect(pluginRegistry.getRepositoryProvider(providerId)?.pluginId).toBe(
      DECLARED_PROVIDER_PLUGIN_ID,
    );
    declarationSpy.mockRestore();
  });

  it("keeps repository provider registration compatible for boot payloads without declarations", async () => {
    const providerId = "legacy-source-control";
    const bundleUrl = "/legacy-provider.js";
    const initialize = vi.fn((registry: PluginRegistry) => {
      registry.registerRepositoryProvider(repositoryProvider(providerId));
    });

    await loadPlugins(
      [activePlugin({ id: LEGACY_PROVIDER_PLUGIN_ID, bundleUrl })],
      makeHostFactory,
      importerFor(LEGACY_PROVIDER_PLUGIN_ID, bundleUrl, initialize),
    );

    expect(initialize).toHaveBeenCalledTimes(1);
    expect(pluginRegistry.getRepositoryProvider(providerId)?.pluginId).toBe(
      LEGACY_PROVIDER_PLUGIN_ID,
    );
  });
});
