"use client";

import { useEffect, useState } from "react";

import { useKubernetesSessionImpact } from "@/hooks/domains/settings/use-kubernetes-settings";
import { isKubernetesExecutorDirty, type KubernetesExecutorForm } from "../kubernetes-config";
import { requireKubernetesSessionSaveConfirmation } from "../kubernetes-save-confirmation";
import { useSettingsSaveContributor } from "../settings-save-provider";
import { serializeSettingsRevision } from "../settings-save-revision";
import type { ExecutorProfileSavePayload } from "./use-executor-profile-save-contributor";

type KubernetesProfilePageSaveOptions = {
  enabled: boolean;
  executorId: string;
  profileId: string;
  connectionForm: KubernetesExecutorForm;
  connectionBaseline: KubernetesExecutorForm;
  profilePayload: ExecutorProfileSavePayload;
  isRemote: boolean;
  gitIdentityLoaded: boolean;
  canManage: boolean;
  invalidReason?: string;
  saveConnection: (form: KubernetesExecutorForm) => Promise<void>;
  markConnectionSaved: (form: KubernetesExecutorForm) => void;
  saveProfile: (payload: ExecutorProfileSavePayload) => Promise<void>;
  discard: () => void;
};

function kubernetesSaveKind(connectionDirty: boolean, profileDirty: boolean) {
  if (connectionDirty && profileDirty) return "connection_and_workload" as const;
  return connectionDirty ? ("connection" as const) : ("workload" as const);
}

export function useKubernetesProfilePageSaveContributor({
  enabled,
  executorId,
  profileId,
  connectionForm,
  connectionBaseline,
  profilePayload,
  isRemote,
  gitIdentityLoaded,
  canManage,
  invalidReason,
  saveConnection,
  markConnectionSaved,
  saveProfile,
  discard,
}: KubernetesProfilePageSaveOptions): boolean {
  const sessionImpact = useKubernetesSessionImpact(executorId, enabled);
  const profileRevision = serializeSettingsRevision(profilePayload);
  const connectionDirty = isKubernetesExecutorDirty(connectionForm, connectionBaseline);
  const [savedProfileRevision, setSavedProfileRevision] = useState(profileRevision);
  const [baselineReady, setBaselineReady] = useState(!isRemote);

  useEffect(() => {
    if (!baselineReady && gitIdentityLoaded) {
      setSavedProfileRevision(profileRevision);
      setBaselineReady(true);
    }
  }, [baselineReady, gitIdentityLoaded, profileRevision]);

  const profileDirty = profileRevision !== savedProfileRevision;
  const revision = serializeSettingsRevision({
    connection: connectionForm,
    profile: profilePayload,
  });
  const handleSave = async () => {
    const count = await sessionImpact.loadActiveSessionCount();
    requireKubernetesSessionSaveConfirmation(
      kubernetesSaveKind(connectionDirty, profileDirty),
      count,
    );
    if (connectionDirty) {
      await saveConnection(connectionForm);
      markConnectionSaved(connectionForm);
    }
    if (profileDirty) {
      await saveProfile(profilePayload);
      setSavedProfileRevision(profileRevision);
    }
  };

  useSettingsSaveContributor({
    id: `kubernetes-profile-page:${profileId}`,
    revision,
    isDirty: enabled && baselineReady && canManage && (connectionDirty || profileDirty),
    canSave: enabled && baselineReady && canManage && !invalidReason,
    invalidReason,
    save: handleSave,
    discard,
  });
  return baselineReady;
}
