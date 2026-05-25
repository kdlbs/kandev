/**
 * Env switch logic for dockview layout management.
 *
 * Layouts are keyed by `taskEnvironmentId`. Sessions sharing an env reuse the
 * same layout, so switching between same-env sessions is a no-op at the
 * layout level (handled by the caller). Cross-env switches use either a
 * "fast path" (skip fromJSON when the structure already matches) or a
 * "slow path" (full layout rebuild via fromJSON).
 */
import type { DockviewApi, SerializedDockview } from "dockview-react";
import { getEnvLayout } from "@/lib/local-storage";
import { applyLayoutFixups } from "./dockview-layout-builders";
import { isLayoutShapeHealthy } from "./dockview-layout-health";
import {
  fromDockviewApi,
  savedLayoutMatchesLive,
  layoutStructuresMatch,
  getPinnedWidth,
  getRootSplitview,
  setPinnedTarget,
} from "./layout-manager";
import type { LayoutState, LayoutGroupIds } from "./layout-manager";
import { ENV_SCOPED_DOCKVIEW_COMPONENTS } from "./dockview-env-scoped-components";
import { createDebugLogger, IS_DEBUG } from "@/lib/debug/log";

const debug = createDebugLogger("dockview:env-switch");

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function snapshotGridShape(node: any, depth = 0): unknown {
  if (!node || depth > 6) return null;
  if (node.type === "leaf") {
    return {
      type: "leaf",
      groupId: node.data?.id,
      activeView: node.data?.activeView,
      views: node.data?.views,
    };
  }
  if (node.type === "branch" && Array.isArray(node.data)) {
    return {
      type: "branch",
      orientation: node.orientation,
      children: node.data.map((c: unknown) => snapshotGridShape(c, depth + 1)),
    };
  }
  return null;
}

const EPHEMERAL_COMPONENTS = ENV_SCOPED_DOCKVIEW_COMPONENTS;

/** Fetch the saved layout for an env, dropping it if its shape is corrupted. */
function getHealthyEnvLayout(envId: string): object | null {
  const saved = getEnvLayout(envId);
  if (!saved) return null;
  return isLayoutShapeHealthy(saved) ? saved : null;
}

/** Check whether a serialized dockview layout contains ephemeral panels. */
function savedLayoutHasEphemeralPanels(serialized: SerializedDockview): boolean {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const panels = (serialized as any).panels as
    | Record<string, { contentComponent?: string }>
    | undefined;
  if (!panels) return false;
  return Object.values(panels).some((p) => EPHEMERAL_COMPONENTS.has(p.contentComponent ?? ""));
}

/** Walk the serialized grid tree collecting (groupId, activeView) for each leaf. */
function collectSavedActiveViews(
  saved: SerializedDockview,
): Array<{ groupId: string; activeView: string }> {
  const out: Array<{ groupId: string; activeView: string }> = [];
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const walk = (node: any): void => {
    if (!node) return;
    if (Array.isArray(node.data)) {
      for (const child of node.data) walk(child);
      return;
    }
    const data = node.data;
    if (data?.id && data.activeView) out.push({ groupId: data.id, activeView: data.activeView });
  };
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  walk((saved as any).grid?.root);
  return out;
}

/**
 * Restore each group's `activeView` from the saved layout. The fast path
 * doesn't call `fromJSON`, so per-group active tabs would otherwise carry
 * over from the outgoing env (e.g. Task B left "changes" focused in the
 * right group, and switching back to Task A would still show "changes"
 * even though Task A had "plan" active when it was last saved).
 *
 * The saved `activeGroup` is applied last so the resulting global focus
 * matches what was persisted.
 */
function restoreSavedActiveViews(api: DockviewApi, saved: SerializedDockview): void {
  const entries = collectSavedActiveViews(saved);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const savedActiveGroup = (saved as any).activeGroup as string | undefined;
  const ordered = savedActiveGroup
    ? [
        ...entries.filter((e) => e.groupId !== savedActiveGroup),
        ...entries.filter((e) => e.groupId === savedActiveGroup),
      ]
    : entries;
  for (const { groupId, activeView } of ordered) {
    const group = api.groups.find((g) => g.id === groupId);
    if (!group) continue;
    const panel = group.panels.find((p) => p.id === activeView);
    if (panel) {
      try {
        panel.api.setActive();
      } catch {
        /* panel may be in a transient state */
      }
    }
  }
}

export type EnvSwitchParams = {
  api: DockviewApi;
  oldEnvId: string | null;
  newEnvId: string;
  /** Active session for the incoming env — used to keep the right session chat tab. */
  activeSessionId: string | null;
  safeWidth: number;
  safeHeight: number;
  buildDefault: (api: DockviewApi) => void;
  getDefaultLayout: () => LayoutState;
};

