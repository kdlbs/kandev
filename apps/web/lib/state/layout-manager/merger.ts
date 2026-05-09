import type { DockviewApi } from "dockview-react";
import type { LayoutState, LayoutPanel } from "./types";
import { fromDockviewApi } from "./serializer";

/** Panel IDs that always come from the preset and should never be merged. */
const PRESET_ONLY_PANELS = new Set(["sidebar"]);

/** Panel IDs that, when surviving as extras, belong next to the chat in the
 *  center column rather than in the side column. The plan panel is paired
 *  with the chat conceptually, so toggling off plan mode keeps it visible
 *  in the center group rather than dumping it into the right column. */
const CENTER_EXTRA_PANELS = new Set(["plan"]);

/** Collect all panels from a LayoutState, flattened. */
function collectAllPanels(state: LayoutState): LayoutPanel[] {
  const panels: LayoutPanel[] = [];
  for (const col of state.columns) {
    for (const group of col.groups) {
      for (const panel of group.panels) {
        panels.push(panel);
      }
    }
  }
  return panels;
}

/** Collect the set of panel IDs present in a LayoutState. */
function collectPanelIds(state: LayoutState): Set<string> {
  return new Set(collectAllPanels(state).map((p) => p.id));
}

/** Identify the column that should receive non-session extras. Prefer the last
 *  non-sidebar, non-center column (e.g. "right", "plan", "preview", "vscode"),
 *  falling back to "center" when the preset has no such side column. */
function pickExtrasColumnId(targetPreset: LayoutState): string {
  const sideCols = targetPreset.columns.filter((c) => c.id !== "sidebar" && c.id !== "center");
  return sideCols[sideCols.length - 1]?.id ?? "center";
}

/** Append `toAdd` panels to a group's first leaf, preserving identity when no-op. */
function appendToFirstGroup(
  groups: LayoutState["columns"][number]["groups"],
  toAdd: LayoutPanel[],
  filterExisting?: (p: LayoutPanel) => boolean,
): LayoutState["columns"][number]["groups"] {
  return groups.map((group, idx) => {
    if (idx !== 0) return group;
    const basePanels = filterExisting ? group.panels.filter(filterExisting) : group.panels;
    const existingIds = new Set(basePanels.map((p) => p.id));
    const additions = toAdd.filter((p) => !existingIds.has(p.id));
    if (additions.length === 0 && basePanels.length === group.panels.length) return group;
    return { ...group, panels: [...basePanels, ...additions] };
  });
}

/**
 * Pure merge logic: merge extra panels from the current state into a target
 * preset layout. Session panels (`session:*`) replace the generic `chat`
 * panel in the center column. Other extras (files, changes, terminal, etc.)
 * are appended to the side column when one exists (e.g. "right"/"plan"/
 * "preview"/"vscode"), otherwise they fall back to the center column.
 */
export function mergePanelsIntoPreset(
  currentState: LayoutState,
  targetPreset: LayoutState,
): LayoutState {
  const currentPanels = collectAllPanels(currentState);
  const targetPanelIds = collectPanelIds(targetPreset);

  const extraPanels = currentPanels.filter(
    (p) => !targetPanelIds.has(p.id) && !PRESET_ONLY_PANELS.has(p.id),
  );

  if (extraPanels.length === 0) {
    return targetPreset;
  }

  const sessionExtras = extraPanels.filter((p) => p.id.startsWith("session:"));
  const centerExtras = extraPanels.filter((p) => CENTER_EXTRA_PANELS.has(p.id));
  const sideExtras = extraPanels.filter(
    (p) => !p.id.startsWith("session:") && !CENTER_EXTRA_PANELS.has(p.id),
  );
  const hasSessionPanels = sessionExtras.length > 0;
  const extrasColumnId = pickExtrasColumnId(targetPreset);

  const mergedColumns = targetPreset.columns.map((col) => {
    if (col.id === "center") {
      // Sessions always replace the generic "chat" placeholder in center.
      // Center-affinity extras (e.g. "plan") sit alongside the chat. When
      // the side column doesn't exist, side extras land here too.
      const fallbackSide = extrasColumnId === "center" ? sideExtras : [];
      const filterChat = hasSessionPanels ? (p: LayoutPanel) => p.id !== "chat" : undefined;
      const additions = [...sessionExtras, ...centerExtras, ...fallbackSide];
      if (additions.length === 0 && !filterChat) return col;
      return { ...col, groups: appendToFirstGroup(col.groups, additions, filterChat) };
    }
    if (col.id === extrasColumnId && sideExtras.length > 0) {
      return { ...col, groups: appendToFirstGroup(col.groups, sideExtras) };
    }
    return col;
  });

  return { columns: mergedColumns };
}

/**
 * Merge current live panels into a target preset layout.
 *
 * Captures panels from the current dockview state that aren't in the target
 * preset and appends them as tabs in the center group. This prevents panels
 * from being lost when switching layouts.
 */
export function mergeCurrentPanelsIntoPreset(
  api: DockviewApi,
  targetPreset: LayoutState,
): LayoutState {
  return mergePanelsIntoPreset(fromDockviewApi(api), targetPreset);
}
