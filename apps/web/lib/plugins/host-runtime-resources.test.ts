import { describe, expect, it, vi } from "vitest";

import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { generationFencedHost, PluginLoadResources } from "./host-runtime-resources";
import type { PluginHostApi } from "./types";

function makeHost(setIntegrationEnabled: PluginHostApi["setIntegrationEnabled"]): PluginHostApi {
  return {
    pluginId: "plugin-a",
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
    ui: {} as PluginHostApi["ui"],
    i18n: {
      locale: "en",
      t: (key) => key,
      useTranslation: () => ({ locale: "en", t: (key: string) => key }),
    },
    useResponsiveBreakpoint,
    theme: "light",
    onThemeChange: () => () => {},
    navigate: () => {},
    openModal: () => ({ close: () => {} }),
    openTaskLinkDialog: () => ({ close: () => {} }),
    openTaskReview: () => {},
    toast: new Proxy(() => 0, { get: () => () => 0 }) as unknown as PluginHostApi["toast"],
    utils: {
      cn: () => "",
      generateUUID: () => "uuid",
      formatRelativeTime: () => "",
      integrationStatusRefreshMs: 90000,
    },
    useSettingsSaveContributor: () => {},
    setIntegrationEnabled,
    storage: {
      get: async () => undefined,
      set: async () => ({ updatedAt: "" }),
      delete: async () => {},
      list: async () => [],
      subscribe: () => () => {},
    },
  };
}

describe("generationFencedHost integration state", () => {
  it("forwards the integration id for the active generation and blocks stale writes", () => {
    const setIntegrationEnabled = vi.fn();
    let current = true;
    const host = makeHost(
      setIntegrationEnabled as unknown as PluginHostApi["setIntegrationEnabled"],
    );
    const fenced = generationFencedHost(
      host,
      () => current,
      new PluginLoadResources(host.pluginId),
    );

    const publish = fenced.setIntegrationEnabled as unknown as (
      integrationId: string,
      workspaceId: string,
      enabled: boolean,
    ) => void;
    publish("source-control", "workspace-1", true);
    current = false;
    publish("source-control", "workspace-1", false);

    expect(setIntegrationEnabled).toHaveBeenCalledExactlyOnceWith(
      "source-control",
      "workspace-1",
      true,
    );
  });
});
