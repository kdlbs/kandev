import { useState } from "react";

import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { providerConfigInvalidReasonKey } from "@/lib/settings/provider-config-validation";
import type { Agent } from "@/lib/types/http";
import type { DraftAgent } from "./agent-save-helpers";

/**
 * First blocking provider-config i18n key across the agent's profiles, or
 * undefined. Only relevant when the agent advertises provider support.
 */
function providerInvalidKey(
  agent: DraftAgent,
  providerSupported: boolean,
): string | undefined {
  if (!providerSupported) return undefined;
  for (const profile of agent.profiles) {
    const key = providerConfigInvalidReasonKey({
      providerKind: profile.providerKind,
      providerBaseUrl: profile.providerBaseUrl,
      providerApiKeySecretId: profile.providerApiKeySecretId,
      model: profile.model,
    });
    if (key) return key;
  }
  return undefined;
}

function areAgentProfilesValid(agent: DraftAgent): boolean {
  return agent.profiles.every((profile) => {
    if (!profile.name.trim()) return false;
    if (profile.kind === "dynamic") return (profile.dynamic?.candidates.length ?? 0) > 0;
    return profile.model.trim().length > 0;
  });
}

function useAgentSaveRevision(agent: DraftAgent) {
  const revision = JSON.stringify(agent);
  const initial = agent.profiles.some((profile) => profile.mcp_config?.dirty) ? "" : revision;
  const [saved, setSaved] = useState(initial);
  return { revision, saved, setSaved };
}

/**
 * Explains why the shared Save control is blocked, or undefined when it is not.
 * Extracted so AgentSetupForm stays within the file's function-length limit.
 */
function resolveSaveInvalidReason(
  t: (key: string) => string,
  profilesValid: boolean,
  hasInvalidMcpConfig: boolean,
  dynamic: boolean,
): string | undefined {
  if (!profilesValid) {
    return t(dynamic ? "agents:noDynamicCandidates" : "agents:everyProfileNeedsNameAndModel");
  }
  if (hasInvalidMcpConfig) return t("agents:fixInvalidMcpConfig");
  return undefined;
}

/**
 * Wires the agent form's dirty/validity state into the shared settings Save
 * control. Extracted so AgentSetupForm stays within the file's
 * function-length limit.
 */
export function useAgentSaveContributor(params: {
  draftAgent: DraftAgent;
  savedAgent: Agent | null;
  isCreateMode: boolean;
  hasInvalidMcpConfig: boolean;
  isAgentDirty: boolean;
  handleSave: () => Promise<DraftAgent | undefined>;
  t: (key: string) => string;
}) {
  const { draftAgent, savedAgent, isCreateMode, hasInvalidMcpConfig, isAgentDirty, handleSave, t } =
    params;
  const saveRevision = useAgentSaveRevision(draftAgent);
  const profilesValid = areAgentProfilesValid(draftAgent);
  const providerBlockKey = providerInvalidKey(
    draftAgent,
    (savedAgent?.profiles ?? []).some((p) => p.providerSupported),
  );
  // Agents and agent profiles are org configuration: every mutating route
  // behind this page requires org.config.manage, which only an administrator
  // holds. Without this the save bar stays live for a member and the write
  // fails with a 403 they cannot act on.
  const canManage = useIsAdmin();
  const saveInvalidReason = !canManage
    ? t("agents:adminOnly")
    : (resolveSaveInvalidReason(
        t,
        profilesValid,
        hasInvalidMcpConfig,
        draftAgent.name === "dynamic",
      ) ?? (providerBlockKey ? t(providerBlockKey) : undefined));
  useSettingsSaveContributor({
    id: `agent:${draftAgent.id}`,
    revision: saveRevision.revision,
    isDirty: isCreateMode ? isAgentDirty : saveRevision.revision !== saveRevision.saved,
    canSave: canManage && profilesValid && !hasInvalidMcpConfig && !providerBlockKey,
    invalidReason: saveInvalidReason,
    save: async () => {
      const savedDraft = await handleSave();
      if (savedDraft) saveRevision.setSaved(JSON.stringify(savedDraft));
    },
    discard: () => undefined,
  });
}
