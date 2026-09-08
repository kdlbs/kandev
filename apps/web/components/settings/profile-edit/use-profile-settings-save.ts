"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  isKubernetesExecutorDirty,
  parseKubernetesExecutorConfig,
  serializeKubernetesExecutorConfig,
  type KubernetesExecutorForm,
} from "@/components/settings/kubernetes-config";
import { useKubernetesExecutorResource } from "@/hooks/domains/settings/use-kubernetes-settings";
import type { Executor } from "@/lib/types/http";
import {
  useExecutorProfileSaveContributor,
  type ExecutorProfileSavePayload,
} from "./use-executor-profile-save-contributor";
import { useKubernetesProfilePageSaveContributor } from "./use-kubernetes-profile-page-save-contributor";

export function useKubernetesProfileConnection(executor: Executor, enabled: boolean) {
  const initial = useMemo(
    () => parseKubernetesExecutorConfig(executor.name, executor.config),
    [executor.config, executor.name],
  );
  const resource = useKubernetesExecutorResource(enabled ? executor.id : undefined);
  const baselineRef = useRef(initial);
  const [baseline, setBaseline] = useState(initial);
  const [form, setForm] = useState(initial);

  useEffect(() => {
    if (!resource.executor) return;
    const next = parseKubernetesExecutorConfig(resource.executor.name, resource.executor.config);
    setForm((current) =>
      isKubernetesExecutorDirty(current, baselineRef.current) ? current : next,
    );
    baselineRef.current = next;
    setBaseline(next);
  }, [resource.executor]);

  const save = useCallback(
    async (next: KubernetesExecutorForm) => {
      await resource.update(next.name.trim(), serializeKubernetesExecutorConfig(next));
    },
    [resource],
  );
  const markSaved = useCallback((next: KubernetesExecutorForm) => {
    const saved = { ...next };
    baselineRef.current = saved;
    setBaseline(saved);
  }, []);
  const reset = useCallback(() => setForm({ ...baselineRef.current }), []);
  const ready = !enabled || (!resource.loading && resource.executor !== null);
  return { form, setForm, baseline, save, markSaved, reset, ready };
}

type ProfileSettingsSaveOptions = {
  executorId: string;
  profileId: string;
  payload: ExecutorProfileSavePayload;
  isKubernetes: boolean;
  isRemote: boolean;
  gitIdentityLoaded: boolean;
  canManageKubernetes: boolean;
  invalidReason?: string;
  connection: ReturnType<typeof useKubernetesProfileConnection>;
  saveProfile: (payload: ExecutorProfileSavePayload) => Promise<void>;
  discardProfile: () => void;
  clearDiagnostics: () => void;
};

export function useProfileSettingsSave({
  executorId,
  profileId,
  payload,
  isKubernetes,
  isRemote,
  gitIdentityLoaded,
  canManageKubernetes,
  invalidReason,
  connection,
  saveProfile,
  discardProfile,
  clearDiagnostics,
}: ProfileSettingsSaveOptions): boolean {
  const profileBaselineReady = useExecutorProfileSaveContributor({
    enabled: !isKubernetes,
    executorId,
    profileId,
    payload,
    isRemote,
    gitIdentityLoaded,
    isKubernetes,
    canManageKubernetes,
    invalidReason,
    save: saveProfile,
    discard: discardProfile,
  });
  const discardKubernetes = useCallback(() => {
    connection.reset();
    discardProfile();
    clearDiagnostics();
  }, [clearDiagnostics, connection, discardProfile]);
  const kubernetesBaselineReady = useKubernetesProfilePageSaveContributor({
    enabled: isKubernetes,
    executorId,
    profileId,
    connectionForm: connection.form,
    connectionBaseline: connection.baseline,
    profilePayload: payload,
    isRemote,
    gitIdentityLoaded,
    connectionLoaded: connection.ready,
    canManage: canManageKubernetes,
    invalidReason,
    saveConnection: connection.save,
    markConnectionSaved: connection.markSaved,
    saveProfile,
    discard: discardKubernetes,
  });
  return isKubernetes ? kubernetesBaselineReady : profileBaselineReady;
}
