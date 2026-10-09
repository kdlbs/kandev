import { areCLIFlagsEqual } from "@/lib/cli-flags";
import { areConfigOptionsEqual } from "@/lib/config-options";
import { arePermissionsDirty } from "@/lib/agent-permissions";
import { areEnvVarsEqual } from "@/components/settings/profile-edit/profile-env-vars-section";
import type { AgentProfile, PermissionSetting } from "@/lib/types/http";

/** True when any OpenAI-compatible provider field of the draft differs. */
export function isProviderConfigDirty(draft: AgentProfile, savedProfile: AgentProfile): boolean {
  return (
    (draft.providerKind ?? "") !== (savedProfile.providerKind ?? "") ||
    (draft.providerBaseUrl ?? "") !== (savedProfile.providerBaseUrl ?? "") ||
    (draft.providerApiKeySecretId ?? "") !== (savedProfile.providerApiKeySecretId ?? "")
  );
}

function areMcpSelectedServersEqual(left: string[] = [], right: string[] = []): boolean {
  if (left.length !== right.length) return false;
  const sortedLeft = [...left].sort();
  const sortedRight = [...right].sort();
  return sortedLeft.every((serverId, index) => serverId === sortedRight[index]);
}

/**
 * True when any editable field of the profile editor draft differs from the
 * last-saved profile. Drives the settings save bar's dirty state.
 */
export function isProfileDirty(
  draft: AgentProfile,
  savedProfile: AgentProfile,
  permissionSettings: Record<string, PermissionSetting>,
): boolean {
  return (
    hasCoreProfileFieldsChanged(draft, savedProfile) ||
    hasExecutionProfileFieldsChanged(draft, savedProfile, permissionSettings)
  );
}

function hasCoreProfileFieldsChanged(draft: AgentProfile, savedProfile: AgentProfile): boolean {
  return [
    draft.name !== savedProfile.name,
    draft.model !== savedProfile.model,
    (draft.fallbackModel ?? "") !== (savedProfile.fallbackModel ?? ""),
    (draft.autoFallback ?? false) !== (savedProfile.autoFallback ?? false),
    (draft.requireExactModel ?? false) !== (savedProfile.requireExactModel ?? false),
    (draft.mode ?? "") !== (savedProfile.mode ?? ""),
    !areConfigOptionsEqual(draft.configOptions, savedProfile.configOptions),
  ].some(Boolean);
}

function hasExecutionProfileFieldsChanged(
  draft: AgentProfile,
  savedProfile: AgentProfile,
  permissionSettings: Record<string, PermissionSetting>,
): boolean {
  return [
    arePermissionsDirty(draft, savedProfile, permissionSettings),
    draft.cliPassthrough !== savedProfile.cliPassthrough,
    (draft.cursorMcpAuthEnabled ?? true) !== (savedProfile.cursorMcpAuthEnabled ?? true),
    (draft.cursorPluginsMcpEnabled ?? true) !== (savedProfile.cursorPluginsMcpEnabled ?? true),
    (draft.mcpSelectionMode ?? "inherit") !== (savedProfile.mcpSelectionMode ?? "inherit"),
    !areMcpSelectedServersEqual(draft.mcpSelectedServers, savedProfile.mcpSelectedServers),
    (draft.enabled ?? true) !== (savedProfile.enabled ?? true),
    !areCLIFlagsEqual(draft.cliFlags ?? [], savedProfile.cliFlags ?? []),
    (draft.commandPrefix ?? "") !== (savedProfile.commandPrefix ?? ""),
    isProviderConfigDirty(draft, savedProfile),
    !areEnvVarsEqual(draft.envVars, savedProfile.envVars),
  ].some(Boolean);
}
