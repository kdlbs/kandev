import { useCallback, useEffect, useRef, useState } from "react";
import type { TFunction } from "i18next";
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

function guardOrFallbackError(
  cause: unknown,
  guard: SessionRecoveryGuardDetails | null,
  translate: TFunction,
  fallback: string,
): Error {
  if (guard) return new Error(sessionRecoveryGuardMessage(guard, translate));
  return asRecoveryError(cause, fallback);
}

type RecoveryOperation = { requestKey: string; operationId: number };

/** Fences in-flight recovery calls so a stale response cannot write newer state. */
function useRecoveryOperationFence(requestKey: string) {
  const activeRequestKeyRef = useRef(requestKey);
  const operationGenerationRef = useRef(0);
  if (activeRequestKeyRef.current !== requestKey) {
    activeRequestKeyRef.current = requestKey;
    operationGenerationRef.current += 1;
  }

  const beginOperation = useCallback(
    (): RecoveryOperation => ({ requestKey, operationId: ++operationGenerationRef.current }),
    [requestKey],
  );

  const isCurrentOperation = useCallback(
    (operation: RecoveryOperation) =>
      activeRequestKeyRef.current === operation.requestKey &&
      operationGenerationRef.current === operation.operationId,
    [],
  );

  return { beginOperation, isCurrentOperation };
}

/** Owns shared manual recovery state while a failed session remains visible. */
export function useSessionRecoveryActions({ taskId, sessionId }: SessionRecoveryActionsOptions) {
  const { t } = useTranslation();
  const requestKey = `${taskId}\u0000${sessionId}`;
  const { beginOperation, isCurrentOperation } = useRecoveryOperationFence(requestKey);
  const [busyAction, setBusyAction] = useState<SessionRecoveryBusyAction>(null);
  const [resumeError, setResumeError] = useState<Error | null>(null);
  const [restoreError, setRestoreError] = useState<Error | null>(null);
  const [branchDetails, setBranchDetails] = useState<BranchRecoveryDetails | null>(null);
  const [guardDetails, setGuardDetails] = useState<SessionRecoveryGuardDetails | null>(null);
  const [lastFailedAction, setLastFailedAction] = useState<SessionRecoveryAction | null>(null);
  const [recoveryNotice, setRecoveryNotice] = useState<string | null>(null);

  useEffect(() => {
    setBusyAction(null);
    setResumeError(null);
    setRestoreError(null);
    setBranchDetails(null);
    setGuardDetails(null);
    setLastFailedAction(null);
    setRecoveryNotice(null);
  }, [requestKey]);

  const recoveryError = combineRecoveryErrors(resumeError, restoreError, t);

  const handleRecover = useCallback(
    async (action: SessionRecoveryAction) => {
      const operation = beginOperation();
      setBusyAction(action);
      try {
        await requestSessionRecover(taskId, sessionId, action, t("task:failedToResumeSession"));
        if (!isCurrentOperation(operation)) return false;
        setResumeError(null);
        setRestoreError(null);
        setBranchDetails(null);
        setGuardDetails(null);
        setLastFailedAction(null);
        setRecoveryNotice(null);
      } catch (cause) {
        if (!isCurrentOperation(operation)) return false;
        const guard = sessionRecoveryGuardDetails(cause);
        setResumeError(guardOrFallbackError(cause, guard, t, t("task:failedToResumeSession")));
        setRestoreError(null);
        setBranchDetails(guard ? null : branchRecoveryDetails(cause));
        setGuardDetails(guard);
        setLastFailedAction(action);
        setRecoveryNotice(null);
        return false;
      } finally {
        if (isCurrentOperation(operation)) setBusyAction(null);
      }
      return true;
    },
    [beginOperation, isCurrentOperation, sessionId, taskId, t],
  );

  const handleRestore = useCallback(async () => {
    const operation = beginOperation();
    setBusyAction("restore");
    setRestoreError(null);
    try {
      await restoreSessionWorkspace(taskId, sessionId, t("task:failedToRestoreWorkspace"));
      if (!isCurrentOperation(operation)) return;
      setResumeError(null);
      setRestoreError(null);
      setBranchDetails(null);
      setGuardDetails(null);
      setLastFailedAction(null);
      setRecoveryNotice(t("task:resumeFailedWorkspaceReadOnly"));
    } catch (cause) {
      if (!isCurrentOperation(operation)) return;
      const guard = sessionRecoveryGuardDetails(cause);
      setRestoreError(guardOrFallbackError(cause, guard, t, t("task:failedToRestoreWorkspace")));
      setGuardDetails(guard ?? guardDetails);
      setRecoveryNotice(null);
    } finally {
      if (isCurrentOperation(operation)) setBusyAction(null);
    }
  }, [beginOperation, guardDetails, isCurrentOperation, sessionId, taskId, t]);

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
