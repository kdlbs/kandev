"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { isDynamicErrorPolicyValid } from "@/components/settings/dynamic-agent-policy-editor";
import { updateAgentProfileAction } from "@/app/actions/agents";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Agent, AgentProfile } from "@/lib/types/http";
import type {
  DynamicAgentCandidate,
  DynamicErrorClass,
  DynamicErrorPolicy,
} from "@/lib/types/agent-profile";
import {
  dynamicDraftRevision,
  useDynamicAgentProfileEditorDraft,
} from "@/components/settings/dynamic-agent-profile-editor-draft";

type DynamicAgentProfileEditorStateProps = {
  agent: Agent;
  profile: AgentProfile;
  onDraftChange?: (patch: Pick<AgentProfile, "name" | "dynamic" | "enabled">) => void;
};

export type DynamicAgentProfileEditorState = {
  name: string;
  profileEnabled: boolean;
  standalone: boolean;
  routingEnabled: boolean;
  enabledLabel: string;
  concreteProfiles: AgentProfile[];
  availableProfileOptions: ReturnType<typeof toAgentProfileOption>[];
  updateName: (name: string) => void;
  updateProfileEnabled: (enabled: boolean) => void;
  addCandidate: (executionProfileId: string) => void;
  moveCandidate: (index: number, direction: -1 | 1) => void;
  removeCandidate: (index: number) => void;
  updateCandidate: (index: number, patch: Partial<DynamicAgentCandidate>) => void;
  updateCandidatePolicy: (
    index: number,
    errorClass: DynamicErrorClass,
    patch: Partial<DynamicErrorPolicy>,
  ) => void;
  candidates: DynamicAgentCandidate[];
};

function dynamicProfilePayload(
  name: string,
  enabled: boolean,
  version: number,
  candidates: DynamicAgentCandidate[],
) {
  return {
    name: name.trim(),
    enabled,
    dynamic: {
      version,
      candidates: candidates.map((candidate, position) => ({
        position,
        execution_profile_id: candidate.executionProfileId,
        enabled: candidate.enabled,
        policies: {
          version: candidate.policies.version,
          transient: {
            retry: {
              enabled: candidate.policies.transient.retry.enabled,
              max_retries: candidate.policies.transient.retry.maxRetries,
              initial_interval_seconds: candidate.policies.transient.retry.initialIntervalSeconds,
            },
            wait_for_reset: {
              enabled: candidate.policies.transient.waitForReset.enabled,
              max_wait_seconds: candidate.policies.transient.waitForReset.maxWaitSeconds,
            },
            on_exhausted: candidate.policies.transient.onExhausted,
          },
          hard: {
            retry: {
              enabled: candidate.policies.hard.retry.enabled,
              max_retries: candidate.policies.hard.retry.maxRetries,
              initial_interval_seconds: candidate.policies.hard.retry.initialIntervalSeconds,
            },
            wait_for_reset: {
              enabled: candidate.policies.hard.waitForReset.enabled,
              max_wait_seconds: candidate.policies.hard.waitForReset.maxWaitSeconds,
            },
            on_exhausted: candidate.policies.hard.onExhausted,
          },
        },
      })),
    },
  };
}

