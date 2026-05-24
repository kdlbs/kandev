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
}
