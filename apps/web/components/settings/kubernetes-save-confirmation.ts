"use client";

import { t as translate } from "@/lib/i18n";
import { SettingsSaveCancelledError } from "./settings-save-provider";

export type KubernetesSessionSaveKind = "connection" | "workload" | "connection_and_workload";

const CONFIRMATION_KEYS: Record<KubernetesSessionSaveKind, string> = {
  connection: "executors:kubernetesConfirmConnectionSessions",
  workload: "executors:kubernetesConfirmWorkloadSessions",
  connection_and_workload: "executors:kubernetesConfirmCombinedSessions",
};

export function requireKubernetesSessionSaveConfirmation(
  kind: KubernetesSessionSaveKind,
  activeSessionCount: number,
): void {
  if (activeSessionCount <= 0) return;
  const key = CONFIRMATION_KEYS[kind];
  if (!window.confirm(translate(key, { count: activeSessionCount }))) {
    throw new SettingsSaveCancelledError();
  }
}

export async function saveWithKubernetesSessionConfirmation({
  kind,
  loadActiveSessionCount,
  save,
}: {
  kind: KubernetesSessionSaveKind;
  loadActiveSessionCount: () => Promise<number>;
  save: () => Promise<void>;
}): Promise<void> {
  const activeSessionCount = await loadActiveSessionCount();
  requireKubernetesSessionSaveConfirmation(kind, activeSessionCount);
  await save();
}
