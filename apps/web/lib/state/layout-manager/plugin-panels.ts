/**
 * Identity helpers for plugin-contributed task panels (Approach A1,
 * docs/plans/plugins/PLUGIN-API.md). A plugin panel is modelled as a single
 * generic dockview component ("plugin-panel") whose identity lives in
 * `params: { pluginId, panelKey }` and whose panel id is
 * `plugin:<pluginId>:<panelKey>` — this keeps `renderPanel` to one branch
 * (rather than one per plugin) and lets a saved layout round-trip even when
 * the plugin that registered the panel is no longer installed.
 */
import { pluginRegistry } from "@/lib/plugins/registry";
import { KNOWN_PANEL_IDS, STRUCTURAL_COMPONENTS } from "./constants";
import type { LayoutPanel } from "./types";

const PLUGIN_PANEL_PREFIX = "plugin:";

/** The dockview `component` name every plugin panel shares. */
export const PLUGIN_PANEL_COMPONENT = "plugin-panel";

/** The dockview `tabComponent` name every plugin panel shares. */
export const PLUGIN_PANEL_TAB_COMPONENT = "pluginPanelTab";

export function pluginPanelId(pluginId: string, panelKey: string): string {
  return `${PLUGIN_PANEL_PREFIX}${pluginId}:${panelKey}`;
}

export interface ParsedPluginPanelId {
  pluginId: string;
  panelKey: string;
}

/** Parses a `plugin:<pluginId>:<panelKey>` panel id. undefined if malformed. */
export function parsePluginPanelId(id: string): ParsedPluginPanelId | undefined {
  if (!id.startsWith(PLUGIN_PANEL_PREFIX)) return undefined;
  const rest = id.slice(PLUGIN_PANEL_PREFIX.length);
  const sep = rest.indexOf(":");
  if (sep <= 0 || sep === rest.length - 1) return undefined;
  return { pluginId: rest.slice(0, sep), panelKey: rest.slice(sep + 1) };
}

/**
 * True for a fixed known panel id, or a plugin panel id whose plugin still
 * has that panel registered. Registry-aware counterpart to
 * `KNOWN_PANEL_IDS.has(id)` alone, so a saved layout referencing a
 * since-uninstalled plugin's panel is recognized as droppable rather than
 * "known" (AC5).
 */
export function isKnownPanelId(id: string): boolean {
  if (KNOWN_PANEL_IDS.has(id)) return true;
  const parsed = parsePluginPanelId(id);
  if (!parsed) return false;
  return pluginRegistry.getTaskPanel(parsed.pluginId, parsed.panelKey) !== undefined;
}

/**
 * True for a fixed structural component name, or the generic plugin-panel
 * component — every plugin panel is structural (survives filterEphemeral)
 * regardless of which plugin registered it.
 */
export function isStructuralComponent(component: string): boolean {
  return STRUCTURAL_COMPONENTS.has(component) || component === PLUGIN_PANEL_COMPONENT;
}

/**
 * Resolves a `plugin:<pluginId>:<panelKey>` id to a full `LayoutPanel`
 * definition using the plugin's current registration (title/icon), or
 * undefined if that id is malformed or the plugin/panel is no longer
 * registered (the caller should drop the panel rather than render it — AC5).
 */
export function resolvePluginPanelDefinition(id: string): LayoutPanel | undefined {
  const parsed = parsePluginPanelId(id);
  if (!parsed) return undefined;
  const registration = pluginRegistry.getTaskPanel(parsed.pluginId, parsed.panelKey);
  if (!registration) return undefined;
  return {
    id,
    component: PLUGIN_PANEL_COMPONENT,
    title: registration.title,
    tabComponent: PLUGIN_PANEL_TAB_COMPONENT,
    params: { pluginId: parsed.pluginId, panelKey: parsed.panelKey },
  };
}
