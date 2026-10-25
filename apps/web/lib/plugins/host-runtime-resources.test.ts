import { describe, expect, it, vi } from "vitest";

import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { generationFencedHost, PluginLoadResources } from "./host-runtime-resources";
import type { PluginHostApi } from "./types";

function makeHost(setIntegrationEnabled: PluginHostApi["setIntegrationEnabled"]): PluginHostApi {
  return {
    pluginId: "plugin-a",
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

  it("returns inert conversation capabilities after generation revocation", async () => {
    let current = true;
    const host = makeHost(vi.fn() as PluginHostApi["setIntegrationEnabled"]);
    const loadMore = vi.fn(async () => 1);
    const retry = vi.fn();
    host.conversation = {
      useSessionMessages: () => ({
        messages: [{ id: "message-1" }] as never,
        loading: false,
        hydrated: true,
        loadingMore: false,
        error: null,
        hasMore: true,
        removed: false,
        loadMore,
        retry,
      }),
      useSessionTurns: () => ({
        turns: [{ id: "turn-1" }] as never,
        loading: false,
        hydrated: true,
        error: null,
        removed: false,
        retry,
      }),
      useMessageFavorite: () => true,
    };
    const fenced = generationFencedHost(
      host,
      () => current,
      new PluginLoadResources(host.pluginId),
    );

    expect(
      fenced.conversation.useSessionMessages({ sessionId: "session-1" }).messages,
    ).toHaveLength(1);
    current = false;
    const staleMessages = fenced.conversation.useSessionMessages({ sessionId: "session-1" });
    const staleTurns = fenced.conversation.useSessionTurns("session-1");

    expect(staleMessages.messages).toEqual([]);
    expect(await staleMessages.loadMore()).toBe(0);
    staleMessages.retry();
    staleTurns.retry();
    expect(retry).not.toHaveBeenCalled();
    expect(fenced.conversation.useMessageFavorite("session-1", "message-1")).toBe(false);
  });
});
