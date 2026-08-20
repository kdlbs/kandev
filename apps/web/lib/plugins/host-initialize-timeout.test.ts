import { afterEach, describe, expect, it, vi } from "vitest";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { loadPlugins, unloadPlugin } from "./host";
import { pluginRegistry } from "./registry";
import type { ActivePlugin, PluginHostApi, PluginRegistry } from "./types";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "" }),
}));

const PLUGIN_HANG_A_ID = "plugin-hang-a";
const PLUGIN_HANG_B_ID = "plugin-hang-b";

type FakeWindow = Window & {
  registerKandevPlugin: (id: string, plugin: unknown) => void;
};

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

afterEach(() => {
  unloadPlugin(PLUGIN_HANG_A_ID, { evictCache: true });
  unloadPlugin(PLUGIN_HANG_B_ID, { evictCache: true });
});

describe("loadPlugins — initialize() timeout isolation", () => {
  it("does not let a hung plugin block the next plugin", async () => {
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    const secondInitialize = vi.fn((registry: PluginRegistry) => {
      registry.registerNavItem({ id: "nav-hang-b", label: "B", path: "/plugin-hang-b" });
    });
    const importer = fakeImporterFor({
      "/hang-bundle.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_HANG_A_ID, {
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
      10,
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

  it("fences registrations that arrive after initialize times out", async () => {
    let finishInitialize!: () => void;
    const initializeFinished = new Promise<void>((resolve) => {
      finishInitialize = resolve;
    });
    const importer = fakeImporterFor({
      "/late-registration.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_HANG_A_ID, {
          initialize: async (registry: PluginRegistry) => {
            await initializeFinished;
            registry.registerNavItem({ id: "late-nav", label: "Late", path: "/late" });
          },
        }),
    });

    await loadPlugins(
      [activePlugin({ id: PLUGIN_HANG_A_ID, bundleUrl: "/late-registration.js" })],
      makeHostFactory,
      importer,
      window,
      10,
    );
    finishInitialize();
    await Promise.resolve();

    expect(pluginRegistry.getNavItems().find((item) => item.id === "late-nav")).toBeUndefined();
  });
});

describe("loadPlugins — timed-out host mutation isolation", () => {
  it("preserves the read-only localization capability on the fenced host", async () => {
    const translated = vi.fn();
    const importer = fakeImporterFor({
      "/localized.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_HANG_A_ID, {
          initialize: (_registry: PluginRegistry, host: PluginHostApi) => {
            translated(host.i18n.t("settings"));
          },
        }),
    });
    const hostFactory = (pluginId: string): PluginHostApi => ({
      ...makeHostFactory(pluginId),
      i18n: {
        locale: "en",
        t: (key) => `localized:${key}`,
        useTranslation: () => ({ locale: "en", t: (key) => `localized:${key}` }),
      },
    });

    await loadPlugins(
      [activePlugin({ id: PLUGIN_HANG_A_ID, bundleUrl: "/localized.js" })],
      hostFactory,
      importer,
      window,
      10,
    );

    expect(translated).toHaveBeenCalledWith("localized:settings");
  });

  it("closes UI resources opened before initialize times out", async () => {
    const modalClose = vi.fn();
    const taskLinkClose = vi.fn();
    const storeUnsubscribe = vi.fn();
    const toastDismiss = vi.fn();
    const toast = vi.fn(() => "owned-toast") as unknown as PluginHostApi["toast"];
    toast.dismiss = toastDismiss;
    const importer = fakeImporterFor({
      "/owned-ui-resources.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_HANG_A_ID, {
          initialize: (_registry: PluginRegistry, host: PluginHostApi) => {
            host.store.subscribe(() => {});
            host.openModal({ title: "Owned modal", content: () => null });
            host.openTaskLinkDialog({
              title: "Owned task link",
              description: "Link a change request",
              inputLabel: "URL",
              placeholder: "https://example.test/pull-requests/1",
              emptyError: "Required",
              failureMessage: "Failed",
              successMessage: "Linked",
              onSubmit: async () => {},
            });
            host.toast("Owned toast");
            return new Promise<void>(() => {});
          },
        }),
    });
    const hostFactory = (pluginId: string): PluginHostApi => ({
      ...makeHostFactory(pluginId),
      store: {
        ...makeHostFactory(pluginId).store,
        subscribe: () => storeUnsubscribe,
      },
      openModal: () => ({ close: modalClose }),
      openTaskLinkDialog: () => ({ close: taskLinkClose }),
      toast,
    });

    await loadPlugins(
      [activePlugin({ id: PLUGIN_HANG_A_ID, bundleUrl: "/owned-ui-resources.js" })],
      hostFactory,
      importer,
      window,
      10,
    );

    expect(storeUnsubscribe).toHaveBeenCalledOnce();
    expect(modalClose).toHaveBeenCalledOnce();
    expect(taskLinkClose).toHaveBeenCalledOnce();
    expect(toastDismiss).toHaveBeenCalledWith("owned-toast");
  });
});

