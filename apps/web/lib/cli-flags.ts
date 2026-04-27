import type { CLIFlag, PermissionSetting } from "@/lib/types/http";

/**
 * Deep-equality comparison for a profile's cli_flags list. Used by the
 * profile dirty-detection paths in both the agent setup page and the
 * per-profile editor. The list order matters — it matches the order the
 * backend stores and the order CLIFlagsField renders.
 */
export function areCLIFlagsEqual(
  a: CLIFlag[] | null | undefined,
  b: CLIFlag[] | null | undefined,
): boolean {
  const left = a ?? [];
  const right = b ?? [];
  if (left.length !== right.length) return false;
  for (let i = 0; i < left.length; i++) {
    if (
      left[i].flag !== right[i].flag ||
      left[i].enabled !== right[i].enabled ||
      (left[i].description ?? "") !== (right[i].description ?? "")
    ) {
      return false;
    }
  }
  return true;
}

/**
 * Build the default cli_flags list for a new draft profile from the
 * agent's curated PermissionSettings catalogue. Mirrors the backend
 * seedCLIFlags in apps/backend/internal/agent/settings/controller/profile_crud.go
 * so a draft created in the UI matches what the backend would seed when
 * the field is omitted from a create request. Only entries that target a
 * CLI flag are included; per-flag metadata (description, flag text,
 * default enabled) is copied so the draft is self-contained.
 */
export function seedDefaultCLIFlags(
  permissionSettings: Record<string, PermissionSetting>,
): CLIFlag[] {
  const out: CLIFlag[] = [];
  for (const s of Object.values(permissionSettings)) {
    if (!s.supported || s.apply_method !== "cli_flag" || !s.cli_flag) continue;
    const flagText = s.cli_flag_value ? `${s.cli_flag} ${s.cli_flag_value}` : s.cli_flag;
    out.push({
      description: s.description || s.label,
      flag: flagText,
      enabled: s.default,
    });
  }
  out.sort((a, b) => a.flag.localeCompare(b.flag));
  return out;
}
