"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import type { AgentProfile } from "@/lib/types/http";
import type {
  DynamicAgentCandidate,
  DynamicAgentPolicy,
  DynamicErrorClass,
  DynamicErrorPolicy,
} from "@/lib/types/agent-profile";
import {
  isProfileRevisionNewer,
  reconcileAgentProfileSnapshot,
  sameEditableProfile,
} from "@/components/settings/agent-profile-reconciliation";

type DynamicAgentProfileEditorDraftProps = {
  profile: AgentProfile;
  onDraftChange?: (patch: Pick<AgentProfile, "name" | "dynamic" | "enabled">) => void;
};

export type DynamicAgentProfileEditorDraft = {
  name: string;
  candidates: DynamicAgentCandidate[];
  profileEnabled: boolean;
  dynamicVersion: number;
  currentProfile: AgentProfile;
  savedProfile: AgentProfile;
  hasExternalConflict: boolean;
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
  applyProfile: (profile: AgentProfile) => void;
  markProfileSubmitted: (profile: AgentProfile | null) => void;
  acceptProfileSaveResponse: (profile: AgentProfile, submitted: AgentProfile) => void;
  reset: () => void;
};

const defaultDynamicErrorPolicy = (): DynamicErrorPolicy => ({
  retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
  waitForReset: { enabled: false, maxWaitSeconds: 0 },
  onExhausted: "skip",
});

const defaultDynamicPolicy = (): DynamicAgentPolicy => ({
  version: 1,
  transient: defaultDynamicErrorPolicy(),
  hard: defaultDynamicErrorPolicy(),
});

export function dynamicDraftRevision(
  name: string,
  candidates: DynamicAgentCandidate[],
  enabled: boolean,
): string {
  return JSON.stringify({ name, candidates, enabled });
}

