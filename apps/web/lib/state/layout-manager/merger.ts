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
  const currentState = fromDockviewApi(api);
  const currentPanels = collectAllPanels(currentState);
  const targetPanelIds = collectPanelIds(targetPreset);

  const extraPanels = currentPanels.filter(
    (p) => !targetPanelIds.has(p.id) && !PRESET_ONLY_PANELS.has(p.id),
  );

  if (extraPanels.length === 0) {
    return targetPreset;
  }

  console.debug(
    "[layout-merger] merging extra panels into preset:",
    extraPanels.map((p) => p.id),
  );

  const mergedColumns = targetPreset.columns.map((col) => {
    if (col.id !== "center") return col;

    const groups = col.groups.map((group, idx) => {
      if (idx !== 0) return group;

      const existingIds = new Set(group.panels.map((p) => p.id));
      const toAdd = extraPanels.filter((p) => !existingIds.has(p.id));
      if (toAdd.length === 0) return group;

      return {
        ...group,
        panels: [...group.panels, ...toAdd],
      };
    });

    return { ...col, groups };
  });

  return { columns: mergedColumns };
}
