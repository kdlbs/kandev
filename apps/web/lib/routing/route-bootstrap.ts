import { getBackendConfig } from "@/lib/config";
import type { WorkspaceState } from "@/lib/state/slices/workspace/types";
import type { ListWorkspacesResponse } from "@/lib/types/http";

export const ACTIVE_WORKSPACE_COOKIE = "kandev-active-workspace";
export const LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE = "office-active-workspace";

// One year; matches the sidebar writer's selection-cookie lifetime.
const WORKSPACE_COOKIE_MAX_AGE = 31536000;

type WorkspaceItem = ListWorkspacesResponse["workspaces"][number];

/**
 * The API origin port. Browsers ignore port when matching cookies, so multiple
 * kandev instances on one host (same IP, different ports) share one cookie
 * jar; suffixing cookie names with this port isolates each instance's
 * selection. Derived from the API base URL, NOT `window.location.port` — they
 * only match in same-origin deployments (split-origin dev runs the SPA on the
 * web port and the API on the backend port).
 */
export function apiOriginPort(): string {
  if (typeof window === "undefined" && !process.env.KANDEV_API_BASE_URL) {
    // Server-side helper beside the backend with no configured base URL: the
    // dev default port means nothing on the real host, and scoping against it
    // would produce a cookie name no browser ever writes. Unscoped is the
    // fail-closed default; the browser (which has the real origin) is the
    // only live cookie reader.
    return "";
  }
  return new URL(getBackendConfig().apiBaseUrl).port;
}

/**
 * Port-scopes a cookie name: `kandev-active-workspace` + port `8443` →
 * `kandev-active-workspace_8443`. An empty/missing port keeps the plain name,
 * so default-port (no port in the URL) deployments are unchanged.
 */
export function scopedCookieName(name: string, port?: string): string {
  const suffix = port ?? apiOriginPort();
  return suffix ? `${name}_${suffix}` : name;
}

/**
 * Reads the port-scoped cookie first, falling back to the legacy unprefixed
 * name so a pre-upgrade selection survives. The explicit `port` parameter
 * keeps the helper pure and unit-testable without jsdom Location stubbing.
 * An empty cookie value is treated as absent: expired/deleted cookies can
 * linger as empty entries in document.cookie and must not shadow the
 * fallback. On a default-port instance the scoped name IS the legacy name,
 * so the fallback read is skipped (identical call, no-op).
 */
export function readScopedCookie(name: string, port?: string): string | null {
  const scopedName = scopedCookieName(name, port);
  const scoped = readCookie(scopedName);
  if (scoped) return scoped;
  if (scopedName === name) return null;
  const legacy = readCookie(name);
  return legacy || null;
}

export function mapWorkspaceItem(ws: WorkspaceItem): WorkspaceState["items"][number] {
  return {
    id: ws.id,
    name: ws.name,
    description: ws.description ?? null,
    owner_id: ws.owner_id,
    // Team access: visibility plus the caller's resolved role and scopes.
    // These gate every owner-only control, so dropping them here silently
    // renders the workspace as read-only to its own owner.
    unit_id: ws.unit_id,
    viewer_role: ws.viewer_role,
    scopes: ws.scopes,
    member_count: ws.member_count,
    default_executor_id: ws.default_executor_id ?? null,
    default_environment_id: ws.default_environment_id ?? null,
    default_agent_profile_id: ws.default_agent_profile_id ?? null,
    default_config_agent_profile_id: ws.default_config_agent_profile_id ?? null,
    office_workflow_id: ws.office_workflow_id ?? null,
    created_at: ws.created_at,
    updated_at: ws.updated_at,
  };
}

export function readCookie(name: string): string | null {
  if (typeof document === "undefined") return null;
  const encodedName = `${encodeURIComponent(name)}=`;
  const entries = document.cookie
    .split(";")
    .map((part) => part.trim())
    .filter((part) => part.startsWith(encodedName));
  const entry = entries[entries.length - 1];
  return entry ? decodeURIComponent(entry.slice(encodedName.length)) : null;
}

export function readActiveWorkspaceCookie(): string | null {
  return readScopedCookie(ACTIVE_WORKSPACE_COOKIE);
}

