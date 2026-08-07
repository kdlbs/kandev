import { afterEach, describe, expect, it } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import {
  isKnownPanelId,
  isStructuralComponent,
  parsePluginPanelId,
  pluginPanelId,
  resolvePluginPanelDefinition,
} from "./plugin-panels";

afterEach(() => {
  pluginRegistry.unregisterPlugin("plugin-a");
});

describe("pluginPanelId / parsePluginPanelId", () => {
  it("round-trips a plugin id and panel key", () => {
    const id = pluginPanelId("kandev-plugin-notes", "notes");
    expect(id).toBe("plugin:kandev-plugin-notes:notes");
    expect(parsePluginPanelId(id)).toEqual({ pluginId: "kandev-plugin-notes", panelKey: "notes" });
  });

  it("returns undefined for a non-plugin id", () => {
    expect(parsePluginPanelId("chat")).toBeUndefined();
    expect(parsePluginPanelId("plugin:")).toBeUndefined();
    expect(parsePluginPanelId("plugin::notes")).toBeUndefined();
    expect(parsePluginPanelId("plugin:kandev-plugin-notes:")).toBeUndefined();
  });
});

describe("isKnownPanelId", () => {
  it("is true for a fixed known panel id", () => {
    expect(isKnownPanelId("chat")).toBe(true);
    expect(isKnownPanelId("plan")).toBe(true);
  });

  it("is true for a plugin panel id whose plugin still has it registered", () => {
    function Notes() {
      return null;
    }
    pluginRegistry
      .forPlugin("plugin-a")
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes });

    expect(isKnownPanelId(pluginPanelId("plugin-a", "notes"))).toBe(true);
  });

  it("is false for a plugin panel id of an uninstalled/unregistered plugin (AC5)", () => {
    expect(isKnownPanelId(pluginPanelId("plugin-gone", "notes"))).toBe(false);
  });

  it("is false for an arbitrary unknown id", () => {
    expect(isKnownPanelId("some-random-id")).toBe(false);
  });
});

describe("isStructuralComponent", () => {
  it("is true for fixed structural components and the generic plugin-panel component", () => {
    expect(isStructuralComponent("chat")).toBe(true);
    expect(isStructuralComponent("plugin-panel")).toBe(true);
  });

  it("is false for a non-structural component", () => {
    expect(isStructuralComponent("file-editor")).toBe(false);
  });
});

describe("resolvePluginPanelDefinition", () => {
  it("resolves a registered plugin panel to a full LayoutPanel", () => {
    function Notes() {
      return null;
    }
    pluginRegistry
      .forPlugin("plugin-a")
      .registerTaskPanel({ id: "notes", title: "Notes", icon: "file-text", Component: Notes });

    const id = pluginPanelId("plugin-a", "notes");
    expect(resolvePluginPanelDefinition(id)).toEqual({
      id,
      component: "plugin-panel",
      title: "Notes",
      tabComponent: "pluginPanelTab",
      params: { pluginId: "plugin-a", panelKey: "notes" },
    });
  });

  it("returns undefined when the plugin/panel is no longer registered (AC5)", () => {
    expect(resolvePluginPanelDefinition(pluginPanelId("plugin-gone", "notes"))).toBeUndefined();
  });

  it("returns undefined for a malformed id", () => {
    expect(resolvePluginPanelDefinition("chat")).toBeUndefined();
  });
});
