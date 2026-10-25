"use client";

import { useEffect, useState } from "react";
import { useKubernetesSessionImpact } from "@/hooks/domains/settings/use-kubernetes-settings";
import type { ProfileEnvVar } from "@/lib/types/http";
import { saveWithKubernetesSessionConfirmation } from "../kubernetes-save-confirmation";
import { useSettingsSaveContributor } from "../settings-save-provider";
import { serializeSettingsRevision } from "../settings-save-revision";

export type ExecutorProfileSavePayload = {
  name: string;
  mcp_policy?: string;
  config?: Record<string, string>;
  prepare_script: string;
  cleanup_script: string;
  env_vars: ProfileEnvVar[];
};

type ExecutorProfileSaveContributorOptions = {
  enabled?: boolean;
  executorId: string;
  profileId: string;
  payload: ExecutorProfileSavePayload;
  isRemote: boolean;
  gitIdentityLoaded: boolean;
  isKubernetes: boolean;
  canManageKubernetes: boolean;
  invalidReason?: string;
  save: (payload: ExecutorProfileSavePayload) => Promise<void>;
  discard: () => void;
};

export function useExecutorProfileSaveContributor({
  enabled = true,
  executorId,
  profileId,
  payload,
  isRemote,
  gitIdentityLoaded,
  isKubernetes,
  canManageKubernetes,
  invalidReason,
  save,
  discard,
}: ExecutorProfileSaveContributorOptions): boolean {
  const sessionImpact = useKubernetesSessionImpact(executorId, enabled && isKubernetes);
  const revision = serializeSettingsRevision(payload);
  const [savedRevision, setSavedRevision] = useState(revision);
  const [baselineReady, setBaselineReady] = useState(!isRemote);
  useEffect(() => {
    if (!baselineReady && gitIdentityLoaded) {
      setSavedRevision(revision);
      setBaselineReady(true);
    }
  }, [baselineReady, gitIdentityLoaded, revision]);

  const handleSave = async () => {
    if (!isKubernetes) {
      await save(payload);
    } else {
      await saveWithKubernetesSessionConfirmation({
        kind: "workload",
        loadActiveSessionCount: sessionImpact.loadActiveSessionCount,
        save: () => save(payload),
      });
    }
    setSavedRevision(revision);
  };
  const allowed = !isKubernetes || canManageKubernetes;
  useSettingsSaveContributor({
    id: `executor-profile:${profileId}`,
    revision,
    isDirty: enabled && baselineReady && allowed && revision !== savedRevision,
    canSave: enabled && baselineReady && allowed && !invalidReason,
    invalidReason,
    save: handleSave,
    discard,
  });
  return baselineReady;
}
