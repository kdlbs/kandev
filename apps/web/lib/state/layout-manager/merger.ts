import type { DockviewApi } from "dockview-react";
import type { LayoutState, LayoutPanel } from "./types";
import { fromDockviewApi } from "./serializer";

/** Panel IDs that always come from the preset and should never be merged. */
const PRESET_ONLY_PANELS = new Set(["sidebar"]);

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

/**
 * Pure merge logic: merge extra panels from the current state into a target
 * preset layout.  Session panels (`session:*`) replace the generic `chat`
 * panel rather than coexisting alongside it.
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

  const hasSessionPanels = extraPanels.some((p) => p.id.startsWith("session:"));

  console.debug(
    "[layout-merger] merging extra panels into preset:",
    extraPanels.map((p) => p.id),
  );

  const mergedColumns = targetPreset.columns.map((col) => {
    if (col.id !== "center") return col;

    const groups = col.groups.map((group, idx) => {
      if (idx !== 0) return group;

      // If extra panels include session tabs, drop the generic "chat"
      // placeholder — session panels replace it.
      const basePanels = hasSessionPanels
        ? group.panels.filter((p) => p.id !== "chat")
        : group.panels;

      const existingIds = new Set(basePanels.map((p) => p.id));
      const toAdd = extraPanels.filter((p) => !existingIds.has(p.id));
      if (toAdd.length === 0 && basePanels.length === group.panels.length) return group;

      return {
        ...group,
        panels: [...basePanels, ...toAdd],
      };
    });

    return { ...col, groups };
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
