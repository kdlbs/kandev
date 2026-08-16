import {
  ACTIVE_WORKSPACE_COOKIE,
  LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE,
} from "@/lib/routing/route-bootstrap";
import { isOfficeWorkspace, type ModeWorkspace } from "@/lib/state/slices/workspace/selectors";

export { workspaceHomeHref } from "@/lib/navigation/workspace-home";

const ACTIVE_WORKSPACE_COOKIE_MAX_AGE = 31536000;

/**
 * Records a workspace as the active one — one write per workspace change.
 * Callers must not write these cookies themselves: split writes are how the
 * active-workspace record and the legacy office cookie drift out of step.
 */
export function rememberWorkspaceSelection(workspace: ModeWorkspace | undefined): void {
  if (!workspace) return;
  rememberWorkspaceSelectionById(workspace.id, isOfficeWorkspace(workspace) ? "office" : "kanban");
}

/**
 * The same write for a caller that knows the kind but does not hold a workspace
 * record — the setup wizard, whose create response returns an id and nothing
 * else. Passing a fabricated record with an invented `office_workflow_id` would
 * be a lie the type system happily accepts.
 */
export function rememberWorkspaceSelectionById(id: string, kind: "office" | "kanban"): void {
  if (!id || typeof document === "undefined") return;
  writeWorkspaceCookie(ACTIVE_WORKSPACE_COOKIE, id);
  // The office boot paths (`app/office/layout.tsx` and the office route
  // bootstrap) still read the legacy cookie to pick an office workspace when
  // the unified one names a kanban board, so it is kept in step here.
  if (kind === "office") {
    writeWorkspaceCookie(LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE, id);
  }
}

function writeWorkspaceCookie(name: string, value: string): void {
  if (typeof document === "undefined") return;
  document.cookie = `${name}=${encodeURIComponent(value)}; path=/; max-age=${ACTIVE_WORKSPACE_COOKIE_MAX_AGE}; samesite=strict`;
}
