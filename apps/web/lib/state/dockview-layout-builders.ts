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
  getPinnedWidth,
  getRootSplitview as getRootSplitviewImpl,
  resolveGroupIds,
  setPinnedTarget,
} from "./layout-manager";
import type { LayoutGroupIds } from "./layout-manager";
import { getGlobalSidebarWidth } from "@/lib/local-storage";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

// Re-export for consumers that import from this module
export { getRootSplitview } from "./layout-manager";

const debugWidths = createDebugLogger("dockview:widths");

/** Dockview's measured grid width, or undefined when not yet laid out — passed
 *  to the cap helpers so they don't fall back to a possibly-stale
 *  `window.innerWidth`. */
function layoutWidth(api: DockviewApi): number | undefined {
  return api.width > 0 ? api.width : undefined;
}

/** Best-effort caller chain for the fixups-capture debug log: pull the first
 *  few stack frames above `applyLayoutFixups` so we can see WHICH layout path
 *  (env-switch / restore / custom-layout / maximize) recorded a given target.
 *  Debug-only — never called when isDebug() is false. */
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
export function applyLayoutFixups(api: DockviewApi, savedRightWidth?: number): LayoutGroupIds {
  const sv = getRootSplitviewImpl(api);
  captureSidebarTarget(api, sv);

  const oldChanges = api.getPanel("diff-files");
  if (oldChanges) oldChanges.api.setTitle("Changes");
  const oldFiles = api.getPanel("all-files");
  if (oldFiles) oldFiles.api.setTitle("Files");

  captureRightTarget(api, sv, savedRightWidth);

  logFixupsCapture(api, sv);

  return resolveGroupIds(api);
}

/** Resolve the sidebar's target width (clamped to the cap): the GLOBAL width
 *  pref when set, else the default width for the current measured layout. */
function resolveSidebarTarget(cap: number, totalWidth: number | undefined): number | undefined {
  const pref = getGlobalSidebarWidth();
  if (pref !== null) return Math.min(pref, cap);
  const width =
    totalWidth ??
    (typeof window !== "undefined" && window.innerWidth > 0 ? window.innerWidth : cap);
  return Math.min(
    getPinnedWidth({ id: "sidebar", pinned: true, groups: [] }, width, undefined),
    cap,
  );
}

/** Lock + constrain the sidebar group and record its target width, clamped to
 *  the cap. The constraint pins the column at `sidebarCap`, so a target above
 *  it is unreachable and makes `enforcePinnedTargets` spin forever. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function captureSidebarTarget(api: DockviewApi, sv: any): void {
  const sb = api.getPanel("sidebar");
  if (!sb) return;
  // Derive the cap from dockview's measured grid width, not the implicit
  // window.innerWidth fallback: innerWidth can read transiently stale during
  // route transitions / devtools toggles, yielding a too-small cap that would
  // clamp the captured target too narrow and persist it — the width drift this
  // pipeline exists to prevent.
  const measuredWidth = layoutWidth(api);
  const sidebarCap = computeSidebarMaxPx(measuredWidth);
  sb.group.locked = SIDEBAR_LOCK;
  sb.group.header.hidden = false;
  sb.group.api.setConstraints({ maximumWidth: sidebarCap, minimumWidth: LAYOUT_PINNED_MIN_PX });
  // Slow-path env restore: fromJSON brought back this env's saved sidebar
  // pixel width, but the sidebar is a GLOBAL pref. Seed the target from the
  // pref (clamped to fit) and resize the column so the restore honors it; fall
  // back to the default sidebar width when no pref exists.
  const target = resolveSidebarTarget(sidebarCap, measuredWidth);
  if (target !== undefined) {
    const cur = sv?.getViewSize?.(0);
    if (typeof cur === "number" && cur > 0 && Math.abs(cur - target) > 1) {
      try {
        sv?.resizeView?.(0, target);
      } catch {
        /* dockview rejects unreachable sizes — ignore */
      }
    }
    setPinnedTarget("sidebar", target);
  }
}

/** Resolve the pinned right column's target width (clamped to the cap): the
 *  per-env SAVED width when the restore supplies one, else the column default
 *  for the current measured layout. Never the live splitview size — see
 *  `captureRightTarget`. */
