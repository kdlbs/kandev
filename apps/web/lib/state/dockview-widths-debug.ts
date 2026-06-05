/**
 * Width snapshots for the dockview pinned-columns pipeline.
 *
 * Used by the `[dockview:widths]` debug namespace to trace where the
 * sidebar / center / right pixel widths come from. The pipeline has many
 * entry points (env-switch fast path, slow path, default build, container
 * ResizeObserver, enforcement loop, sash drag) and three sources of truth
 * (splitview live sizes, store `pinnedWidths`, `pinned-targets` map), so a
 * shared single-line format makes the console output easy to diff across
 * events when triaging "wrong width on env-prepare / fresh-load" bugs.
 */
import type { DockviewApi } from "dockview-react";
import { getRootSplitview, getPinnedTarget } from "./layout-manager";

export type ColumnWidthsSnapshot = {
  /** sidebar live width — splitview idx 0 (only meaningful when sidebar is visible). */
  left: number | null;
  /** Live width of the first column between sidebar and right. Null when fewer
   *  than 3 columns (we can't distinguish center from sidebar/right in a
   *  2-column layout without inspecting the layout model). */
  center: number | null;
  /** right column live width — splitview last index (only meaningful when
   *  rightPanelsVisible is true and >=2 columns exist). */
  right: number | null;
  /** Number of root splitview children (== column count). */
  columns: number;
  /** Dockview's internal grid width — diverges from the DOM container width
   *  after fromJSON with a stale recorded `grid.width`. Logging both lets us
   *  spot drift. */
  apiW: number;
  apiH: number;
};

export function snapshotColumnWidths(api: DockviewApi): ColumnWidthsSnapshot {
  const apiW = api.width;
  const apiH = api.height;
  const sv = getRootSplitview(api);
  if (!sv || sv.length === 0) {
    return { left: null, center: null, right: null, columns: 0, apiW, apiH };
  }
  const len = sv.length;
  const left = sv.getViewSize(0);
  const right = len >= 2 ? sv.getViewSize(len - 1) : null;
  const center = len >= 3 ? sv.getViewSize(1) : null;
  return { left, center, right, columns: len, apiW, apiH };
}

/** Format a width snapshot as a single string so devtools renders it inline
 *  rather than collapsing object args to "Object". Includes the live pinned
 *  targets so a "target mismatches live" bug is visible at a glance. */
export function formatWidthsSnapshot(snap: ColumnWidthsSnapshot): string {
  const round = (n: number | null): string => (n === null ? "-" : String(Math.round(n)));
  const tgtL = getPinnedTarget("sidebar");
  const tgtR = getPinnedTarget("right");
  const tL = tgtL === undefined ? "-" : String(Math.round(tgtL));
  const tR = tgtR === undefined ? "-" : String(Math.round(tgtR));
  return (
    `L=${round(snap.left)} C=${round(snap.center)} R=${round(snap.right)} ` +
    `cols=${snap.columns} api=${snap.apiW}x${snap.apiH} tgt=L${tL}/R${tR}`
  );
}
