"use client";

/**
 * Draft state, save/delete actions and store synchronisation for the agent
 * profile editor. Split out of `agent-profile-page.tsx` so that file stays
 * presentation-only and under the 600-line cap.
 *
 * This module holds no JSX, so `i18next/no-literal-string` never inspects it —
 * its toast copy is guarded only by the pseudo-locale.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { permissionsToProfilePatch } from "@/lib/agent-permissions";
import { deleteAgentProfileAction, updateAgentProfileAction } from "@/app/actions/agents";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { isProfileDirty } from "@/components/settings/agent-profile-dirty";
import type { useToast } from "@/components/toast-provider";
import type { AgentProfileDeleteConflict } from "@/components/settings/agent-profile-delete-dialog";
import { t as translate } from "@/lib/i18n";
import {
  mergeOptionsByNewest,
  toAgentProfileOption,
  type AgentProfileOption,
} from "@/lib/state/slices/settings/types";
import { ApiError, isHandledApiError } from "@/lib/api/client";
import type { Agent, AgentProfile, PermissionSetting } from "@/lib/types/http";
import {
  isProfileRevisionNewer,
  reconcileAgentProfileSnapshot,
  sameEditableProfile,
} from "@/components/settings/agent-profile-reconciliation";

export type SaveStatus = "idle" | "loading" | "success" | "error";

/**
 * Reconcile the options slice with the next agent list by ID: options the
 * rebuild does not represent (e.g. profiles the WS handler delivered for
 * agents temporarily absent from `settingsAgents`) are preserved, and rebuilt
 * options replace any stale versions. Same rule as
 * `applyProfileDuplicated` in the duplicate hook — a save must never wipe
 * orphan options.
 */
export function reconcileAgentProfileOptions(
  previousOptions: AgentProfileOption[],
  nextAgents: Agent[],
): AgentProfileOption[] {
  const rebuiltOptions = nextAgents.flatMap((agentItem) =>
    agentItem.profiles.map((agentProfile) => toAgentProfileOption(agentItem, agentProfile)),
  );
  return mergeOptionsByNewest(previousOptions, rebuiltOptions);
}

export function useSyncAgentsToStore() {
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const storeApi = useAppStoreApi();
  return (nextAgents: Agent[]) => {
    setSettingsAgents(nextAgents);
    setAgentProfiles(
      reconcileAgentProfileOptions(storeApi.getState().agentProfiles.items, nextAgents),
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
  const [hasExternalConflict, setHasExternalConflict] = useState(false);
  const previousProfileRef = useRef(profile);
  const submittedProfileRef = useRef<AgentProfile | null>(null);
  const draftRef = useRef(draft);
  const savedProfileRef = useRef(savedProfile);
  draftRef.current = draft;
  savedProfileRef.current = savedProfile;

  useEffect(() => {
    const previous = previousProfileRef.current;
    previousProfileRef.current = profile;
    if (profile.id !== previous.id) {
      submittedProfileRef.current = null;
      setDraft(profile);
      setSavedProfile(profile);
      setHasExternalConflict(false);
      setSaveStatus("idle");
      return;
    }

    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming: profile,
      draft: draftRef.current,
      saved: savedProfileRef.current,
      submitted: submittedProfileRef.current,
      conflicted: hasExternalConflict,
    });
    if (result.kind === "ignored") return;
    setSavedProfile(result.saved);
    setDraft(result.draft);
    setHasExternalConflict(result.conflicted);
    if (result.kind === "own-acknowledgement") submittedProfileRef.current = null;
  }, [hasExternalConflict, profile]);

  const markProfileSubmitted = useCallback((submitted: AgentProfile | null) => {
    submittedProfileRef.current = submitted;
  }, []);

  const acceptProfileSaveResponse = useCallback(
    (response: AgentProfile, submitted: AgentProfile) => {
      const currentSaved = savedProfileRef.current;
      if (!isProfileRevisionNewer(response, currentSaved)) {
        submittedProfileRef.current = null;
        return;
      }
      setSavedProfile(response);
      if (sameEditableProfile(draftRef.current, submitted)) setDraft(response);
      setHasExternalConflict(false);
      submittedProfileRef.current = null;
    },
    [],
  );

  const discardProfileDraft = useCallback(() => {
    setDraft(savedProfileRef.current);
    setHasExternalConflict(false);
    submittedProfileRef.current = null;
  }, []);

  const isDirty = useMemo(
    () => isProfileDirty(draft, savedProfile, permissionSettings),
    [draft, savedProfile, permissionSettings],
  );

  return {
    draft,
    setDraft,
    savedProfile,
    setSavedProfile,
    saveStatus,
    setSaveStatus,
    isDirty,
    hasExternalConflict,
    markProfileSubmitted,
    acceptProfileSaveResponse,
    discardProfileDraft,
  };
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : translate("agents:requestFailed");
}

type ProfileEditorActionsOptions = {
  agent: Agent;
  draft: AgentProfile;
  savedProfile: AgentProfile;
  setSaveStatus: (s: SaveStatus) => void;
  markProfileSubmitted: (profile: AgentProfile | null) => void;
  acceptProfileSaveResponse: (response: AgentProfile, submitted: AgentProfile) => void;
  settingsAgents: Agent[];
  syncAgentsToStore: (agents: Agent[]) => void;
  toast: ReturnType<typeof useToast>["toast"];
  onUtilityConflict?: (agents: Array<{ id: string; name: string }>) => void;
};

export function useProfileSave({
  agent,
  draft,
  savedProfile,
  setSaveStatus,
  markProfileSubmitted,
  acceptProfileSaveResponse,
  settingsAgents,
  syncAgentsToStore,
  toast,
  onUtilityConflict,
}: ProfileEditorActionsOptions) {
  // eslint-disable-next-line complexity
  return async (force = false) => {
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
    const submitted = draft;
    markProfileSubmitted(submitted);
    try {
      const updated = await updateAgentProfileAction(
        draft.id,
        {
          name: draft.name,
          model: draft.model,
          fallback_model: draft.fallbackModel ?? "",
          auto_fallback: draft.autoFallback ?? false,
          mode: draft.mode,
          config_options: draft.configOptions ?? {},
          ...permissionsToProfilePatch(draft),
          cli_passthrough: draft.cliPassthrough,
          // Omit an unchanged enabled value so a profile editor save cannot
          // resurrect a concurrent list-toggle response from its stale draft.
          enabled:
            (draft.enabled ?? true) !== (savedProfile.enabled ?? true)
              ? (draft.enabled ?? true)
              : undefined,
          cli_flags: draft.cliFlags,
          command_prefix: draft.commandPrefix ?? "",
          env_vars: draft.envVars ?? [],
        },
        force,
      );
      acceptProfileSaveResponse(updated, submitted);
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
      markProfileSubmitted(null);
      if (
        error instanceof ApiError &&
        error.status === 409 &&
        Array.isArray((error.body as { utility_agents?: unknown[] } | null)?.utility_agents)
      ) {
        onUtilityConflict?.(
          (error.body as { utility_agents: Array<{ id: string; name: string }> }).utility_agents,
        );
      }
      setSaveStatus("error");
      if (!isHandledApiError(error)) {
        toast({
          title: translate("agents:failedToSaveProfile"),
          description: errorMessage(error),
          variant: "error",
        });
      }
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
        automations: result.automations,
        utilityAgents: result.utilityAgents,
      });
    } else if (!result.handled) {
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
        automations: result.automations,
        utilityAgents: result.utilityAgents,
      });
    } else if (result.status === "error" && !result.handled) {
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
