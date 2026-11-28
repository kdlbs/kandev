import type { TFunction } from "i18next";
import type { ManualSessionRecoveryFailure } from "@/hooks/domains/session/use-session-recovery-actions";
import type { ActiveSessionRecovery } from "@/lib/active-session-recovery";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { sessionRecoveryFailureLabel } from "./session-recovery-failure-label";
import { sessionRecoveryAction } from "./messages/action-message-recovery";

export function sessionRecoveryFailureCopy(
  input: {
    operation?: ManualSessionRecoveryFailure["operation"];
    branchUnavailable: boolean;
    guardedError?: string | null;
  },
  t: TFunction,
) {
  let failure = sessionRecoveryFailureLabel(input.operation, t);
  if (input.branchUnavailable) failure = t("task:branchIsNoLongerAvailable");
  if (input.guardedError) failure = sanitizeSessionErrorDetails(input.guardedError, 240) || failure;
  return failure;
}

export function isManagedRuntimeFailure(kind: string) {
  return (
    kind === "managed_runtime_npm_resolution" ||
    kind === "managed_runtime_npm_policy" ||
    kind === "managed_runtime_startup"
  );
}

export function isManagedRuntimeRecovery(model: ActiveSessionRecovery) {
  if (isManagedRuntimeFailure(model.kind)) return true;
  return Boolean(
    model.metadata?.actions?.some((action) => sessionRecoveryAction(action) === "runtime_retry"),
  );
}
