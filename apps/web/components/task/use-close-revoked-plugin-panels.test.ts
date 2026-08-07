import { afterEach, describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { pluginRegistry } from "@/lib/plugins/registry";
import { useCloseRevokedPluginPanels } from "./use-close-revoked-plugin-panels";

const PLUGIN_ID = "plugin-a";
const PLUGIN_PANEL_ID = "plugin:plugin-a:notes";
const REVOKED_PLUGIN_PANEL_ID = "plugin:plugin-gone:notes";
const UNKNOWN_PLUGIN_PANEL_ID = "plugin:plugin-unknown:notes";

type FakePanel = { id: string };

function makeFakeApi(panelIds: string[]) {
  const panels: FakePanel[] = panelIds.map((id) => ({ id }));
  const removed: string[] = [];
  const api = {
    get panels() {
      return panels;
    },
    removePanel(panel: FakePanel) {
      const i = panels.findIndex((p) => p.id === panel.id);
      if (i >= 0) panels.splice(i, 1);
      removed.push(panel.id);
    },
  };
  return { api, removed };
}

function registerNotesPanel() {
  function Notes() {
    return null;
  }
  pluginRegistry
    .forPlugin(PLUGIN_ID)
    .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes });
}

afterEach(() => {
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

describe("useCloseRevokedPluginPanels", () => {
  it("does nothing when api is null", () => {
    expect(() => renderHook(() => useCloseRevokedPluginPanels(null))).not.toThrow();
  });

  it("leaves non-plugin panels and registered plugin panels alone", () => {
    registerNotesPanel();
    const { api } = makeFakeApi(["chat", PLUGIN_PANEL_ID]);

    renderHook(() => useCloseRevokedPluginPanels(api as never));

    expect(api.panels.map((p) => p.id)).toEqual(["chat", PLUGIN_PANEL_ID]);
  });

  it("closes a plugin panel whose plugin is no longer registered (AC4)", () => {
    pluginRegistry.markPluginReady("plugin-gone", 1);
    const { api, removed } = makeFakeApi(["chat", REVOKED_PLUGIN_PANEL_ID]);

    renderHook(() => useCloseRevokedPluginPanels(api as never));

    expect(api.panels.map((p) => p.id)).toEqual(["chat"]);
    expect(removed).toEqual([REVOKED_PLUGIN_PANEL_ID]);
  });

  it("preserves a plugin panel until its lifecycle snapshot is known", () => {
    const { api, removed } = makeFakeApi([UNKNOWN_PLUGIN_PANEL_ID]);

    renderHook(() => useCloseRevokedPluginPanels(api as never));

    expect(api.panels.map((p) => p.id)).toEqual([UNKNOWN_PLUGIN_PANEL_ID]);
    expect(removed).toEqual([]);
  });

  it("closes a plugin panel after its plugin is unregistered mid-session (AC4)", () => {
    registerNotesPanel();
    const { api } = makeFakeApi([PLUGIN_PANEL_ID]);

    const { rerender } = renderHook(() => useCloseRevokedPluginPanels(api as never));
    expect(api.panels.map((p) => p.id)).toEqual([PLUGIN_PANEL_ID]);

    pluginRegistry.unregisterPlugin(PLUGIN_ID);
    pluginRegistry.markPluginReady(PLUGIN_ID, 11);
    rerender();

    expect(api.panels.map((p) => p.id)).toEqual([]);
  });

  it("preserves missing panels while a plugin generation is loading or failed", () => {
    registerNotesPanel();
    const { api } = makeFakeApi([PLUGIN_PANEL_ID]);

    const { rerender } = renderHook(() => useCloseRevokedPluginPanels(api as never));

    pluginRegistry.markPluginLoading(PLUGIN_ID, 20);
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
    rerender();

    expect(api.panels.map((p) => p.id)).toEqual([PLUGIN_PANEL_ID]);

    pluginRegistry.markPluginFailed(PLUGIN_ID, 20);
    rerender();

    expect(api.panels.map((p) => p.id)).toEqual([PLUGIN_PANEL_ID]);

    pluginRegistry.markPluginReady(PLUGIN_ID, 20);
    rerender();

    expect(api.panels).toEqual([]);
  });

  it("closes every owned panel immediately after definitive removal", () => {
    const { api, removed } = makeFakeApi([
      "plugin:plugin-gone:notes",
      "plugin:plugin-gone:other",
      "chat",
    ]);

    pluginRegistry.markPluginRemoved("plugin-gone", 11);
    renderHook(() => useCloseRevokedPluginPanels(api as never));

    expect(api.panels.map((p) => p.id)).toEqual(["chat"]);
    expect(removed).toEqual(["plugin:plugin-gone:notes", "plugin:plugin-gone:other"]);
  });
});