// eslint-disable-next-line max-lines-per-function -- coordinates one draft and its candidate mutations.
export function useDynamicAgentProfileEditorDraft({
  profile,
  onDraftChange,
}: DynamicAgentProfileEditorDraftProps): DynamicAgentProfileEditorDraft {
  const [name, setName] = useState(profile.name);
  const [candidates, setCandidates] = useState<DynamicAgentCandidate[]>(
    profile.dynamic?.candidates ?? [],
  );
  const [profileEnabled, setProfileEnabled] = useState(profile.enabled !== false);
  const [dynamicVersion, setDynamicVersion] = useState(profile.dynamic?.version ?? 1);
  const [savedProfile, setSavedProfile] = useState(profile);
  const [hasExternalConflict, setHasExternalConflict] = useState(false);
  const previousProfileRef = useRef(profile);
  const submittedProfileRef = useRef<AgentProfile | null>(null);
  const savedProfileRef = useRef(savedProfile);
  const currentProfileRef = useRef(profile);

  const currentProfile = {
    ...profile,
    name,
    enabled: profileEnabled,
    dynamic: { version: dynamicVersion, candidates },
  };
  savedProfileRef.current = savedProfile;
  currentProfileRef.current = currentProfile;

  const notifyDraft = (
    nextName: string,
    nextCandidates: DynamicAgentCandidate[],
    nextEnabled = profileEnabled,
  ) => {
    onDraftChange?.({
      name: nextName,
      enabled: nextEnabled,
      dynamic: { version: dynamicVersion, candidates: nextCandidates },
    });
  };

  const updateName = (nextName: string) => {
    setName(nextName);
    notifyDraft(nextName, candidates);
  };

  const updateProfileEnabled = (enabled: boolean) => {
    setProfileEnabled(enabled);
    notifyDraft(name, candidates, enabled);
  };

  const addCandidate = (executionProfileId: string) => {
    setCandidates((current) => {
      const next = [
        ...current,
        {
          position: current.length,
          executionProfileId: toAgentProfileId(executionProfileId),
          enabled: true,
          policies: defaultDynamicPolicy(),
        },
      ];
      notifyDraft(name, next);
      return next;
    });
  };

  const moveCandidate = (index: number, direction: -1 | 1) => {
    setCandidates((current) => {
      const target = index + direction;
      if (target < 0 || target >= current.length) return current;
      const next = [...current];
      [next[index], next[target]] = [next[target], next[index]];
      const reordered = next.map((candidate, position) => ({ ...candidate, position }));
      notifyDraft(name, reordered);
      return reordered;
    });
  };

  const removeCandidate = (index: number) => {
    setCandidates((current) => {
      const next = current
        .filter((_, candidateIndex) => candidateIndex !== index)
        .map((candidate, position) => ({ ...candidate, position }));
      notifyDraft(name, next);
      return next;
    });
  };

  const updateCandidate = (index: number, patch: Partial<DynamicAgentCandidate>) => {
    setCandidates((current) => {
      const next = current.map((candidate, candidateIndex) =>
        candidateIndex === index ? { ...candidate, ...patch } : candidate,
      );
      notifyDraft(name, next);
      return next;
    });
  };

  const updateCandidatePolicy = (
    index: number,
    errorClass: DynamicErrorClass,
    patch: Partial<DynamicErrorPolicy>,
  ) => {
    setCandidates((current) => {
      const next = current.map((candidate, candidateIndex) =>
        candidateIndex === index
          ? {
              ...candidate,
              policies: {
                ...candidate.policies,
                [errorClass]: { ...candidate.policies[errorClass], ...patch },
              },
            }
          : candidate,
      );
      notifyDraft(name, next);
      return next;
    });
  };

  const reset = () => {
    const nextProfile = savedProfileRef.current;
    setName(nextProfile.name);
    setProfileEnabled(nextProfile.enabled !== false);
    setCandidates(nextProfile.dynamic?.candidates ?? []);
    setDynamicVersion(nextProfile.dynamic?.version ?? 1);
    setHasExternalConflict(false);
    submittedProfileRef.current = null;
  };

  const applyProfile = useCallback((nextProfile: AgentProfile) => {
    setName(nextProfile.name);
    setProfileEnabled(nextProfile.enabled !== false);
    setCandidates(nextProfile.dynamic?.candidates ?? []);
    setDynamicVersion(nextProfile.dynamic?.version ?? 1);
  }, []);

  const markProfileSubmitted = useCallback((submitted: AgentProfile | null) => {
    submittedProfileRef.current = submitted;
  }, []);

  const acceptProfileSaveResponse = useCallback(
    (nextProfile: AgentProfile, submitted: AgentProfile) => {
      if (!isProfileRevisionNewer(nextProfile, savedProfileRef.current)) {
        submittedProfileRef.current = null;
        return;
      }
      setSavedProfile(nextProfile);
      if (sameEditableProfile(currentProfileRef.current, submitted)) applyProfile(nextProfile);
      setHasExternalConflict(false);
      submittedProfileRef.current = null;
    },
    [applyProfile],
  );

  useEffect(() => {
    const previous = previousProfileRef.current;
    previousProfileRef.current = profile;
    if (profile.id !== previous.id) {
      submittedProfileRef.current = null;
      setSavedProfile(profile);
      applyProfile(profile);
      setHasExternalConflict(false);
      return;
    }

    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming: profile,
      draft: currentProfileRef.current,
      saved: savedProfileRef.current,
      submitted: submittedProfileRef.current,
      conflicted: hasExternalConflict,
    });
    if (result.kind === "ignored") return;
    setSavedProfile(result.saved);
    if (!sameEditableProfile(result.draft, currentProfileRef.current)) applyProfile(result.draft);
    setHasExternalConflict(result.conflicted);
    if (result.kind === "own-acknowledgement") submittedProfileRef.current = null;
  }, [applyProfile, hasExternalConflict, profile]);

  return {
    name,
    candidates,
    profileEnabled,
    dynamicVersion,
    currentProfile,
    savedProfile,
    hasExternalConflict,
    updateName,
    updateProfileEnabled,
    addCandidate,
    moveCandidate,
    removeCandidate,
    updateCandidate,
    updateCandidatePolicy,
    applyProfile,
    markProfileSubmitted,
    acceptProfileSaveResponse,
    reset,
  };
}
