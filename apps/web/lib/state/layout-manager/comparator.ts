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
 * Build a component-count fingerprint from a SerializedDockview.
 * Returns a sorted "component:count" string for structural comparison.
 * Counts preserve multiplicity (e.g. two terminals ≠ one terminal).
 */
function componentCountsFromSerialized(serialized: SerializedDockview): string {
  const counts = new Map<string, number>();
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const panels = (serialized as any).panels as
    | Record<string, { contentComponent?: string }>
    | undefined;
  if (!panels) return "";
  for (const p of Object.values(panels)) {
    if (p.contentComponent && STRUCTURAL_COMPONENTS.has(p.contentComponent)) {
      counts.set(p.contentComponent, (counts.get(p.contentComponent) ?? 0) + 1);
    }
  }
  return [...counts.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, v]) => `${k}:${v}`)
    .join(",");
}

/** Build a component-count fingerprint from a LayoutState (filtered). */
function componentCountsFromLayout(state: LayoutState): string {
  const counts = new Map<string, number>();
  const filtered = filterEphemeral(state);
  for (const col of filtered.columns) {
    for (const group of col.groups) {
      for (const panel of group.panels) {
        if (STRUCTURAL_COMPONENTS.has(panel.component)) {
          counts.set(panel.component, (counts.get(panel.component) ?? 0) + 1);
        }
      }
    }
  }
  return [...counts.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, v]) => `${k}:${v}`)
    .join(",");
}

/**
 * Quick check: do a live LayoutState and a saved SerializedDockview have
 * the same structural panels (including multiplicity)?
 *
 * This is a weaker check than `layoutStructuresMatch` (doesn't verify
 * column arrangement) but works without deserializing the grid tree.
 * Counts are compared so two terminals ≠ one terminal.
 */
export function savedLayoutMatchesLive(live: LayoutState, saved: SerializedDockview): boolean {
  return componentCountsFromLayout(live) === componentCountsFromSerialized(saved);
}
