import type { AppState } from "@/lib/state/store";
import type { WorkspaceState } from "./types";

export type WorkspaceItem = WorkspaceState["items"][number];

/**
 * The fields an Office-vs-kanban decision needs, and nothing more.
 *
 * Structural rather than `WorkspaceItem` because callers hold workspace-shaped
 * records from several sources — store items, API list responses, test
 * fixtures — and every one of them can answer this question without first
 * being widened to the full record.
 */
export type ModeWorkspace = { id: string; office_workflow_id?: string | null };

/**
 * True when the workspace is an Office workspace.
 *
 * The single definition of that question — inline
 * `!!workspace.office_workflow_id` copies are how surfaces come to disagree
 * about which workspaces count.
 */
export function isOfficeWorkspace(workspace: ModeWorkspace | null | undefined): boolean {
  return Boolean(workspace?.office_workflow_id);
}

/**
 * The active workspace record, or `undefined` while the workspace list is
 * still unhydrated.
 *
 * Every boot passes through that unhydrated state, so a consumer deriving UI
 * from the workspace cannot read `undefined` as "not an Office workspace" —
 * it has to hold until the list arrives. Returning `undefined` rather than a
 * fabricated default is what keeps that distinction available to callers.
 */
export function selectActiveWorkspace(state: AppState): WorkspaceItem | undefined {
  return state.workspaces.items.find((workspace) => workspace.id === state.workspaces.activeId);
}

/**
 * The workspace record matching `workspaceId`, independent of which
 * workspace is globally active.
 *
 * A route that carries its own workspace id in its URL (like the coordinator
 * screens) must derive workspace-scoped data from that id, not from
 * `activeId` — the two can disagree, e.g. a background WS event switching
 * the active workspace while the user still has an old workspace's route open.
 */
export function selectWorkspaceById(
  workspaceId: string,
): (state: AppState) => WorkspaceItem | undefined {
  return (state) => state.workspaces.items.find((workspace) => workspace.id === workspaceId);
}
