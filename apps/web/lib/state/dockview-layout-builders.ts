import type { DockviewApi, AddPanelOptions } from "dockview-react";
import {
  SIDEBAR_LOCK,
  SIDEBAR_GROUP,
  CENTER_GROUP,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
  LAYOUT_PINNED_MIN_PX,
  computeSidebarMaxPx,
  computeRightMaxPx,
  getRootSplitview as getRootSplitviewImpl,
  resolveGroupIds,
  setPinnedTarget,
} from "./layout-manager";
import type { LayoutGroupIds } from "./layout-manager";
import { createDebugLogger, IS_DEBUG } from "@/lib/debug/log";

// Re-export for consumers that import from this module
export { getRootSplitview } from "./layout-manager";

const debugWidths = createDebugLogger("dockview:widths");

/** Best-effort caller chain for the fixups-capture debug log: pull the first
 *  few stack frames above `applyLayoutFixups` so we can see WHICH layout path
 *  (env-switch / restore / custom-layout / maximize) recorded a given target.
 *  Debug-only — never called when IS_DEBUG is false. */
const CALLER_CHAIN_SKIP = new Set(["captureCallerChain", "applyLayoutFixups", "logFixupsCapture"]);

function captureCallerChain(): string {
  const stack = new Error().stack;
  if (!stack) return "-";
  const lines = stack.split("\n").slice(1);
  const frames: string[] = [];
  for (const line of lines) {
    const m = /at (\S+)/.exec(line);
    const name = m?.[1] ?? "";
    if (!name || CALLER_CHAIN_SKIP.has(name)) continue;
    const short = name.split(".").pop() ?? name;
    frames.push(short);
    if (frames.length >= 3) break;
  }
  return frames.join("<") || "-";
}

/** After fromJSON() restores a session layout, apply fixups and return group IDs.
 *
 *  Apply loose runtime caps so the user can drag freely; the just-restored
 *  widths become the new pinned targets, and `enforcePinnedTargets` restores
 *  the column to that target on every subsequent rebalance. */
export function applyLayoutFixups(api: DockviewApi): LayoutGroupIds {
  const sv = getRootSplitviewImpl(api);
  captureSidebarTarget(api, sv);

  const oldChanges = api.getPanel("diff-files");
  if (oldChanges) oldChanges.api.setTitle("Changes");
  const oldFiles = api.getPanel("all-files");
  if (oldFiles) oldFiles.api.setTitle("Files");

  captureRightTarget(api, sv);

  logFixupsCapture(api, sv);

  return resolveGroupIds(api);
}

