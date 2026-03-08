import type { SerializedDockview } from "dockview-react";
import type { LayoutState, LayoutGroup } from "./types";
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

/**
 * Extract the set of structural component names from a SerializedDockview.
 *
 * Used for lightweight comparison: if the current live layout has the same
 * structural components as a saved layout, we can skip `fromJSON`.
 */
export function structuralComponentsFromSerialized(serialized: SerializedDockview): Set<string> {
  const components = new Set<string>();
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const panels = (serialized as any).panels as
    | Record<string, { contentComponent?: string }>
    | undefined;
  if (!panels) return components;
  for (const p of Object.values(panels)) {
    if (p.contentComponent && STRUCTURAL_COMPONENTS.has(p.contentComponent)) {
      components.add(p.contentComponent);
    }
  }
  return components;
}

/** Extract structural component names from a LayoutState (filtered). */
export function structuralComponentsFromLayout(state: LayoutState): Set<string> {
  const components = new Set<string>();
  const filtered = filterEphemeral(state);
  for (const col of filtered.columns) {
    for (const group of col.groups) {
      for (const panel of group.panels) {
        if (STRUCTURAL_COMPONENTS.has(panel.component)) {
          components.add(panel.component);
        }
      }
    }
  }
  return components;
}

/**
 * Quick check: do a live LayoutState and a saved SerializedDockview have
 * the same set of structural panels?
 *
 * This is a weaker check than `layoutStructuresMatch` (doesn't verify
 * column arrangement) but works without deserializing the grid tree.
 * False positives are safe — worst case we skip `fromJSON` when the grid
 * arrangement differs, but the same panels are present.
 */
export function savedLayoutMatchesLive(live: LayoutState, saved: SerializedDockview): boolean {
  const liveSet = structuralComponentsFromLayout(live);
  const savedSet = structuralComponentsFromSerialized(saved);
  if (liveSet.size !== savedSet.size) return false;
  for (const c of liveSet) {
    if (!savedSet.has(c)) return false;
  }
  return true;
}