/**
 * One-time promotion of a validated legacy selection into the scoped cookie.
 *
 * A ported instance whose scoped cookie was never written (fresh upgrade, or
 * no explicit selection yet on this instance) falls back to the legacy
 * unprefixed name on every boot, keeping it coupled to the shared jar. Once
 * the boot resolves a legacy value that names one of the instance's own
 * workspaces, copy it into the port-scoped name so later boots read the
 * scoped cookie — without touching the legacy name, which is the live
 * selection cookie of any default-port instance on the host and the
 * migration fallback for other instances (spec: no proactive legacy
 * scrubbing). No-op when the scoped cookie already exists, the legacy value
 * is absent or invalid, or the instance has no port (scoped == legacy).
 */
export function promoteLegacyWorkspaceSelection(
  workspaceItems: { id: string }[],
  name: string = ACTIVE_WORKSPACE_COOKIE,
): void {
  if (typeof document === "undefined") return;
  const port = apiOriginPort();
  if (!port) return;
  const scoped = `${name}_${port}`;
  if (readCookie(scoped)) return;
  const legacy = readCookie(name);
  if (!legacy || !workspaceItems.some((ws) => ws.id === legacy)) return;
  document.cookie = `${scoped}=${encodeURIComponent(legacy)}; path=/; max-age=${WORKSPACE_COOKIE_MAX_AGE}; samesite=strict`;
}

export type OfficeWorkspaceItem = { id: string; office_workflow_id?: string | null };

export type OfficeWorkspaceCandidates = {
  routeWorkspaceId?: string | null;
  generalWorkspaceId?: string | null;
  officeWorkspaceId?: string | null;
  settingsWorkspaceId?: string | null;
};

/**
 * Canonical office active-workspace resolver (frontend half of the
 * fix-multi-instance-cookie-isolation task-03 contract). Precedence:
 * explicit route override → general cookie when it names an office workspace
 * → office cookie → user settings → first office workspace. Every candidate is
 * validated against the office workspace set passed in. `routeWorkspaceId` is
 * a client/page-only candidate: server layouts and the backend boot payload
 * are cookie-first by design and pass null.
 */
export function resolveOfficeWorkspaceId(
  workspaceItems: OfficeWorkspaceItem[],
  candidates: OfficeWorkspaceCandidates,
): string | null {
  const { routeWorkspaceId, generalWorkspaceId, officeWorkspaceId, settingsWorkspaceId } =
    candidates;
  return (
    workspaceItems.find((workspace) => workspace.id === routeWorkspaceId)?.id ??
    workspaceItems.find((workspace) => workspace.id === generalWorkspaceId)?.id ??
    workspaceItems.find((workspace) => workspace.id === officeWorkspaceId)?.id ??
    workspaceItems.find((workspace) => workspace.id === settingsWorkspaceId)?.id ??
    workspaceItems[0]?.id ??
    null
  );
}

type SettingsWorkspaceItem = {
  id: string;
  office_workflow_id?: string | null;
};

/**
 * The active workspace for a settings boot: whatever the user last had active,
 * then their stored preference, then the first workspace that exists.
 *
 * Deliberately not filtered to kanban workspaces. It used to prefer them, which
 * was invisible while Office-vs-kanban chrome came from the pathname — Settings
 * is not an `/office` route, so it rendered kanban chrome regardless. Now that
 * the chrome follows the active workspace, that filter would *write* a kanban
 * workspace into the store on every settings visit, so an Office user opening
 * Settings would silently have their workspace switched underneath them.
 *
 * Settings is shared chrome, reachable from either mode. It has no business
 * preferring one.
 */
export function resolveSettingsActiveWorkspaceId(
  workspaceItems: SettingsWorkspaceItem[],
  activeCookieWorkspaceId: string | null,
  settingsWorkspaceId: string | null,
): string | null {
  return (
    workspaceItems.find((workspace) => workspace.id === activeCookieWorkspaceId)?.id ??
    workspaceItems.find((workspace) => workspace.id === settingsWorkspaceId)?.id ??
    workspaceItems[0]?.id ??
    null
  );
}
