"use client";

/**
 * Draft state, save/delete actions and store synchronisation for the agent
 * profile editor. Split out of `agent-profile-page.tsx` so that file stays
 * presentation-only and under the 600-line cap.
 *
 * This module holds no JSX, so `i18next/no-literal-string` never inspects it —
 * its toast copy is guarded only by the pseudo-locale.
 */

import { useMemo, useState } from "react";
import { areCLIFlagsEqual } from "@/lib/cli-flags";
import { areConfigOptionsEqual } from "@/lib/config-options";
import { arePermissionsDirty, permissionsToProfilePatch } from "@/lib/agent-permissions";
import { deleteAgentProfileAction, updateAgentProfileAction } from "@/app/actions/agents";
import { useAppStore } from "@/components/state-provider";
import type { useToast } from "@/components/toast-provider";
import type { AgentProfileDeleteConflict } from "@/components/settings/agent-profile-delete-dialog";
import { areEnvVarsEqual } from "@/components/settings/profile-edit/profile-env-vars-section";
import { t as translate } from "@/lib/i18n";
import type { Agent, AgentProfile, PermissionSetting } from "@/lib/types/http";

export type SaveStatus = "idle" | "loading" | "success" | "error";

export function useSyncAgentsToStore() {
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  return (nextAgents: Agent[]) => {
    setSettingsAgents(nextAgents);
    setAgentProfiles(
      nextAgents.flatMap((agentItem) =>
        agentItem.profiles.map((agentProfile) => ({
          id: agentProfile.id,
          label: `${agentProfile.agentDisplayName ?? ""} • ${agentProfile.name}`,
          agent_id: agentItem.id,
          agent_name: agentItem.name,
          cli_passthrough: agentProfile.cliPassthrough ?? false,
        })),
      ),
    );
  };
}

export function useProfileEditorState(
  profile: AgentProfile,
  permissionSettings: Record<string, PermissionSetting>,
) {
  const [draft, setDraft] = useState<AgentProfile>({ ...profile });
  const [savedProfile, setSavedProfile] = useState<AgentProfile>(profile);
  const [saveStatus, setSaveStatus] = useState<"idle" | "loading" | "success" | "error">("idle");

  const isDirty = useMemo(
    () =>
      draft.name !== savedProfile.name ||
      draft.model !== savedProfile.model ||
      (draft.mode ?? "") !== (savedProfile.mode ?? "") ||
      !areConfigOptionsEqual(draft.configOptions, savedProfile.configOptions) ||
      arePermissionsDirty(draft, savedProfile, permissionSettings) ||
      draft.cliPassthrough !== savedProfile.cliPassthrough ||
      !areCLIFlagsEqual(draft.cliFlags ?? [], savedProfile.cliFlags ?? []) ||
      (draft.commandPrefix ?? "") !== (savedProfile.commandPrefix ?? "") ||
      !areEnvVarsEqual(draft.envVars, savedProfile.envVars),
    [draft, savedProfile, permissionSettings],
  );

  return { draft, setDraft, savedProfile, setSavedProfile, saveStatus, setSaveStatus, isDirty };
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : translate("agents:requestFailed");
}

type ProfileEditorActionsOptions = {
  agent: Agent;
  draft: AgentProfile;
  setSavedProfile: (p: AgentProfile) => void;
  setDraft: React.Dispatch<React.SetStateAction<AgentProfile>>;
  setSaveStatus: (s: SaveStatus) => void;
  settingsAgents: Agent[];
  syncAgentsToStore: (agents: Agent[]) => void;
  toast: ReturnType<typeof useToast>["toast"];
};

export function useProfileSave({
  agent,
  draft,
  setSavedProfile,
  setDraft,
  setSaveStatus,
  settingsAgents,
  syncAgentsToStore,
  toast,
}: ProfileEditorActionsOptions) {
  return async () => {
    if (!draft.name.trim()) {
      toast({
        title: translate("agents:profileNameRequiredTitle"),
        description: translate("agents:profileNameRequiredDescription"),
        variant: "error",
      });
      return;
    }
    // Model is optional — an empty profile model means "use the agent's
    // default", which is applied through ACP session model selection at session start.
    setSaveStatus("loading");
    try {
      const updated = await updateAgentProfileAction(draft.id, {
        name: draft.name,
        model: draft.model,
        mode: draft.mode,
        config_options: draft.configOptions ?? {},
        ...permissionsToProfilePatch(draft),
        cli_passthrough: draft.cliPassthrough,
        cli_flags: draft.cliFlags,
        command_prefix: draft.commandPrefix ?? "",
        env_vars: draft.envVars ?? [],
      });
      setSavedProfile(updated);
      setDraft((current) => preserveNewerProfileDraft(current, draft, updated));
      const nextAgents = settingsAgents.map((agentItem: Agent) =>
        agentItem.id === agent.id
          ? {
              ...agentItem,
              profiles: agentItem.profiles.map((p: AgentProfile) =>
                p.id === updated.id ? updated : p,
              ),
            }
          : agentItem,
      );
      syncAgentsToStore(nextAgents);
      setSaveStatus("success");
    } catch (error) {
      setSaveStatus("error");
      toast({
        title: translate("agents:failedToSaveProfile"),
        description: errorMessage(error),
        variant: "error",
      });
      throw error;
    }
  };
}

export function preserveNewerProfileDraft(
  current: AgentProfile,
  submitted: AgentProfile,
  saved: AgentProfile,
): AgentProfile {
  return current === submitted ? saved : current;
}

export function useProfileDelete(
  agent: Agent,
  draft: AgentProfile,
  settingsAgents: Agent[],
  syncAgentsToStore: (agents: Agent[]) => void,
  toast: ReturnType<typeof useToast>["toast"],
) {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [conflict, setConflict] = useState<AgentProfileDeleteConflict | null>(null);

  const removeProfileFromStore = () => {
    const nextAgents = settingsAgents.map((agentItem: Agent) =>
      agentItem.id === agent.id
        ? {
            ...agentItem,
            profiles: agentItem.profiles.filter((p: AgentProfile) => p.id !== draft.id),
          }
        : agentItem,
    );
    syncAgentsToStore(nextAgents);
    window.location.assign("/settings/agents");
  };

  const requestDelete = () => {
    setShowDeleteConfirm(true);
  };

  const handleDeleteProfile = async () => {
    setShowDeleteConfirm(false);
    const result = await deleteAgentProfileAction(draft.id);
    if (result.status === "ok") {
      removeProfileFromStore();
    } else if (result.status === "conflict") {
      setConflict({
        activeSessions: result.activeSessions,
        watchers: result.watchers,
        routingTiers: result.routingTiers,
      });
    } else {
      toast({
        title: translate("agents:failedToDeleteProfile"),
        description: result.message,
        variant: "error",
      });
    }
  };

  const handleForceDelete = async () => {
    const result = await deleteAgentProfileAction(draft.id, true);
    setConflict(null);
    if (result.status === "ok") {
      removeProfileFromStore();
    } else if (result.status === "conflict") {
      setConflict({
        activeSessions: result.activeSessions,
        watchers: result.watchers,
        routingTiers: result.routingTiers,
      });
    } else if (result.status === "error") {
      toast({
        title: translate("agents:failedToDeleteProfile"),
        description: result.message,
        variant: "error",
      });
    }
  };

  return {
    requestDelete,
    showDeleteConfirm,
    setShowDeleteConfirm,
    handleDeleteProfile,
    conflict,
    setConflict,
    handleForceDelete,
  };
}
