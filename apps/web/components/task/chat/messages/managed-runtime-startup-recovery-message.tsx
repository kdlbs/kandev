"use client";

import { useTranslation } from "react-i18next";
import { managedRuntimeStartupCopy } from "../managed-runtime-startup-copy";
import { ManagedRuntimeRecoveryMessageShell } from "./managed-runtime-recovery-message-shell";
import type { ActionMeta } from "./action-message-details";

export function ManagedRuntimeStartupRecoveryMessage({
  metadata,
  taskId,
  onRecoveryRequested,
}: {
  metadata: ActionMeta;
  taskId?: string;
  onRecoveryRequested: () => void;
}) {
  const { t } = useTranslation();
  const copy = managedRuntimeStartupCopy(metadata, t);
  return (
    <ManagedRuntimeRecoveryMessageShell
      testId="managed-runtime-startup-recovery"
      title={copy.title}
      summary={copy.summary}
      metadata={metadata}
      taskId={taskId}
      onRecoveryRequested={onRecoveryRequested}
      retryLabel={t("chat:managedRuntimeRetry")}
    />
  );
}