describe("loadPlugins — timed-out subscription isolation", () => {
  it("revokes subscriptions and requests and calls destroy after initialize times out", async () => {
    const themeUnsubscribe = vi.fn();
    const storageUnsubscribe = vi.fn();
    const contextUnsubscribe = vi.fn();
    let requestSignal: AbortSignal | undefined;
    const destroy = vi.fn();
    const importer = fakeImporterFor({
      "/owned-resources.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_HANG_A_ID, {
          initialize: (_registry: PluginRegistry, host: PluginHostApi) => {
            host.onThemeChange(() => {});
            host.storage.subscribe({ scope: "instance" }, () => {});
            host.context.subscribeActiveWorkspace(() => {});
            void host.api.fetch("/slow").catch(() => undefined);
            return new Promise<void>(() => {});
          },
          destroy,
        }),
    });
    const hostFactory = (pluginId: string): PluginHostApi => ({
      ...makeHostFactory(pluginId),
      onThemeChange: () => themeUnsubscribe,
      api: {
        ...makeHostFactory(pluginId).api,
        fetch: async (_path, init) => {
          requestSignal = init?.signal ?? undefined;
          return await new Promise<Response>((_resolve, reject) => {
            requestSignal?.addEventListener(
              "abort",
              () => reject(new DOMException("aborted", "AbortError")),
              { once: true },
            );
          });
        },
      },
      storage: {
        ...makeHostFactory(pluginId).storage,
        subscribe: () => storageUnsubscribe,
      },
      context: {
        ...makeHostFactory(pluginId).context,
        subscribeActiveWorkspace: () => contextUnsubscribe,
      },
    });

    await loadPlugins(
      [activePlugin({ id: PLUGIN_HANG_A_ID, bundleUrl: "/owned-resources.js" })],
      hostFactory,
      importer,
      window,
      10,
    );

    expect(requestSignal?.aborted).toBe(true);
    expect(themeUnsubscribe).toHaveBeenCalledOnce();
    expect(storageUnsubscribe).toHaveBeenCalledOnce();
    expect(contextUnsubscribe).toHaveBeenCalledOnce();
    expect(destroy).toHaveBeenCalledOnce();
  });
});

describe("loadPlugins — late host mutation isolation", () => {
  it("fences host mutations that arrive after initialize times out", async () => {
    let finishInitialize!: () => void;
    const initializeFinished = new Promise<void>((resolve) => {
      finishInitialize = resolve;
    });
    const openModal = vi.fn(() => ({ close: () => {} }));
    const openTaskLinkDialog = vi.fn(() => ({ close: () => {} }));
    const navigate = vi.fn();
    const openTaskReview = vi.fn();
    const storageSet = vi.fn().mockResolvedValue({ updatedAt: "now" });
    const toast = vi.fn() as unknown as PluginHostApi["toast"];
    const setState = vi.fn();
    const importer = fakeImporterFor({
      "/late-host-mutation.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_HANG_A_ID, {
          initialize: async (_registry: PluginRegistry, host: PluginHostApi) => {
            await initializeFinished;
            host.openModal({ title: "Late modal", content: () => null });
            host.openTaskLinkDialog({
              title: "Late task link",
              description: "Link a change request",
              inputLabel: "URL",
              placeholder: "https://example.test/pull-requests/1",
              emptyError: "Required",
              failureMessage: "Failed",
              successMessage: "Linked",
              onSubmit: async () => {},
            });
            host.navigate("/late");
            host.store.setState({} as never);
            host.openTaskReview({
              providerId: "late",
              reviewKey: "late-review",
              connectionScope: "https://late.example.test",
              repositoryId: "late-repository",
              changeRequestNumber: 1,
              title: "Late review",
              presentation: "desktop",
            });
            host.toast("Late toast");
            await host.storage.set("instance", "user", "late", true);
          },
        }),
    });
    const hostFactory = (pluginId: string): PluginHostApi => ({
      ...makeHostFactory(pluginId),
      openModal,
      openTaskLinkDialog,
      navigate,
      openTaskReview,
      toast,
      storage: {
        ...makeHostFactory(pluginId).storage,
        set: storageSet,
      },
      store: {
        ...makeHostFactory(pluginId).store,
        setState,
      },
    });

    await loadPlugins(
      [activePlugin({ id: PLUGIN_HANG_A_ID, bundleUrl: "/late-host-mutation.js" })],
      hostFactory,
      importer,
      window,
      10,
    );
    finishInitialize();
    await Promise.resolve();

    expect(openModal).not.toHaveBeenCalled();
    expect(openTaskLinkDialog).not.toHaveBeenCalled();
    expect(navigate).not.toHaveBeenCalled();
    expect(setState).not.toHaveBeenCalled();
    expect(openTaskReview).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
    expect(storageSet).not.toHaveBeenCalled();
  });
});