function resolveRightTarget(
  cap: number,
  totalWidth: number | undefined,
  savedRightWidth: number | undefined,
): number {
  if (savedRightWidth !== undefined && savedRightWidth > 0) return Math.min(savedRightWidth, cap);
  const width =
    totalWidth ??
    (typeof window !== "undefined" && window.innerWidth > 0 ? window.innerWidth : cap);
  return Math.min(getPinnedWidth({ id: "right", pinned: true, groups: [] }, width, undefined), cap);
}

/** Constrain the default layout's right column groups and record the side
 *  column's target width, clamped to the cap.
 *
 *  For the DEFAULT preset (the right column is pinned — identified by the
 *  well-known RIGHT_TOP_GROUP), the target is anchored to a STABLE width: the
 *  per-env saved width passed by the restore, or the column default when none.
 *  It is deliberately NOT read from the live splitview: dockview's
 *  post-`fromJSON` proportional rebalance reports a transient/rescaled size,
 *  and persisting that as the target ratcheted the right column wider on every
 *  restore (the dockview-wrong-width drift). The sidebar is handled the same
 *  way in `captureSidebarTarget` — neither pinned column captures its live size.
 *
 *  For the vscode/preview/plan presets the side column has a generated group id
 *  and is NOT pinned, so there is no saved/default width to fall back on; there
 *  we keep recording the just-restored live width so switching to one of those
 *  tasks doesn't leak the previous task's right target.
 *
 *  `sv.length >= 3` excludes the 2-column global-fallback restore (sidebar +
 *  center, right panels stripped as env-scoped), where the last splitview child
 *  is the CENTER column — recording its width as the "right" target would
 *  inflate the real right column once the full layout materializes. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function captureRightTarget(api: DockviewApi, sv: any, savedRightWidth?: number): void {
  // Constrain the default preset's right column groups (stable well-known IDs).
  // Other presets' side columns aren't pinned and carry no max-width cap.
  // Use the measured grid width, not the window.innerWidth fallback (see
  // captureSidebarTarget).
  const measuredWidth = layoutWidth(api);
  const rightCap = computeRightMaxPx(measuredWidth);
  for (const gid of [RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP]) {
    const group = api.groups.find((g) => g.id === gid);
    if (group) {
      group.api.setConstraints({ maximumWidth: rightCap, minimumWidth: LAYOUT_PINNED_MIN_PX });
    }
  }
  if (!sv || sv.length < 3) return;
  const idx = sv.length - 1;
  // Default preset (pinned right column): anchor to saved-or-default, resize the
  // column to it, and record it as the target. Never the live size.
  if (api.groups.some((g) => g.id === RIGHT_TOP_GROUP || g.id === RIGHT_BOTTOM_GROUP)) {
    const target = resolveRightTarget(rightCap, measuredWidth, savedRightWidth);
    const cur = sv.getViewSize(idx);
    if (typeof cur === "number" && cur > 0 && Math.abs(cur - target) > 1) {
      try {
        sv?.resizeView?.(idx, target);
      } catch {
        /* dockview rejects unreachable sizes — ignore */
      }
    }
    setPinnedTarget("right", target);
    return;
  }
  // Non-default presets: side column is not pinned — keep the live width.
  const liveRight = sv.getViewSize(idx);
  if (typeof liveRight === "number" && liveRight > 0) {
    setPinnedTarget("right", Math.min(liveRight, rightCap));
  }
}

/** Emit the `fixups-capture` width snapshot. Pulled out of `applyLayoutFixups`
 *  to keep that function under the complexity limit; no-op in prod. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function logFixupsCapture(api: DockviewApi, sv: any): void {
  if (!isDebug()) return;
  // Decisive fields: `sidebarOverCap=true` means the recorded target exceeds
  // the cap that enforcement clamps the column to — i.e. an unreachable target
  // that makes `enforcePinnedTargets` spin forever. `cols` shows whether the
  // layout was complete at capture (cols<3 → no real right column). api.width
  // vs window.innerWidth surfaces the window-fallback cap divergence.
  const w = layoutWidth(api);
  const sidebarCap = computeSidebarMaxPx(w);
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
      `rightCap=${Math.round(computeRightMaxPx(w))} liveRight=${r(liveRight)} ` +
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
