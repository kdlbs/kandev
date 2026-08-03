/**
 * Sentinel step ID used to collect tasks whose workflow_step_id no longer
 * matches any rendered column. Tasks remapped here are visible as a
 * "Needs Reassignment" fallback column so they are never silently hidden.
 */
export const ORPHAN_STEP_ID = "__kandev_orphan__";

export const ORPHAN_STEP = {
  id: ORPHAN_STEP_ID,
  title: "Needs Reassignment",
  color: "#f59e0b",
} as const;

/**
 * The "Needs Reassignment" column is a display-only fallback, not a real
 * workflow step — it must never be offered as a manual move destination
 * (drag-and-drop, "Move to" menus, or Pipeline navigation).
 */
export function isOrphanMoveTarget(targetStepId: string): boolean {
  return targetStepId === ORPHAN_STEP_ID;
}
