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
 * Remove ephemeral panels (file-editors, diffs, commit-details) from the
 * live layout. These are env-scoped panels that shouldn't carry over.
 *
 * When `keepSessionId` is provided, session chat panels whose ID does not
 * match `session:{keepSessionId}` are also removed. This handles cross-env
 * (cross-task) switches where the fast path is taken: without this, session
 * tabs from the old env's task remain visible alongside the new task's tab.
 */
function removeEphemeralPanels(api: DockviewApi, keepSessionId: string | null): void {
  const toRemove = api.panels.filter((p) => {
    const comp = p.api.component;
    if (EPHEMERAL_COMPONENTS.has(comp)) {
      return true;
    }
    if (
      keepSessionId !== null &&
      comp === "chat" &&
      p.id.startsWith("session:") &&
      p.id !== `session:${keepSessionId}`
    ) {
      return true;
    }
    return false;
  });
  for (const p of toRemove) {
    try {
      p.api.close();
    } catch {
      /* panel may already be gone */
    }
  }
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
  const outgoingGroupId = outgoingSessionPanel?.group?.id;

  removeEphemeralPanels(api, activeSessionId);

  if (activeSessionId && !api.getPanel(`session:${activeSessionId}`)) {
    const sidebarPanel = api.getPanel("sidebar");
    let position: import("dockview-react").AddPanelOptions["position"];
    if (outgoingGroupId && api.groups.some((g) => g.id === outgoingGroupId)) {
      position = { referenceGroup: outgoingGroupId };
    } else if (sidebarPanel) {
      position = { direction: "right" as const, referencePanel: "sidebar" };
    }
    api.addPanel({
      id: `session:${activeSessionId}`,
      component: "chat",
      tabComponent: "sessionTab",
      title: "Agent",
      params: { sessionId: activeSessionId },
      position,
    });
  }

  api.layout(params.safeWidth, params.safeHeight);
  return applyLayoutFixups(api);
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
