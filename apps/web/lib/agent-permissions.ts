import type { PermissionSetting } from "@/lib/types/http";

/**
 * Permission keys retained on the frontend. After the ACP-first migration the
 * only surviving CLI-flag-driven permission is auggie's `allow_indexing` — all
 * other agents express permission stance through ACP session modes (rendered
 * as a separate Mode picker) and the interactive permission_request message UI.
 *
 * Keys are kept in **snake_case wire format** because they pass straight
 * through to backend permission-payload bodies and the `permissionSettings`
 * map keyed by snake_case agent metadata.
 */
export const PERMISSION_KEYS = ["allow_indexing"] as const;

export type PermissionKey = (typeof PERMISSION_KEYS)[number];

/** Extract permission booleans from a profile-like (snake_case) object. */
export function profileToPermissionsMap(
  profile: Partial<Record<PermissionKey, boolean>>,
  permissionSettings: Record<string, PermissionSetting>,
): Record<PermissionKey, boolean> {
  const result = {} as Record<PermissionKey, boolean>;
  for (const key of PERMISSION_KEYS) {
    const setting = permissionSettings[key];
    result[key] = profile[key] ?? setting?.default ?? false;
  }
  return result;
}

/** Convert an object containing permission keys to a typed patch for API calls. */
export function permissionsToProfilePatch(
  perms: Partial<Record<PermissionKey, boolean>>,
): Record<PermissionKey, boolean> {
  const result = {} as Record<PermissionKey, boolean>;
  for (const key of PERMISSION_KEYS) {
    result[key] = perms[key] ?? false;
  }
  return result;
}

/** Create default permission values from backend metadata. */
export function buildDefaultPermissions(
  permissionSettings: Record<string, PermissionSetting>,
): Record<PermissionKey, boolean> {
  const result = {} as Record<PermissionKey, boolean>;
  for (const key of PERMISSION_KEYS) {
    result[key] = permissionSettings[key]?.default ?? false;
  }
  return result;
}

/** Compare permission fields between two snake_case permission-bearing objects. */
export function arePermissionsDirty(
  draft: Partial<Record<PermissionKey, boolean>>,
  saved: Partial<Record<PermissionKey, boolean>>,
): boolean {
  for (const key of PERMISSION_KEYS) {
    if (draft[key] !== saved[key]) return true;
  }
  return false;
}
