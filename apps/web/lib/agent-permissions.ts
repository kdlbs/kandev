import type { AgentProfile, PermissionSetting } from "@/lib/types/http";

/**
 * Single source of truth for permission keys.
 * Adding a key here that doesn't exist on AgentProfile will cause a compile error.
 */
export const PERMISSION_KEYS = [
  "auto_approve",
  "dangerously_skip_permissions",
  "allow_indexing",
] as const;

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
