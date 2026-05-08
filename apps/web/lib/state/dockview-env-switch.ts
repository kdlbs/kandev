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
import { fromDockviewApi, savedLayoutMatchesLive, layoutStructuresMatch } from "./layout-manager";
import type { LayoutState, LayoutGroupIds } from "./layout-manager";

const EPHEMERAL_COMPONENTS = new Set([
  "file-editor",
  "browser",
  "vscode",
  "commit-detail",
  "diff-viewer",
  "pr-detail",
]);

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

  if (!structuresMatch) return null;
  if (saved && savedLayoutHasEphemeralPanels(saved as SerializedDockview)) return null;

  const outgoingSessionPanel = api.panels.find(
    (p) => p.id.startsWith("session:") || p.api.component === "chat",
  );
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

  api.layout(params.safeWidth, params.safeHeight);
  return applyLayoutFixups(api);
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
  const { api, newEnvId, safeWidth, safeHeight, buildDefault } = params;

  const fastResult = tryFastEnvSwitch(params);
  if (fastResult) return fastResult;

  const saved = getHealthyEnvLayout(newEnvId);
  if (saved) {
    try {
      api.fromJSON(saved as SerializedDockview);
      api.layout(safeWidth, safeHeight);
      return applyLayoutFixups(api);
    } catch (err) {
      console.warn("performEnvSwitch: fromJSON threw", err);
      /* fall through to default layout build */
    }
  }
  buildDefault(api);
  api.layout(safeWidth, safeHeight);
  return applyLayoutFixups(api);
}
