import { useCallback, useState } from "react";
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
  const [recoveryError, setRecoveryError] = useState<Error | null>(null);
  const [branchDetails, setBranchDetails] = useState<BranchRecoveryDetails | null>(null);
  const [lastFailedAction, setLastFailedAction] = useState<SessionRecoveryAction | null>(null);
  const [recoveryNotice, setRecoveryNotice] = useState<string | null>(null);

  const handleRecover = useCallback(
    async (action: SessionRecoveryAction) => {
      setBusyAction(action);
      try {
        await requestSessionRecover(taskId, sessionId, action, t("task:failedToResumeSession"));
        setRecoveryError(null);
        setBranchDetails(null);
        setLastFailedAction(null);
        setRecoveryNotice(null);
      } catch (cause) {
        setRecoveryError(asRecoveryError(cause, t("task:failedToResumeSession")));
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
    const failedMessage = recoveryError?.message ?? t("task:failedToResumeSession");
    setBusyAction("restore");
    try {
      await restoreSessionWorkspace(taskId, sessionId, t("task:failedToRestoreWorkspace"));
      setRecoveryError(null);
      setBranchDetails(null);
      setLastFailedAction(null);
      setRecoveryNotice(t("task:resumeFailedWorkspaceReadOnly", { error: failedMessage }));
    } catch (cause) {
      setRecoveryError(asRecoveryError(cause, t("task:failedToRestoreWorkspace")));
      setRecoveryNotice(null);
    } finally {
      setBusyAction(null);
    }
  }, [recoveryError, sessionId, taskId, t]);

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
