import type { TFunction } from "i18next";
import {
  asRecoveryError,
  resolveRequestErrorMessage,
  runtimeCleanupFailureDetails,
  sessionRecoveryGuardMessage,
  type SessionRecoveryGuardDetails,
} from "@/lib/services/session-recovery-service";

export function combineRecoveryErrors(
  resumeError: Error | null,
  restoreError: Error | null,
  translate: TFunction,
): Error | null {
  if (!resumeError || !restoreError) return restoreError ?? resumeError;
  return new Error(
    translate("task:resumeAndRestoreFailed", {
      resumeError: resumeError.message,
      restoreError: restoreError.message,
    }),
  );
}

export function guardOrFallbackError(
  cause: unknown,
  guard: SessionRecoveryGuardDetails | null,
  translate: TFunction,
  fallback: string,
): Error {
  if (guard) return new Error(sessionRecoveryGuardMessage(guard, translate));
  if (runtimeCleanupFailureDetails(cause)) {
    return new Error(resolveRequestErrorMessage(cause, translate, fallback));
  }
  return asRecoveryError(cause, fallback);
}
