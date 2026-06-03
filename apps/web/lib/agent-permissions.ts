import type { PermissionSetting } from "@/lib/types/http";

/**
 * Profile permission keys (snake_case) mirrored from backend PermissionSettings.
 *
 * - `auto_approve`: Kandev agentctl auto-allows ACP permission_request frames
 *   (all agents via CatalogPermissionSettings).
 * - `allow_indexing`: legacy auggie CLI flag still exposed on its profile form.
 */
export const PERMISSION_KEYS = ["auto_approve", "allow_indexing"] as const;

/** apply_method sentinel for the universal agentctl auto-approve toggle. */
export const PERMISSION_APPLY_AGENTCTL_AUTO_APPROVE = "agentctl_auto_approve";

export type PermissionKey = (typeof PERMISSION_KEYS)[number];

/** Profile-shaped input that may carry permissions in snake_case or camelCase. */
export type PermissionProfileInput = Partial<Record<PermissionKey, boolean>> & {
  autoApprove?: boolean;
  allowIndexing?: boolean;
};

const CAMEL_BY_KEY: Record<PermissionKey, "autoApprove" | "allowIndexing"> = {
  auto_approve: "autoApprove",
  allow_indexing: "allowIndexing",
};

/**
 * Read one permission boolean, preferring camelCase (canonical AgentProfile)
 * over snake_case (form wire keys) so toggles never lose to stale defaults.
 */
export function readPermissionValue(
  profile: PermissionProfileInput,
  key: PermissionKey,
  permissionSettings: Record<string, PermissionSetting> = {},
): boolean {
  const camelKey = CAMEL_BY_KEY[key];
  const camelVal = profile[camelKey];
  if (typeof camelVal === "boolean") return camelVal;
  const snakeVal = profile[key];
  if (typeof snakeVal === "boolean") return snakeVal;
  return permissionSettings[key]?.default ?? false;
}

/** Normalized snake_case permission map for forms and API payloads. */
export function profilePermissionValues(
  profile: PermissionProfileInput,
  permissionSettings: Record<string, PermissionSetting> = {},
): Record<PermissionKey, boolean> {
  const result = {} as Record<PermissionKey, boolean>;
  for (const key of PERMISSION_KEYS) {
    result[key] = readPermissionValue(profile, key, permissionSettings);
  }
  return result;
}

/** @deprecated Use profilePermissionValues — kept for call-site compatibility. */
export function profileToPermissionsMap(
  profile: PermissionProfileInput,
  permissionSettings: Record<string, PermissionSetting>,
): Record<PermissionKey, boolean> {
  return profilePermissionValues(profile, permissionSettings);
}

/** Snake_case permission fields for create/update profile API bodies. */
export function permissionsToProfilePatch(
  profile: PermissionProfileInput,
  permissionSettings: Record<string, PermissionSetting> = {},
): Record<PermissionKey, boolean> {
  return profilePermissionValues(profile, permissionSettings);
}

/** Canonical camelCase defaults for new AgentProfile / DraftProfile rows. */
export function buildDefaultPermissions(permissionSettings: Record<string, PermissionSetting>): {
  autoApprove: boolean;
  allowIndexing: boolean;
} {
  const perms = profilePermissionValues({}, permissionSettings);
  return {
    autoApprove: perms.auto_approve,
    allowIndexing: perms.allow_indexing,
  };
}

/** Compare permission fields between two profile-shaped objects. */
export function arePermissionsDirty(
  draft: PermissionProfileInput,
  saved: PermissionProfileInput,
  permissionSettings: Record<string, PermissionSetting> = {},
): boolean {
  const draftPerms = profilePermissionValues(draft, permissionSettings);
  const savedPerms = profilePermissionValues(saved, permissionSettings);
  for (const key of PERMISSION_KEYS) {
    if (draftPerms[key] !== savedPerms[key]) return true;
  }
  return false;
}