/**
 * Predicate matching panels that `removeEphemeralPanels` will close.
 *
 * Ephemeral panels (file-editors, diffs, commit-details, etc.) are env-scoped
 * and never carry across switches. When `keepSessionId` is provided, chat
 * panels for any other session are also removed so the old env's session tab
 * doesn't bleed into the new env. Pulled out so `computeSurvivingIndex` can
 * reuse the same survival rules without duplicating them.
 */
function shouldRemoveDuringSwitch(
  panel: { id: string; api: { component: string } },
  keepSessionId: string | null,
): boolean {
  const comp = panel.api.component;
  if (EPHEMERAL_COMPONENTS.has(comp)) return true;
  if (
    keepSessionId !== null &&
    comp === "chat" &&
    panel.id.startsWith("session:") &&
    panel.id !== `session:${keepSessionId}`
  ) {
    return true;
  }
  return false;
}

/**
 * Close every panel that matches `shouldRemoveDuringSwitch`. Used to clear
 * env-scoped ephemerals before the new env's panels are restored.
 */
function removeEphemeralPanels(api: DockviewApi, keepSessionId: string | null): void {
  const toRemove = api.panels.filter((p) => shouldRemoveDuringSwitch(p, keepSessionId));
  for (const p of toRemove) {
    try {
      p.api.close();
    } catch {
      /* panel may already be gone */
    }
  }
}

/**
 * Replace stale session chat panels with the incoming active session.
 *
 * A "stale" session is any `session:*` panel whose id isn't
 * `session:${keepSessionId}` — typically a phantom carried in from a saved
 * layout whose session belongs to a different env (or has been deleted).
 *
 * Before closing the first stale panel, add the active session at the same
 * (group, tab-index). This preserves the user's grouping when the stale was
 * co-tabbed with non-session siblings (pr-detail, dragged file-editors,
 * etc.) — without it, the siblings would be orphaned in a group with no
 * session, and `useAutoSessionTab` would later add the active session as a
 * fresh split next to the sidebar.
 *
 * File-editors/diffs/browser/etc. are NEVER touched here — they
 * legitimately belong to this env's saved state.
 */
function replaceStaleSessionPanels(api: DockviewApi, keepSessionId: string | null): void {
  const keepId = keepSessionId ? `session:${keepSessionId}` : null;
  // keepId=null (sessionless task) → strips all session panels, unlike the
  // fast path's shouldRemoveDuringSwitch which keeps them. In practice
  // sessionless tasks should have no session panels; useAutoSessionTab
  // re-adds the panel when a session arrives.
  const stale = api.panels.filter(
    (p) => p.api.component === "chat" && p.id.startsWith("session:") && p.id !== keepId,
  );

  // Anchor the active session to the first stale's (group, index) so co-tabbed
  // siblings (pr-detail etc.) stay grouped with the agent tab. Skipped when:
  //   - no keepSessionId (sessionless task)
  //   - the active session panel already exists in the layout
  //   - the stale's group is missing from the live api (defensive)
  //
  // Limitation: if the saved layout had stale sessions in multiple groups
  // (rare — requires multi-session contamination across env boundaries),
  // only the first stale's group keeps its siblings. Sessions in other
  // groups still close, orphaning anything co-tabbed with them. One active
  // session can only re-anchor one group.
  if (keepSessionId && !api.getPanel(`session:${keepSessionId}`) && stale.length > 0) {
    const first = stale[0];
    const groupId = first.group.id;
    if (api.groups.some((g) => g.id === groupId)) {
      const idx = first.group.panels.findIndex((p) => p.id === first.id);
      addIncomingSessionPanel(api, keepSessionId, groupId, idx);
    }
  }

  if (IS_DEBUG) {
    debug("replaceStaleSessionPanels", {
      keepSessionId,
      livePanelIds: api.panels.map((p) => p.id),
      removingIds: stale.map((p) => p.id),
    });
  }
  for (const p of stale) {
    try {
      p.api.close();
    } catch {
      /* panel may already be gone */
    }
  }
}

/**
 * Given the panels of a group and the id of the panel being replaced, return
 * the target tab index for the replacement among the siblings that will
 * survive `removeEphemeralPanels`. Returns -1 if the panel isn't in the group.
 */
function computeSurvivingIndex(
  groupPanels: readonly { id: string; api: { component: string } }[],
  outgoingPanelId: string | undefined,
  keepSessionId: string | null,
): number {
  if (!outgoingPanelId) return -1;
  const idx = groupPanels.findIndex((p) => p.id === outgoingPanelId);
  if (idx < 0) return -1;
  let count = 0;
  for (let i = 0; i < idx; i++) {
    if (!shouldRemoveDuringSwitch(groupPanels[i], keepSessionId)) count++;
  }
  return count;
}

