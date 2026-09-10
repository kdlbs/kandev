import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  asRecoveryError,
  branchRecoveryDetails,
  requestSessionRecover,
  restoreSessionWorkspace,
  sessionRecoveryGuardDetails,
  sessionRecoveryGuardMessage,
  type BranchRecoveryDetails,
  type SessionRecoveryAction,
  type SessionRecoveryGuardDetails,
} from "@/lib/services/session-recovery-service";

export type SessionRecoveryBusyAction = SessionRecoveryAction | "restore" | null;

type SessionRecoveryActionsOptions = {
  taskId: string;
  sessionId: string;
};

/** Owns shared manual recovery state while a failed session remains visible. */
export function useSessionRecoveryActions({ taskId, sessionId }: SessionRecoveryActionsOptions) {
  const { t } = useTranslation();
  const [busyAction, setBusyAction] = useState<SessionRecoveryBusyAction>(null);
  const [resumeError, setResumeError] = useState<Error | null>(null);
  const [restoreError, setRestoreError] = useState<Error | null>(null);
  const [branchDetails, setBranchDetails] = useState<BranchRecoveryDetails | null>(null);
  const [guardDetails, setGuardDetails] = useState<SessionRecoveryGuardDetails | null>(null);
  const [lastFailedAction, setLastFailedAction] = useState<SessionRecoveryAction | null>(null);
  const [recoveryNotice, setRecoveryNotice] = useState<string | null>(null);

  const recoveryError = useMemo(() => {
    if (resumeError && restoreError) {
      return new Error(
        t("task:resumeAndRestoreFailed", {
          resumeError: resumeError.message,
          restoreError: restoreError.message,
        }),
      );
    }
    return restoreError ?? resumeError;
  }, [restoreError, resumeError, t]);

  const handleRecover = useCallback(
    async (action: SessionRecoveryAction) => {
      setBusyAction(action);
      try {
        await requestSessionRecover(taskId, sessionId, action, t("task:failedToResumeSession"));
        setResumeError(null);
        setRestoreError(null);
        setBranchDetails(null);
        setGuardDetails(null);
        setLastFailedAction(null);
        setRecoveryNotice(null);
      } catch (cause) {
        const guard = sessionRecoveryGuardDetails(cause);
        setResumeError(
          guard
            ? new Error(sessionRecoveryGuardMessage(guard, t))
            : asRecoveryError(cause, t("task:failedToResumeSession")),
        );
        setRestoreError(null);
        setBranchDetails(guard ? null : branchRecoveryDetails(cause));
        setGuardDetails(guard);
        setLastFailedAction(action);
        setRecoveryNotice(null);
        return false;
      } finally {
        setBusyAction(null);
      }
      return true;
    },
    [sessionId, taskId, t],
  );

  const handleRestore = useCallback(async () => {
    setBusyAction("restore");
    setRestoreError(null);
    try {
      await restoreSessionWorkspace(taskId, sessionId, t("task:failedToRestoreWorkspace"));
      setResumeError(null);
      setRestoreError(null);
      setBranchDetails(null);
      setGuardDetails(null);
      setLastFailedAction(null);
      setRecoveryNotice(t("task:resumeFailedWorkspaceReadOnly"));
    } catch (cause) {
      const guard = sessionRecoveryGuardDetails(cause);
      setRestoreError(
        guard
          ? new Error(sessionRecoveryGuardMessage(guard, t))
          : asRecoveryError(cause, t("task:failedToRestoreWorkspace")),
      );
      setGuardDetails(guard ?? guardDetails);
      setRecoveryNotice(null);
    } finally {
      setBusyAction(null);
    }
  }, [guardDetails, resumeError, sessionId, taskId, t]);

  const handleRetry = useCallback(() => {
    return handleRecover(lastFailedAction ?? "resume");
  }, [handleRecover, lastFailedAction]);

  const handleNewBranch = useCallback(() => {
    return handleRecover("resume_new_branch");
  }, [handleRecover]);

  return {
    busyAction,
    recoveryError,
    branchDetails,
    guardDetails,
    recoveryNotice,
    handleRecover,
    handleRestore,
    handleRetry,
    handleNewBranch,
  };
}

export type SessionRecoveryActions = ReturnType<typeof useSessionRecoveryActions>;