// eslint-disable-next-line max-lines-per-function -- coordinates persistence and the shared save surface.
export function useDynamicAgentProfileEditorState({
  agent,
  profile,
  onDraftChange,
}: DynamicAgentProfileEditorStateProps): DynamicAgentProfileEditorState {
  const { t } = useTranslation();
  const enabledLabel = t("agents:enabled");
  const { toast } = useToast();
  const routingEnabled = useFeature("dynamicAgentRouting");
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const draft = useDynamicAgentProfileEditorDraft({ profile, onDraftChange });
  const [savedRevision, setSavedRevision] = useState(
    dynamicDraftRevision(
      profile.name,
      profile.dynamic?.candidates ?? [],
      profile.enabled !== false,
    ),
  );
  const [saving, setSaving] = useState(false);
  const standalone = onDraftChange === undefined;

  const concreteProfiles = useMemo(
    () =>
      settingsAgents
        .filter((item) => item.name !== "dynamic")
        .flatMap((item) => item.profiles)
        .filter(
          (candidate) =>
            candidate.kind !== "dynamic" && candidate.enabled !== false && !candidate.workspaceId,
        ),
    [settingsAgents],
  );
  const availableProfileOptions = useMemo(
    () =>
      settingsAgents.flatMap((item) =>
        item.name === "dynamic"
          ? []
          : item.profiles
              .filter(
                (candidate) =>
                  candidate.kind !== "dynamic" &&
                  candidate.enabled !== false &&
                  !candidate.workspaceId &&
                  !draft.candidates.some((current) => current.executionProfileId === candidate.id),
              )
              .map((candidate) => toAgentProfileOption(item, candidate)),
      ),
    [draft.candidates, settingsAgents],
  );

  const save = async () => {
    if (
      !routingEnabled ||
      !draft.name.trim() ||
      draft.candidates.length === 0 ||
      !profile.dynamic
    ) {
      return;
    }
    setSaving(true);
    try {
      const draftPayload = {
        name: draft.name.trim(),
        enabled: draft.profileEnabled,
        dynamic: {
          version: draft.dynamicVersion,
          candidates: draft.candidates,
        },
      };
      const payload = dynamicProfilePayload(
        draft.name,
        draft.profileEnabled,
        draft.dynamicVersion,
        draft.candidates,
      );
      if (onDraftChange) {
        onDraftChange(draftPayload);
        return;
      }
      const updated = await updateAgentProfileAction(profile.id, payload);
      const nextAgents = settingsAgents.map((item) =>
        item.id !== agent.id
          ? item
          : {
              ...item,
              profiles: item.profiles.map((itemProfile) =>
                itemProfile.id === updated.id ? updated : itemProfile,
              ),
            },
      );
      setSettingsAgents(nextAgents);
      setAgentProfiles(
        nextAgents.flatMap((item) =>
          item.profiles.map((itemProfile) => toAgentProfileOption(item, itemProfile)),
        ),
      );
      draft.applyProfile(updated);
      setSavedRevision(
        dynamicDraftRevision(
          updated.name,
          updated.dynamic?.candidates ?? draft.candidates,
          updated.enabled !== false,
        ),
      );
      toast({ title: t("agents:dynamicProfileSaved") });
    } catch (error) {
      toast({
        title: t("agents:failedToSaveProfile"),
        description: error instanceof Error ? error.message : undefined,
        variant: "error",
      });
    } finally {
      setSaving(false);
    }
  };

  const draftRevision = dynamicDraftRevision(draft.name, draft.candidates, draft.profileEnabled);
  const policiesValid = draft.candidates.every(
    (candidate) =>
      isDynamicErrorPolicyValid(candidate.policies.transient) &&
      isDynamicErrorPolicyValid(candidate.policies.hard),
  );
  let invalidReason = t("agents:dynamicPolicyValidation");
  if (!draft.name.trim()) invalidReason = t("agents:profileNameRequired");
  else if (draft.candidates.length === 0) invalidReason = t("agents:noDynamicCandidates");
  useSettingsSaveContributor({
    id: `dynamic-profile:${profile.id}`,
    revision: draftRevision,
    isDirty: standalone && draftRevision !== savedRevision,
    canSave:
      routingEnabled &&
      !saving &&
      Boolean(draft.name.trim()) &&
      draft.candidates.length > 0 &&
      policiesValid,
    invalidReason,
    save,
    discard: () => {
      if (!standalone) return;
      draft.reset();
      setSavedRevision(
        dynamicDraftRevision(
          profile.name,
          profile.dynamic?.candidates ?? [],
          profile.enabled !== false,
        ),
      );
    },
  });

  return {
    name: draft.name,
    profileEnabled: draft.profileEnabled,
    standalone,
    routingEnabled,
    enabledLabel,
    concreteProfiles,
    availableProfileOptions,
    updateName: draft.updateName,
    updateProfileEnabled: draft.updateProfileEnabled,
    addCandidate: draft.addCandidate,
    moveCandidate: draft.moveCandidate,
    removeCandidate: draft.removeCandidate,
    updateCandidate: draft.updateCandidate,
    updateCandidatePolicy: draft.updateCandidatePolicy,
    candidates: draft.candidates,
  };
}