/**
 * Fast path: check if we can skip `api.fromJSON()` because the layout
 * structure hasn't changed. Returns group IDs if the fast path was taken,
 * or null if a full rebuild is needed.
 */
function tryFastEnvSwitch(params: EnvSwitchParams): LayoutGroupIds | null {
  const { api, newEnvId, activeSessionId, getDefaultLayout } = params;
  const currentLayout = fromDockviewApi(api);
  const saved = getHealthyEnvLayout(newEnvId);

  let structuresMatch = false;
  if (saved) {
    structuresMatch = savedLayoutMatchesLive(currentLayout, saved as SerializedDockview);
  } else {
    structuresMatch = layoutStructuresMatch(currentLayout, getDefaultLayout());
  }

  if (!structuresMatch) {
    if (IS_DEBUG) {
      debug("tryFastEnvSwitch: structures do not match, falling back to slow path", {
        newEnvId,
        hasSaved: !!saved,
        currentPanelIds: api.panels.map((p) => p.id),
      });
    }
    return null;
  }
  if (saved && savedLayoutHasEphemeralPanels(saved as SerializedDockview)) {
    debug("tryFastEnvSwitch: saved layout has ephemeral panels, falling back to slow path", {
      newEnvId,
    });
    return null;
  }
  if (IS_DEBUG) {
    debug("tryFastEnvSwitch: taking fast path", {
      newEnvId,
      activeSessionId,
      hasSaved: !!saved,
      currentPanelIds: api.panels.map((p) => p.id),
    });
  }

  // Prefer the active session panel so multi-session tasks anchor the
  // incoming panel to the group the user was looking at, not whichever
  // session tab happens to come first in `api.panels` iteration order.
  const isSessionPanel = (p: (typeof api.panels)[number]) =>
    p.id.startsWith("session:") || p.api.component === "chat";
  const outgoingSessionPanel =
    api.panels.find((p) => isSessionPanel(p) && p.api.isActive) ?? api.panels.find(isSessionPanel);
  const outgoingGroup = outgoingSessionPanel?.group;
  const outgoingGroupId = outgoingGroup?.id;
  // Capture the session's index among siblings that will survive
  // `removeEphemeralPanels`, so the new session panel lands in the same tab
  // slot. Without this, dockview appends and the agent tab drifts to the end
  // of the group on every cross-task fast-path switch.
  const outgoingIndex = outgoingGroup
    ? computeSurvivingIndex(outgoingGroup.panels, outgoingSessionPanel?.id, activeSessionId)
    : -1;

  removeEphemeralPanels(api, activeSessionId);
  if (activeSessionId && !api.getPanel(`session:${activeSessionId}`)) {
    addIncomingSessionPanel(api, activeSessionId, outgoingGroupId, outgoingIndex);
  }

  // The fast path skips `fromJSON`, so per-group active tabs from the
  // outgoing env would otherwise persist into the incoming env. Reapply
  // them from the saved layout to match what `fromJSON` would have done.
  if (saved) restoreSavedActiveViews(api, saved as SerializedDockview);

  // Column widths from the outgoing env stay live across the switch because
  // we skipped fromJSON. Apply the target env's widths explicitly:
  //   - saved layout exists → use its serialized sizes
  //   - no saved layout (brand-new env) → compute fresh defaults via
  //     getPinnedWidth (ratio-based, clamped to legacy initial cap)
  applyPinnedColumnSizes(api, saved as SerializedDockview | null, params.safeWidth);

  api.layout(params.safeWidth, params.safeHeight);
  return applyLayoutFixups(api);
}

/** Extract per-column sizes from a saved SerializedDockview grid root. */
function extractSavedColumnSizes(saved: SerializedDockview): number[] | null {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const root = (saved as any).grid?.root;
  if (!root?.data || !Array.isArray(root.data)) return null;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return root.data.map((child: any) => (typeof child?.size === "number" ? child.size : NaN));
}

/** Compute the target width for a pinned column from saved sizes or fall
 *  back to the preset's ratio-based default. */
function targetPinnedWidth(
  col: LayoutState["columns"][number],
  index: number,
  savedSizes: number[] | null,
  totalWidth: number,
): number | undefined {
  if (savedSizes && Number.isFinite(savedSizes[index])) return savedSizes[index];
  return getPinnedWidth(col, totalWidth, undefined);
}

/**
 * After a fast-path env switch, override the inherited column widths with
 * the target env's values. Without this, the outgoing env's user-resized
 * widths bleed into the new env — a brand-new task would open at whatever
 * width the user last dragged the previous task's sidebar/right to.
 */
