/**
 * Session switch logic for dockview layout management.
 *
 * Handles both a "fast path" (skip fromJSON when layout structure matches)
 * and a "slow path" (full layout rebuild via fromJSON).
 */
import type { DockviewApi, SerializedDockview } from "dockview-react";
import { getSessionLayout } from "@/lib/local-storage";
import { applyLayoutFixups } from "./dockview-layout-builders";
import { fromDockviewApi, savedLayoutMatchesLive, layoutStructuresMatch } from "./layout-manager";
import type { LayoutState, LayoutGroupIds } from "./layout-manager";

const EPHEMERAL_COMPONENTS = new Set(["file-editor", "diff-viewer", "commit-detail"]);

/** Check whether a serialized dockview layout contains ephemeral panels. */
function savedLayoutHasEphemeralPanels(serialized: SerializedDockview): boolean {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const panels = (serialized as any).panels as
    | Record<string, { contentComponent?: string }>
    | undefined;
  if (!panels) return false;
  return Object.values(panels).some((p) => EPHEMERAL_COMPONENTS.has(p.contentComponent ?? ""));
}

export type SessionSwitchParams = {
  api: DockviewApi;
  oldSessionId: string | null;
  newSessionId: string;
  safeWidth: number;
  safeHeight: number;
  buildDefault: (api: DockviewApi) => void;
  getDefaultLayout: () => LayoutState;
};

/**
 * Remove ephemeral panels (file-editors, diffs, commit-details) from the
 * live layout. These are session-specific panels that shouldn't carry over.
 *
 * When `keepSessionId` is provided, session chat panels whose ID does not
 * match `session:{keepSessionId}` are also removed. This handles cross-task
 * switches where the fast path is taken: without this, session tabs from the
 * old task remain visible alongside the new task's tab.
 */
function removeEphemeralPanels(api: DockviewApi, keepSessionId?: string): void {
  const toRemove = api.panels.filter((p) => {
    const comp = p.api.component;
    if (comp === "file-editor" || comp === "diff-viewer" || comp === "commit-detail") {
      return true;
    }
    // Remove session chat tabs that belong to a different session.
    // This covers cross-task switches where the layout structures match and
    // the fast path is taken — old task's session tabs must not bleed through.
    if (
      keepSessionId !== undefined &&
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
function tryFastSessionSwitch(params: SessionSwitchParams): LayoutGroupIds | null {
  const { api, newSessionId, getDefaultLayout } = params;
  const currentLayout = fromDockviewApi(api);
  const saved = getSessionLayout(newSessionId);

  let structuresMatch = false;
  if (saved) {
    // Compare live layout against saved layout by structural component set
    structuresMatch = savedLayoutMatchesLive(currentLayout, saved as SerializedDockview);
  } else {
    // No saved layout — target is the default; compare LayoutState structures
    structuresMatch = layoutStructuresMatch(currentLayout, getDefaultLayout());
  }

  if (!structuresMatch) return null;

  // If the saved layout contains ephemeral panels (file-editors, diffs,
  // commit-details), fall through to the slow path so that `api.fromJSON()`
  // restores them.  The fast path skips fromJSON and removeEphemeralPanels
  // would discard the current ones without restoring the target session's.
  if (saved && savedLayoutHasEphemeralPanels(saved as SerializedDockview)) return null;

  // Fast path: keep the grid structure, clean up ephemeral panels and any
  // session chat tabs that belong to a different session (cross-task switch).
  // Session-scoped portals (browser, vscode, etc.) will be re-acquired
  // via usePortalSlot's sessionId dependency change.
  removeEphemeralPanels(api, newSessionId);

  // Create the new session tab inline so there's no gap between removing the
  // old tab and creating the new one. useAutoSessionTab will detect the panel
  // already exists and skip creation.
  if (!api.getPanel(`session:${newSessionId}`)) {
    const chatPanel = api.panels.find(
      (p) => p.id.startsWith("session:") || p.api.component === "chat",
    );
    const sidebarPanel = api.getPanel("sidebar");
    let position: import("dockview-react").AddPanelOptions["position"];
    if (chatPanel) {
      position = { referenceGroup: chatPanel.group.id };
    } else if (sidebarPanel) {
      position = { direction: "right" as const, referencePanel: "sidebar" };
    }
    api.addPanel({
      id: `session:${newSessionId}`,
      component: "chat",
      tabComponent: "sessionTab",
      title: "Agent",
      params: { sessionId: newSessionId },
      position,
    });
  }

  api.layout(params.safeWidth, params.safeHeight);
  return applyLayoutFixups(api);
}

/**
 * Switch the dockview layout between sessions.
 *
 * Uses a fast path when layouts are structurally identical (common case),
 * falling back to a full `api.fromJSON()` rebuild when they differ.
 *
 * The caller is responsible for saving the old session layout and releasing
 * session-scoped portals before calling this function.
 */
export function performSessionSwitch(params: SessionSwitchParams): LayoutGroupIds {
  const { api, newSessionId, safeWidth, safeHeight, buildDefault } = params;

  // Try fast path: skip fromJSON when layout structure hasn't changed
  const fastResult = tryFastSessionSwitch(params);
  if (fastResult) {
    return fastResult;
  }

  // Slow path: full layout rebuild via fromJSON
  const saved = getSessionLayout(newSessionId);
  if (saved) {
    try {
      api.fromJSON(saved as SerializedDockview);
      api.layout(safeWidth, safeHeight);
      return applyLayoutFixups(api);
    } catch {
      /* fall through */
    }
  }
  buildDefault(api);
  api.layout(safeWidth, safeHeight);
  return applyLayoutFixups(api);
}
