import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  asRecoveryError,
  branchRecoveryDetails,
  requestSessionRecover,
  restoreSessionWorkspace,
  type BranchRecoveryDetails,
  type SessionRecoveryAction,
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
        setLastFailedAction(null);
        setRecoveryNotice(null);
      } catch (cause) {
        setResumeError(asRecoveryError(cause, t("task:failedToResumeSession")));
        setRestoreError(null);
        setBranchDetails(branchRecoveryDetails(cause));
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
    const failedMessage = resumeError?.message ?? t("task:failedToResumeSession");
    setBusyAction("restore");
    setRestoreError(null);
    try {
      await restoreSessionWorkspace(taskId, sessionId, t("task:failedToRestoreWorkspace"));
      setResumeError(null);
      setRestoreError(null);
      setBranchDetails(null);
      setLastFailedAction(null);
      setRecoveryNotice(t("task:resumeFailedWorkspaceReadOnly", { error: failedMessage }));
    } catch (cause) {
      setRestoreError(asRecoveryError(cause, t("task:failedToRestoreWorkspace")));
      setRecoveryNotice(null);
    } finally {
      setBusyAction(null);
    }
  }, [resumeError, sessionId, taskId, t]);

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
    recoveryNotice,
    handleRecover,
    handleRestore,
    handleRetry,
    handleNewBranch,
  };
}

export type SessionRecoveryActions = ReturnType<typeof useSessionRecoveryActions>;