function applyPinnedColumnSizes(
  api: DockviewApi,
  saved: SerializedDockview | null,
  totalWidth: number,
): void {
  const sv = getRootSplitview(api);
  if (!sv || sv.length < 2) return;

  const savedSizes = saved ? extractSavedColumnSizes(saved) : null;
  const liveLayout = fromDockviewApi(api);
  for (let i = 0; i < liveLayout.columns.length && i < sv.length; i++) {
    const col = liveLayout.columns[i];
    if (col.id !== "sidebar" && col.id !== "right") continue;
    const target = targetPinnedWidth(col, i, savedSizes, totalWidth);
    if (typeof target !== "number" || target <= 0) continue;
    try {
      sv.resizeView(i, target);
      // Update the pinned-target so enforcement keeps the new env's width
      // through subsequent rebalances.
      setPinnedTarget(col.id, target);
    } catch {
      /* dockview rejects out-of-range sizes — ignore */
    }
  }
}

/**
 * Add the incoming task's session chat panel, restoring it to the same tab
 * slot the outgoing session occupied within `outgoingGroupId` when possible.
 */
function addIncomingSessionPanel(
  api: DockviewApi,
  sessionId: string,
  outgoingGroupId: string | undefined,
  outgoingIndex: number,
): void {
  let position: import("dockview-react").AddPanelOptions["position"];
  if (outgoingGroupId && api.groups.some((g) => g.id === outgoingGroupId)) {
    position =
      outgoingIndex >= 0
        ? { referenceGroup: outgoingGroupId, index: outgoingIndex }
        : { referenceGroup: outgoingGroupId };
  } else if (api.getPanel("sidebar")) {
    position = { direction: "right" as const, referencePanel: "sidebar" };
  }
  api.addPanel({
    id: `session:${sessionId}`,
    component: "chat",
    tabComponent: "sessionTab",
    title: "Agent",
    params: { sessionId },
    position,
  });
}

/**
 * Switch the dockview layout between task environments.
 *
 * Uses a fast path when layouts are structurally identical (common case),
 * falling back to a full `api.fromJSON()` rebuild when they differ.
 *
 * The caller is responsible for saving the old env's layout and releasing
 * env-scoped portals before calling this function.
 */
export function performEnvSwitch(params: EnvSwitchParams): LayoutGroupIds {
  const { api, oldEnvId, newEnvId, activeSessionId, safeWidth, safeHeight, buildDefault } = params;
  if (IS_DEBUG) {
    debug("performEnvSwitch: entry", {
      oldEnvId,
      newEnvId,
      activeSessionId,
      livePanelIdsBefore: api.panels.map((p) => p.id),
    });
  }

  const fastResult = tryFastEnvSwitch(params);
  if (fastResult) {
    if (IS_DEBUG) {
      debug("performEnvSwitch: completed via fast path", {
        newEnvId,
        livePanelIdsAfter: api.panels.map((p) => p.id),
      });
    }
    return fastResult;
  }

  const saved = getHealthyEnvLayout(newEnvId);
  if (saved) {
    try {
      if (IS_DEBUG) {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const savedPanelIds = Object.keys((saved as any).panels ?? {});
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const savedShape = snapshotGridShape((saved as any).grid?.root);
        debug("performEnvSwitch: slow path - calling api.fromJSON", {
          newEnvId,
          savedPanelIds,
          savedShape,
        });
      }
      api.fromJSON(saved as SerializedDockview);
      // Saved layout may carry a stale session panel from a previously-deleted
      // task (phantom). Replace stale session panels with the incoming active
      // session in the same (group, tab-index), then close the stale ones —
      // preserves grouping with co-tabbed siblings (pr-detail, dragged file
      // editors, etc.). File editors/diffs/etc. on their own are legitimately
      // part of this env's saved state and must NOT be touched.
      // useAutoSessionTab will still no-op if the panel was just added here.
      replaceStaleSessionPanels(api, activeSessionId);
      api.layout(safeWidth, safeHeight);
      if (IS_DEBUG) {
        debug("performEnvSwitch: completed via slow path (fromJSON)", {
          newEnvId,
          livePanelIdsAfter: api.panels.map((p) => p.id),
        });
      }
      return applyLayoutFixups(api);
    } catch (err) {
      console.warn("performEnvSwitch: fromJSON threw", err);
      debug("performEnvSwitch: fromJSON threw, falling through to default", { newEnvId, err });
      /* fall through to default layout build */
    }
  }
  debug("performEnvSwitch: building default layout", { newEnvId, hasSaved: !!saved });
  buildDefault(api);
  api.layout(safeWidth, safeHeight);
  if (IS_DEBUG) {
    debug("performEnvSwitch: completed via default build", {
      newEnvId,
      livePanelIdsAfter: api.panels.map((p) => p.id),
    });
  }
  return applyLayoutFixups(api);
}
