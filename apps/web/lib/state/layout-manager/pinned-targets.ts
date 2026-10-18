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
  reusableRightTarget = null;
}

/**
 * The right column share resolved from a reusable layout's saved geometry, owned
 * by exactly one environment's active build. It ranks between an actual manual
 * width (always wins) and the stock responsive default, and is held as a ratio
 * so it scales with the workbench and stays under the runtime caps. In-memory
 * only: it is never a manual preference and never reaches portable storage.
 */
type ReusableRightTarget = { envId: string | null; ratio: number };

let reusableRightTarget: ReusableRightTarget | null = null;

/** Own the resolved right width (`rightWidth` of a `totalWidth` workbench) for `envId`. */
export function setReusableRightTarget(
  envId: string | null,
  rightWidth: number,
  totalWidth: number,
): void {
  const valid =
    Number.isFinite(rightWidth) && rightWidth > 0 && Number.isFinite(totalWidth) && totalWidth > 0;
  reusableRightTarget = valid ? { envId, ratio: rightWidth / totalWidth } : null;
}

/** The owned right width scaled to `totalWidth`, or null when `envId` is not the owner. */
export function getReusableRightWidth(
  envId: string | null | undefined,
  totalWidth: number | undefined,
): number | null {
  const owned = reusableRightTarget;
  if (!owned || envId === undefined || owned.envId !== envId) return null;
  if (totalWidth === undefined || !Number.isFinite(totalWidth) || totalWidth <= 0) return null;
  return Math.max(LAYOUT_PINNED_MIN_PX, Math.round(owned.ratio * totalWidth));
}

export function clearReusableRightTarget(): void {
  reusableRightTarget = null;
}