/** Lock + constrain the sidebar group and record its target width, clamped to
 *  the cap. The constraint pins the column at `sidebarCap`, so a target above
 *  it is unreachable and makes `enforcePinnedTargets` spin forever. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function captureSidebarTarget(api: DockviewApi, sv: any): void {
  const sb = api.getPanel("sidebar");
  if (!sb) return;
  const sidebarCap = computeSidebarMaxPx();
  sb.group.locked = SIDEBAR_LOCK;
  sb.group.header.hidden = false;
  sb.group.api.setConstraints({ maximumWidth: sidebarCap, minimumWidth: LAYOUT_PINNED_MIN_PX });
  const live = sv?.getViewSize?.(0) ?? sb.group.width;
  if (typeof live === "number" && live > 0) {
    setPinnedTarget("sidebar", Math.min(live, sidebarCap));
  }
}

/** Constrain the right column's groups and record its target width, clamped to
 *  the cap.
 *
 *  Only records a right target when a genuine right column exists AND it is a
 *  distinct column from the center (`sv.length >= 3`). In the 2-column
 *  global-fallback restore (right panels stripped as env-scoped, only
 *  sidebar + center remain), the LAST splitview child is the CENTER column —
 *  recording its width as the "right" target inflates the real right column
 *  once the full layout materializes, and the inflated width gets persisted
 *  into the env layout, so every later open loads broken widths. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function captureRightTarget(api: DockviewApi, sv: any): void {
  // Groups created from presets carry stable IDs (e.g. "group-right-top"),
  // so this works regardless of which panels are in them.
  const rightCap = computeRightMaxPx();
  let hasRightColumn = false;
  for (const gid of [RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP]) {
    const group = api.groups.find((g) => g.id === gid);
    if (group) {
      hasRightColumn = true;
      group.api.setConstraints({ maximumWidth: rightCap, minimumWidth: LAYOUT_PINNED_MIN_PX });
    }
  }
  if (!hasRightColumn || !sv || sv.length < 3) return;
  const liveRight = sv.getViewSize(sv.length - 1);
  if (typeof liveRight === "number" && liveRight > 0) {
    setPinnedTarget("right", Math.min(liveRight, rightCap));
  }
}

/** Emit the `fixups-capture` width snapshot. Pulled out of `applyLayoutFixups`
 *  to keep that function under the complexity limit; no-op in prod. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function logFixupsCapture(api: DockviewApi, sv: any): void {
  if (!IS_DEBUG) return;
  // Decisive fields: `sidebarOverCap=true` means the recorded target exceeds
  // the cap that enforcement clamps the column to — i.e. an unreachable target
  // that makes `enforcePinnedTargets` spin forever. `cols` shows whether the
  // layout was complete at capture (cols<3 → no real right column). api.width
  // vs window.innerWidth surfaces the window-fallback cap divergence.
  const sidebarCap = computeSidebarMaxPx();
  const innerW = typeof window !== "undefined" ? window.innerWidth : -1;
  const liveSidebar = sv?.getViewSize?.(0);
  // Threshold matches captureRightTarget (>= 3): in a 2-column layout the last
  // child is the center, not a right column, so we don't mislabel it here.
  const liveRight = sv && sv.length >= 3 ? sv.getViewSize(sv.length - 1) : undefined;
  const r = (n: number | undefined): string =>
    typeof n === "number" ? String(Math.round(n)) : "-";
  debugWidths(
    `fixups-capture caller=${captureCallerChain()} apiW=${api.width} innerW=${innerW} ` +
      `cols=${sv?.length ?? 0} sidebarCap=${Math.round(sidebarCap)} liveSidebar=${r(liveSidebar)} ` +
      `rightCap=${Math.round(computeRightMaxPx())} liveRight=${r(liveRight)} ` +
      `sidebarOverCap=${typeof liveSidebar === "number" && liveSidebar > sidebarCap + 1}`,
  );
}

/**
 * Resolve a fallback group position when the intended reference is stale.
 *
 * Tries to land in the center column, in this order:
 *   1. Well-known CENTER_GROUP id.
 *   2. Group containing the `chat` panel (post-drag, the well-known id may be
 *      gone but the chat panel still marks the center column).
 *   3. Group containing any `session:*` panel (active session: chat is removed
 *      and replaced with per-session tabs).
 *   4. Any group that is NOT the sidebar AND NOT a right-column group
 *      (Changes/Files/Terminal). Returning a right-column group would leak the
 *      panel into the narrow tools column — same UX bug as the sidebar leak.
 *
 * Returns undefined if no center-like group exists. The caller drops the
 * position so dockview picks a default. Never returns the sidebar.
 */
export function fallbackGroupPosition(api: DockviewApi): { referenceGroup: string } | undefined {
  const centerGroup = api.groups.find((g) => g.id === CENTER_GROUP);
  if (centerGroup) return { referenceGroup: centerGroup.id };

  const chatGroupId = api.getPanel("chat")?.group?.id;
  if (chatGroupId) return { referenceGroup: chatGroupId };

  const sessionGroupId = api.panels.find((p) => p.id.startsWith("session:"))?.group?.id;
  if (sessionGroupId) return { referenceGroup: sessionGroupId };

  const centerish = api.groups.find(
    (g) => g.id !== SIDEBAR_GROUP && g.id !== RIGHT_TOP_GROUP && g.id !== RIGHT_BOTTOM_GROUP,
  );
  if (centerish) return { referenceGroup: centerish.id };

  return undefined;
}

export function focusOrAddPanel(
  api: DockviewApi,
  options: AddPanelOptions & { id: string },
  quiet = false,
): void {
  const existing = api.getPanel(options.id);
  if (existing) {
    if (!quiet) existing.api.setActive();
    return;
  }
  // Guard: if the referenced group or panel no longer exists (stale ID after
  // layout transition), fall back to a known group. Avoid the active panel's
  // group because the user may have just clicked in the sidebar.
  const pos = options.position;
  if (pos && "referenceGroup" in pos) {
    const groupExists = api.groups.some((g) => g.id === pos.referenceGroup);
    if (!groupExists) {
      const fallback = fallbackGroupPosition(api);
      options = fallback
        ? { ...options, position: { ...pos, ...fallback } }
        : (Object.fromEntries(
            Object.entries(options).filter(([k]) => k !== "position"),
          ) as typeof options);
    }
  }

  if (pos && "referencePanel" in pos) {
    const refPanel = api.getPanel(pos.referencePanel as string);
    if (!refPanel) {
      const fallback = fallbackGroupPosition(api);
      options = fallback
        ? { ...options, position: fallback }
        : (Object.fromEntries(
            Object.entries(options).filter(([k]) => k !== "position"),
          ) as typeof options);
    }
  }

  // For quiet adds use dockview's `inactive` flag so the new panel is never
  // briefly activated. The save-active / restore-active dance flips the new
  // panel's `isActive` to true and then back, which fires spurious
  // onDidActiveChange events on listeners (e.g. PlanTab's seen-mark).
  if (quiet) {
    api.addPanel({ ...options, inactive: true });
  } else {
    api.addPanel(options);
  }
}
