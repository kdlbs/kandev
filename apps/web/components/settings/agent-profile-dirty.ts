import { areCLIFlagsEqual } from "@/lib/cli-flags";
import { areConfigOptionsEqual } from "@/lib/config-options";
import { arePermissionsDirty } from "@/lib/agent-permissions";
import { areEnvVarsEqual } from "@/components/settings/profile-edit/profile-env-vars-section";
import type { AgentProfile, PermissionSetting } from "@/lib/types/http";

/**
 * True when any editable field of the profile editor draft differs from the
 * last-saved profile. Drives the settings save bar's dirty state.
 */
export function isProfileDirty(
  draft: AgentProfile,
  savedProfile: AgentProfile,
  permissionSettings: Record<string, PermissionSetting>,
): boolean {
  const changed = [
    draft.name !== savedProfile.name,
    draft.model !== savedProfile.model,
    (draft.mode ?? "") !== (savedProfile.mode ?? ""),
    !areConfigOptionsEqual(draft.configOptions, savedProfile.configOptions),
    arePermissionsDirty(draft, savedProfile, permissionSettings),
    draft.cliPassthrough !== savedProfile.cliPassthrough,
    (draft.enabled ?? true) !== (savedProfile.enabled ?? true),
    !areCLIFlagsEqual(draft.cliFlags ?? [], savedProfile.cliFlags ?? []),
    (draft.commandPrefix ?? "") !== (savedProfile.commandPrefix ?? ""),
    !areEnvVarsEqual(draft.envVars, savedProfile.envVars),
  ];
  return changed.some(Boolean);
}
