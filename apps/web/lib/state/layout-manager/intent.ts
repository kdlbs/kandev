import type { LayoutState, LayoutIntentPanel, LayoutPanel } from "./types";
import { CENTER_GROUP, RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP } from "./constants";

/** Map short aliases to well-known group IDs. */
const GROUP_ALIASES: Record<string, string> = {
  center: CENTER_GROUP,
  "right-top": RIGHT_TOP_GROUP,
  "right-bottom": RIGHT_BOTTOM_GROUP,
};

function resolveGroupId(target: string): string {
  return GROUP_ALIASES[target] ?? target;
}

/** Inject LayoutIntentPanels into the matching groups of a LayoutState. */
export function injectIntentPanels(state: LayoutState, panels: LayoutIntentPanel[]): LayoutState {
  const columns = state.columns.map((col) => {
    const groups = col.groups.map((group) => {
      const matching = panels.filter((p) => {
        const targetId = resolveGroupId(p.targetGroup ?? "center");
        if (group.id === targetId) return true;
        // Fallback: match center column's first group when no explicit group ID
        if (targetId === CENTER_GROUP && col.id === "center" && col.groups[0] === group) {
          return true;
        }
        return false;
      });

      if (matching.length === 0) return group;

      const newPanels: LayoutPanel[] = matching.map((p) => ({
        id: p.id,
        component: p.component,
        title: p.title,
        ...(p.tabComponent ? { tabComponent: p.tabComponent } : {}),
        ...(p.params ? { params: p.params } : {}),
      }));

      return { ...group, panels: [...group.panels, ...newPanels] };
    });
    return { ...col, groups };
  });
  return { columns };
}

/** Set activePanel on groups that match the override map. */
export function applyActivePanelOverrides(
  state: LayoutState,
  overrides: Record<string, string>,
): LayoutState {
  const columns = state.columns.map((col) => {
    const groups = col.groups.map((group) => {
      if (!group.id || !overrides[group.id]) return group;
      const targetPanelId = overrides[group.id];
      if (!group.panels.some((p) => p.id === targetPanelId)) return group;
      return { ...group, activePanel: targetPanelId };
    });
    return { ...col, groups };
  });
  return { columns };
}
