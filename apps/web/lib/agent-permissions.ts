import type { AgentProfile, PermissionSetting } from "@/lib/types/http";

/**
 * Permission keys retained on the frontend. After the ACP-first migration the
 * only surviving CLI-flag-driven permission is auggie's `allow_indexing` — all
 * other agents express permission stance through ACP session modes (rendered
 * as a separate Mode picker) and the interactive permission_request message UI.
 *
 * This file is kept as a thin compatibility shim so call sites that still use
 * the old permission-map helpers continue to compile. Non-auggie agents will
 * simply have an empty permission_settings map from the backend, causing the
 * UI to render no toggles at all.
 */
export const PERMISSION_KEYS = ["allow_indexing"] as const;

export type PermissionKey = (typeof PERMISSION_KEYS)[number];

// Compile-time check: every PermissionKey must be a boolean key on AgentProfile.
// eslint-disable-next-line @typescript-eslint/no-unused-vars
type _AssertKeysExist = {
  [K in PermissionKey]: AgentProfile[K] extends boolean ? true : never;
};

/** Extract permission booleans from a profile-like object, using backend defaults for missing values. */
export function profileToPermissionsMap(
  profile: Partial<Pick<AgentProfile, PermissionKey>>,
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
): Pick<AgentProfile, PermissionKey> {
  const result = {} as Pick<AgentProfile, PermissionKey>;
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

/** Compare permission fields between two profile-like objects. */
export function arePermissionsDirty(
  draft: Partial<Pick<AgentProfile, PermissionKey>>,
  saved: Partial<Pick<AgentProfile, PermissionKey>>,
): boolean {
  for (const key of PERMISSION_KEYS) {
    if (draft[key] !== saved[key]) return true;
  }
  return false;
}
