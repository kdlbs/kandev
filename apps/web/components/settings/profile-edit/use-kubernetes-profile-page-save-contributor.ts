"use client";

import { useEffect, useRef, useState } from "react";

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
  connectionLoaded: boolean;
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
  connectionLoaded,
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
  const [baselineReady, setBaselineReady] = useState(!isRemote && connectionLoaded);
  const latestProfile = useRef({ payload: profilePayload, revision: profileRevision });
  latestProfile.current = { payload: profilePayload, revision: profileRevision };

  useEffect(() => {
    if (!baselineReady && connectionLoaded && gitIdentityLoaded) {
      setSavedProfileRevision(profileRevision);
      setBaselineReady(true);
    }
  }, [baselineReady, connectionLoaded, gitIdentityLoaded, profileRevision]);

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
      const latest = latestProfile.current;
      await saveProfile(latest.payload);
      setSavedProfileRevision(latest.revision);
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
