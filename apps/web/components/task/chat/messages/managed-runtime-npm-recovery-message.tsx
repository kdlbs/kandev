"use client";

import { useTranslation } from "react-i18next";
import { ManagedRuntimeRecoveryMessageShell } from "./managed-runtime-recovery-message-shell";
import type { ActionMeta } from "./action-message-details";

export function ManagedRuntimeNpmRecoveryMessage({
  metadata,
  taskId,
  onRecoveryRequested,
}: {
  metadata: ActionMeta;
  taskId?: string;
  onRecoveryRequested: () => void;
}) {
  const { t } = useTranslation();
  const isPolicyFailure = metadata.failure_kind === "managed_runtime_npm_policy";
  return (
    <ManagedRuntimeRecoveryMessageShell
      testId="managed-runtime-npm-recovery"
      title={t(
        isPolicyFailure ? "chat:managedRuntimeNpmPolicyTitle" : "chat:managedRuntimeNpmTitle",
      )}
      summary={t(
        isPolicyFailure ? "chat:managedRuntimeNpmPolicyBody" : "chat:managedRuntimeNpmBody",
      )}
      metadata={metadata}
      taskId={taskId}
      onRecoveryRequested={onRecoveryRequested}
      retryLabel={t("chat:managedRuntimeRetry")}
    />
  );
}
