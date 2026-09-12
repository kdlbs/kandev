import { useCallback, useEffect, useRef, useState } from "react";
import type { TFunction } from "i18next";
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

export type ManualSessionRecoveryFailure = {
  operation: "resume" | "restore_workspace";
};

type SessionRecoveryActionsOptions = {
  taskId: string;
  sessionId: string;
  errorStamp?: string | null;
};

function combineRecoveryErrors(
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

/** Owns shared manual recovery state while a failed session remains visible. */
// eslint-disable-next-line max-lines-per-function -- the hook owns one coherent recovery state machine.
export function useSessionRecoveryActions({
  taskId,
  sessionId,
  errorStamp,
}: SessionRecoveryActionsOptions) {
  const { t } = useTranslation();
  const requestKey = `${taskId}\u0000${sessionId}\u0000${errorStamp ?? ""}`;
  const activeRequestKeyRef = useRef(requestKey);
  const operationGenerationRef = useRef(0);
  if (activeRequestKeyRef.current !== requestKey) {
    activeRequestKeyRef.current = requestKey;
    operationGenerationRef.current += 1;
  }
  const [busyAction, setBusyAction] = useState<SessionRecoveryBusyAction>(null);
  const [resumeError, setResumeError] = useState<Error | null>(null);
  const [restoreError, setRestoreError] = useState<Error | null>(null);
  const [branchDetails, setBranchDetails] = useState<BranchRecoveryDetails | null>(null);
  const [lastFailedAction, setLastFailedAction] = useState<SessionRecoveryAction | null>(null);
  const [recoveryNotice, setRecoveryNotice] = useState<string | null>(null);
  const [manualRecoveryFailure, setManualRecoveryFailure] =
    useState<ManualSessionRecoveryFailure | null>(null);

  useEffect(() => {
    setBusyAction(null);
    setResumeError(null);
    setRestoreError(null);
    setBranchDetails(null);
    setLastFailedAction(null);
    setRecoveryNotice(null);
    setManualRecoveryFailure(null);
  }, [requestKey]);

  const beginOperation = useCallback(
    (action: SessionRecoveryBusyAction) => {
      const operationId = ++operationGenerationRef.current;
      setBusyAction(action);
      return { requestKey, operationId };
    },
    [requestKey],
  );

  const isCurrentOperation = useCallback(
    (operation: { requestKey: string; operationId: number }) =>
      activeRequestKeyRef.current === operation.requestKey &&
      operationGenerationRef.current === operation.operationId,
    [],
  );

  const recoveryError = combineRecoveryErrors(resumeError, restoreError, t);

  const handleRecover = useCallback(
    async (action: SessionRecoveryAction) => {
      const operation = beginOperation(action);
      try {
        await requestSessionRecover(taskId, sessionId, action, t("task:failedToResumeSession"));
        if (!isCurrentOperation(operation)) return false;
        setResumeError(null);
        setRestoreError(null);
        setBranchDetails(null);
        setLastFailedAction(null);
        setRecoveryNotice(null);
        setManualRecoveryFailure(null);
      } catch (cause) {
        if (!isCurrentOperation(operation)) return false;
        setResumeError(asRecoveryError(cause, t("task:failedToResumeSession")));
        setRestoreError(null);
        setBranchDetails(branchRecoveryDetails(cause));
        setLastFailedAction(action);
        setRecoveryNotice(null);
        setManualRecoveryFailure({ operation: "resume" });
        return false;
      } finally {
        if (isCurrentOperation(operation)) setBusyAction(null);
      }
      return true;
    },
    [beginOperation, isCurrentOperation, sessionId, taskId, t],
  );

  const handleRestore = useCallback(async () => {
    const operation = beginOperation("restore");
    setRestoreError(null);
    try {
      await restoreSessionWorkspace(taskId, sessionId, t("task:failedToRestoreWorkspace"));
      if (!isCurrentOperation(operation)) return;
      setResumeError(null);
      setRestoreError(null);
      setBranchDetails(null);
      setLastFailedAction(null);
      setRecoveryNotice(t("task:resumeFailedWorkspaceReadOnly"));
      setManualRecoveryFailure(null);
    } catch (cause) {
      if (!isCurrentOperation(operation)) return;
      setRestoreError(asRecoveryError(cause, t("task:failedToRestoreWorkspace")));
      setRecoveryNotice(null);
      setManualRecoveryFailure({ operation: "restore_workspace" });
    } finally {
      if (isCurrentOperation(operation)) setBusyAction(null);
    }
  }, [beginOperation, isCurrentOperation, sessionId, taskId, t]);

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
    manualRecoveryFailure,
    handleRecover,
    handleRestore,
    handleRetry,
    handleNewBranch,
  };
}

export type SessionRecoveryActions = ReturnType<typeof useSessionRecoveryActions>;
