/**
 * Pinned-column target widths.
 *
 * Dockview's splitview rebalances proportionally on every `api.layout` call,
 * which would otherwise grow sidebar/right past their initial defaults on
 * container expansion and shrink them on container contraction. Instead of
 * fighting dockview with `setConstraints` locks (which ratchets the column
 * down on transient contractions), we track an explicit *target width* per
 * pinned column and force-restore the column to that target after every
 * `onDidLayoutChange` event.
 *
 * Targets are updated only by deliberate actions:
 * - `applyLayout` records the computed default width when building a layout.
 * - `applyLayoutFixups` records the just-restored width after `fromJSON`.
 * - The sash-drag handler in `dockview-layout-setup.ts` records the new
 *   width on `mouseup` so future rebalances restore to the user's choice.
 */
import { LAYOUT_PINNED_MIN_PX } from "./caps";
import type { LayoutColumn, LayoutState } from "./types";
import { isSessionChatPanel } from "./session-panels";
import { computeColumnWidths } from "./sizing";

const targets = new Map<"sidebar" | "right", number>();

export function setPinnedTarget(column: "sidebar" | "right", width: number): void {
  if (!Number.isFinite(width) || width <= 0) return;
  targets.set(column, Math.max(width, LAYOUT_PINNED_MIN_PX));
}

export function getPinnedTarget(column: "sidebar" | "right"): number | undefined {
  return targets.get(column);
}

export function clearPinnedTarget(column: "sidebar" | "right"): void {
  targets.delete(column);
}

export function clearAllPinnedTargets(): void {
  targets.clear();
  reusableRightTarget = null;
}

/**
 * Unclamped reusable source geometry, owned by exactly one environment.
 * Complete snapshots scale by their saved share; incomplete snapshots keep
 * the saved absolute width. Runtime caps are applied only when resolving it.
 */
type ReusableRootGeometry = { columns: LayoutColumn[]; anchors: Set<string>[] };

function rootPanelIds(column: LayoutColumn): Set<string> {
  return new Set(
    column.groups.flatMap((group) =>
      group.panels.map((panel) => (isSessionChatPanel(panel) ? "chat" : panel.id)),
    ),
  );
}

function captureRootGeometry(layout: LayoutState): ReusableRootGeometry | null {
  const anchors = layout.columns.map(rootPanelIds);
  if (anchors.some((ids) => ids.size === 0)) return null;
  return {
    columns: layout.columns.map((column) => ({
      id: column.id,
      width: column.width,
      pinned: column.pinned,
      groups: [],
      // Pinned overrides are already resolved from the live constrained extents.
      minWidth: 0,
      maxWidth: Infinity,
    })),
    anchors,
  };
}

type ReusableRightTarget = {
  envId: string | null;
  ratio: number | null;
  width: number;
  minWidth: number;
  maxWidth: number;
  root: ReusableRootGeometry | null;
};

let reusableRightTarget: ReusableRightTarget | null = null;

/** Claim saved source geometry, never a runtime-clamped pixel result. */
export function setReusableRightTarget(envId: string | null, layout: LayoutState): void {
  const right = layout.columns.find((column) => column.id === "right" && column.pinned);
  const validWidth = (width: number | undefined): width is number =>
    typeof width === "number" && Number.isFinite(width) && width > 0;
  if (layout.rootOrientation === "VERTICAL" || !right || !validWidth(right.width)) {
    reusableRightTarget = null;
    return;
  }
  const complete = layout.columns.every((column) => validWidth(column.width));
  const total = complete ? layout.columns.reduce((sum, column) => sum + column.width!, 0) : 0;
  reusableRightTarget = {
    envId,
    ratio: total > 0 ? right.width / total : null,
    width: right.width,
    minWidth: right.minWidth ?? LAYOUT_PINNED_MIN_PX,
    maxWidth: right.maxWidth ?? Infinity,
    root: complete ? captureRootGeometry(layout) : null,
  };
}

/** The owned right width scaled to `totalWidth`, or null when `envId` is not the owner. */
export function getReusableRightWidth(
  envId: string | null | undefined,
  totalWidth: number | undefined,
): number | null {
  const owned = reusableRightTarget;
  if (!owned || envId === undefined || owned.envId !== envId) return null;
  if (totalWidth === undefined || !Number.isFinite(totalWidth) || totalWidth <= 0) return null;
  const width = owned.ratio === null ? owned.width : Math.round(owned.ratio * totalWidth);
  return Math.max(owned.minWidth, Math.min(width, owned.maxWidth));
}

export function clearReusableRightTarget(): void {
  reusableRightTarget = null;
}

/** Stop automatic all-root allocation after an explicit root edit without changing right-width policy. */
export function clearReusableRootGeometry(envId: string | null | undefined): void {
  if (reusableRightTarget && reusableRightTarget.envId === envId) reusableRightTarget.root = null;
}

/** Resolve remaining root widths from source weights, preserving already constrained pinned extents. */
export function resolveReusableRootSizes(
  envId: string | null | undefined,
  live: LayoutState,
  width: number,
  rightTarget: number,
  retainedColumn?: { sourceIndex: number; column: LayoutColumn } | null,
): number[] | null {
  const owned = reusableRightTarget;
  if (!owned || owned.envId !== envId || !owned.root || live.rootOrientation === "VERTICAL")
    return null;
  const geometry = owned.root;
  const columns = retainedColumn
    ? live.columns.toSpliced(retainedColumn.sourceIndex, 0, retainedColumn.column)
    : live.columns;
  const liveAnchors = columns.map(rootPanelIds);
  const matches =
    columns.length === geometry.columns.length &&
    geometry.anchors.every((ids, index) => {
      let survives = false;
      for (const id of ids) {
        if (liveAnchors[index].has(id)) survives = true;
        // Removed tabs do not edit a surviving root; moving an original tab does.
        else if (liveAnchors.some((other) => other.has(id))) return false;
      }
      return survives;
    });
  if (!matches) {
    owned.root = null;
    return null;
  }
  // Validate the complete retained structure, but never allocate widths to absent live roots.
  if (retainedColumn) return null;
  const pinned = new Map<string, number>();
  geometry.columns.forEach((column, index) => {
    if (column.pinned)
      pinned.set(column.id, column.id === "right" ? rightTarget : (live.columns[index].width ?? 0));
  });
  const sizes = computeColumnWidths(geometry.columns, width, pinned);
  const lastFlexible = geometry.columns.findLastIndex((column) => !column.pinned);
  if (lastFlexible >= 0)
    sizes[lastFlexible] += Math.round(width - sizes.reduce((sum, size) => sum + size, 0));
  return sizes;
}
