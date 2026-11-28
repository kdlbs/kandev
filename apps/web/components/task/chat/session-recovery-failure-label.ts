import type { TFunction } from "i18next";
import type { ManualSessionRecoveryFailure } from "@/hooks/domains/session/use-session-recovery-actions";

export function sessionRecoveryFailureLabel(
  operation: ManualSessionRecoveryFailure["operation"] | undefined,
  t: TFunction,
) {
  if (operation === "restore_workspace") return t("task:failedToRestoreWorkspace");
  if (operation === "fresh_start") return t("task:failedToStartFreshSession");
  return t("task:failedToResumeSession");
}
