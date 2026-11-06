import type { SerializedDockview } from "dockview-react";
import type { LayoutState, LayoutGroup, LayoutNode } from "./types";
import { filterEphemeral } from "./serializer";
import { STRUCTURAL_COMPONENTS } from "./constants";

/**
 * Extract a structural fingerprint from a group: sorted component names.
 * Ignores panel IDs, params, sizes — only the *kind* of panels matters.
 */
function groupFingerprint(group: LayoutGroup): string {
  return group.panels
    .map((p) => p.component)
    .sort()
    .join(",");
}

/**
 * Check whether two LayoutStates have the same structural skeleton.
 *
 * Both layouts are filtered through `filterEphemeral` first so that
 * transient panels (file-editors, diffs, commit-details) don't cause
 * false mismatches.
 *
 * Two layouts match when:
 *  - Same number of columns, same column IDs in the same order
 *  - Each column has the same number of groups
 *  - Each group has the same set of panel component types
 *
 * Sizes, proportions, active panels, and panel params are ignored.
 */
export function layoutStructuresMatch(a: LayoutState, b: LayoutState): boolean {
  const fa = filterEphemeral(a);
  const fb = filterEphemeral(b);

  if (fa.columns.length !== fb.columns.length) return false;

  for (let i = 0; i < fa.columns.length; i++) {
    const colA = fa.columns[i];
    const colB = fb.columns[i];
    if (colA.id !== colB.id) return false;
    if (colA.groups.length !== colB.groups.length) return false;
    for (let j = 0; j < colA.groups.length; j++) {
      if (groupFingerprint(colA.groups[j]) !== groupFingerprint(colB.groups[j])) {
        return false;
      }
    }
  }

  return true;
}

type StructuralShape =
  | { type: "leaf"; components: string[] }
  | { type: "branch"; children: StructuralShape[] };

function structuralGroup(components: string[]): StructuralShape {
  return {
    type: "leaf",
    components: components.filter((component) => STRUCTURAL_COMPONENTS.has(component)).sort(),
  };
}

function liveGroupShape(group: LayoutGroup): StructuralShape {
  return structuralGroup(group.panels.map((panel) => panel.component));
}

function liveNodeShape(node: LayoutNode): StructuralShape {
  if (node.type === "leaf") return liveGroupShape(node.group);
  return { type: "branch", children: node.children.map(liveNodeShape) };
}

function liveLayoutShape(live: LayoutState): StructuralShape {
  return {
    type: "branch",
    children: live.columns.map((column) => {
      if (column.tree) return liveNodeShape(column.tree);
      if (column.groups.length === 1) return liveGroupShape(column.groups[0]);
      return { type: "branch", children: column.groups.map(liveGroupShape) };
    }),
  };
}

function savedNodeShape(
  node: SerializedDockview["grid"]["root"],
  panels: SerializedDockview["panels"],
): StructuralShape {
  if (Array.isArray(node.data))
    return { type: "branch", children: node.data.map((child) => savedNodeShape(child, panels)) };
  return structuralGroup(node.data.views.map((id) => panels[id]?.contentComponent ?? ""));
}

/** Reuse a saved layout only when its split tree and structural groups match.
 * Sizes, active tabs, and environment-owned panel IDs are restored separately. */
export function savedLayoutMatchesLive(live: LayoutState, saved: SerializedDockview): boolean {
  if (!saved.grid?.root || !saved.panels) return false;
  if ((live.rootOrientation ?? "HORIZONTAL") !== saved.grid.orientation) return false;
  return (
    JSON.stringify(liveLayoutShape(live)) ===
    JSON.stringify(savedNodeShape(saved.grid.root, saved.panels))
  );
}
