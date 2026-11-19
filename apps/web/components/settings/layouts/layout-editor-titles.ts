import { useEffect, type RefObject } from "react";
import type { DockviewApi } from "dockview-react";
import { useTranslation } from "react-i18next";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { LayoutGroup, LayoutNode, LayoutState } from "@/lib/state/layout-manager";
import {
  PLUGIN_PANEL_COMPONENT,
  canonicalPluginPanelTitle,
  parsePluginPanelId,
  resolvePluginPanelDefinition,
} from "@/lib/state/layout-manager/plugin-panels";

/**
 * Canonical (stored) titles of the plugin panels in an editor, by full panel ID.
 *
 * Dockview only holds the localized display title, and the live registry is
 * the sole other source of the canonical one. Once a registration is gone the
 * canonical title would be unrecoverable, so the editor keeps its own copy for
 * as long as the panel is in the draft. It is transient editor state: never part
 * of plugin params or the portable layout schema.
 */
export type CanonicalTitles = Map<string, string>;

function isPluginPanel(panel: { id: string; component: string }): boolean {
  return panel.component === PLUGIN_PANEL_COMPONENT && parsePluginPanelId(panel.id) !== undefined;
}

function treeGroups(node: LayoutNode): LayoutGroup[] {
  return node.type === "leaf" ? [node.group] : node.children.flatMap(treeGroups);
}

/** Replace the map with the plugin panel titles stored in the layout being edited. */
export function seedCanonicalTitles(layout: LayoutState, titles: CanonicalTitles): void {
  titles.clear();
  for (const column of layout.columns) {
    const groups = column.tree ? treeGroups(column.tree) : column.groups;
    for (const group of groups) {
      for (const panel of group.panels) {
        if (isPluginPanel(panel)) titles.set(panel.id, panel.title);
      }
    }
  }
}

/**
 * Refresh entries from live registrations and drop panels that left the
 * editor. A missing registration never erases an entry.
 */
export function refreshCanonicalTitles(api: DockviewApi, titles: CanonicalTitles): void {
  const present = new Set<string>();
  for (const panel of api.panels) {
    if (parsePluginPanelId(panel.id) === undefined) continue;
    present.add(panel.id);
    const canonical = canonicalPluginPanelTitle(panel.id);
    if (canonical !== undefined) titles.set(panel.id, canonical);
  }
  for (const id of [...titles.keys()]) {
    if (!present.has(id)) titles.delete(id);
  }
}

function restoreGroup(group: LayoutGroup, titles: CanonicalTitles): LayoutGroup {
  return {
    ...group,
    panels: group.panels.map((panel) => {
      const canonical = isPluginPanel(panel) ? titles.get(panel.id) : undefined;
      return canonical === undefined ? panel : { ...panel, title: canonical };
    }),
  };
}

function restoreNode(node: LayoutNode, titles: CanonicalTitles): LayoutNode {
  if (node.type === "leaf") return { ...node, group: restoreGroup(node.group, titles) };
  return { ...node, children: node.children.map((child) => restoreNode(child, titles)) };
}

/** Put the retained canonical titles into the tree leaves and the flat projection of a capture. */
export function restoreCanonicalTitles(layout: LayoutState, titles: CanonicalTitles): LayoutState {
  return {
    ...layout,
    columns: layout.columns.map((column) => ({
      ...column,
      groups: column.groups.map((group) => restoreGroup(group, titles)),
      ...(column.tree ? { tree: restoreNode(column.tree, titles) } : {}),
    })),
  };
}

/**
 * Re-resolve each plugin tab's display title: the registered, localized copy
 * when available, otherwise the retained canonical title. Returns true when any
 * tab title changed.
 */
function reconcileDisplayTitles(api: DockviewApi, titles: CanonicalTitles): boolean {
  let changed = false;
  for (const panel of api.panels) {
    if (parsePluginPanelId(panel.id) === undefined) continue;
    const display = resolvePluginPanelDefinition(panel.id)?.title ?? titles.get(panel.id);
    if (display === undefined || display === panel.title) continue;
    panel.api.setTitle(display);
    changed = true;
  }
  return changed;
}

type DisplayTitleReconciliation = {
  api: DockviewApi | null;
  titles: CanonicalTitles;
  /** The editor's capture guard: set while the preview is mutated programmatically. */
  applyingRef: RefObject<boolean>;
  holdApplying: () => void;
  onChanged: () => void;
};

/**
 * Registration, availability, and locale changes only re-resolve tab titles.
 * They run under the capture guard so they never touch placement, selection,
 * or the draft.
 */
export function useDisplayTitleReconciliation({
  api,
  titles,
  applyingRef,
  holdApplying,
  onChanged,
}: DisplayTitleReconciliation): void {
  const { i18n } = useTranslation();
  const registryVersion = usePluginRegistry().getVersion();
  useEffect(() => {
    if (!api) return;
    const wasApplying = applyingRef.current;
    applyingRef.current = true;
    if (reconcileDisplayTitles(api, titles)) {
      holdApplying();
      onChanged();
    } else {
      applyingRef.current = wasApplying;
    }
  }, [api, titles, applyingRef, holdApplying, onChanged, registryVersion, i18n.language]);
}
