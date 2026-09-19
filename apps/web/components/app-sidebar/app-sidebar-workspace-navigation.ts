import {
  ACTIVE_WORKSPACE_COOKIE,
  LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE,
  scopedCookieName,
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
  // The office boot path (`src/office-routes.tsx`) reads the office-family
  // cookie (scoped name first, legacy unprefixed name as read-only fallback)
  // to pick an office workspace when the unified one names a kanban board, so
  // it is kept in step here. Only the port-scoped names are ever written.
  if (kind === "office") {
    writeWorkspaceCookie(LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE, id);
  }
}

/**
 * Writes one workspace-selection cookie under its port-scoped name (API-origin
 * port; the plain name on a no-port instance). The legacy unprefixed name is
 * deliberately left untouched: on a host serving a default-port instance it is
 * that instance's LIVE selection cookie, and for upgraded ported instances it
 * is the validated migration fallback until each writes its own scoped value.
 * Scrubbing it from one instance would change the other instances' next boot
 * (see docs/specs/auth/requirements/fix-multi-instance-cookie-isolation.md: the upgraded
 * instance does not proactively delete the legacy cookie).
 */
function writeWorkspaceCookie(name: string, value: string): void {
  if (typeof document === "undefined") return;
  document.cookie = `${scopedCookieName(name)}=${encodeURIComponent(value)}; path=/; max-age=${ACTIVE_WORKSPACE_COOKIE_MAX_AGE}; samesite=strict`;
}
