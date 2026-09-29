import { claimSessionRecovery, usePendingSessionRecovery } from "./session-recovery-pending";
import { useCallback, useEffect, useRef, useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import {
  asRecoveryError,
  branchRecoveryDetails,
  managedCloneRelocationRecoveryDetails,
  requestSessionRecover,
  restoreSessionWorkspace,
  sessionRecoveryGuardDetails,
  sessionRecoveryGuardMessage,
  type BranchRecoveryDetails,
  type SessionRecoveryAction,
  type SessionRecoveryGuardDetails,
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

function guardOrFallbackError(
  cause: unknown,
  guard: SessionRecoveryGuardDetails | null,
  translate: TFunction,
  fallback: string,
): Error {
  if (guard) return new Error(sessionRecoveryGuardMessage(guard, translate));
  return asRecoveryError(cause, fallback);
}

type RecoveryOperation = { requestKey: string; sessionKey: string; operationId: number };

/** Fences in-flight recovery calls so a stale response cannot write newer state. */
function useRecoveryOperationFence(
  requestKey: string,
  sessionKey: string,
  errorStamp?: string | null,
) {
  const activeRequestKeyRef = useRef(requestKey);
  const activeSessionKeyRef = useRef(sessionKey);
  const latestErrorStampRef = useRef(errorStamp);
  const operationGenerationRef = useRef(0);
  activeSessionKeyRef.current = sessionKey;
  latestErrorStampRef.current = errorStamp;
  if (activeRequestKeyRef.current !== requestKey) {
    activeRequestKeyRef.current = requestKey;
    operationGenerationRef.current += 1;
  }

  const beginOperation = useCallback(
    (): RecoveryOperation => ({
      requestKey,
      sessionKey,
      operationId: ++operationGenerationRef.current,
    }),
    [requestKey, sessionKey],
  );

  const isCurrentOperation = useCallback(
    (operation: RecoveryOperation) =>
      activeRequestKeyRef.current === operation.requestKey &&
      activeSessionKeyRef.current === operation.sessionKey &&
      operationGenerationRef.current === operation.operationId,
    [],
  );

  const isCurrentSession = useCallback(
    (operation: RecoveryOperation) => activeSessionKeyRef.current === operation.sessionKey,
    [],
  );

  const matchesLatestErrorStamp = useCallback(
    (stamp: string | undefined) => Boolean(stamp && latestErrorStampRef.current === stamp),
    [],
  );

  return { beginOperation, isCurrentOperation, isCurrentSession, matchesLatestErrorStamp };
}

/** Owns shared manual recovery state while a failed session remains visible. */
// eslint-disable-next-line max-lines-per-function -- the hook owns one coherent recovery state machine.
export function useSessionRecoveryActions({
  taskId,
  sessionId,
  errorStamp,
}: SessionRecoveryActionsOptions) {
  const { t } = useTranslation();
  const pendingKey = `${taskId}\u0000${sessionId}`;
  const sharedBusyAction = usePendingSessionRecovery(pendingKey);
  const sessionKey = pendingKey;
  const requestKey = `${taskId}\u0000${sessionId}\u0000${errorStamp ?? ""}`;
  const { beginOperation, isCurrentOperation, isCurrentSession, matchesLatestErrorStamp } =
    useRecoveryOperationFence(requestKey, sessionKey, errorStamp);
  const [busyAction, setBusyAction] = useState<SessionRecoveryBusyAction>(null);
  const [resumeError, setResumeError] = useState<Error | null>(null);
  const [restoreError, setRestoreError] = useState<Error | null>(null);
  const [branchDetails, setBranchDetails] = useState<BranchRecoveryDetails | null>(null);
  const [guardDetails, setGuardDetails] = useState<SessionRecoveryGuardDetails | null>(null);
  const [managedCloneRecoveryStamp, setManagedCloneRecoveryStamp] = useState<string | null>(null);
  const [lastFailedAction, setLastFailedAction] = useState<SessionRecoveryAction | null>(null);
  const [recoveryNotice, setRecoveryNotice] = useState<string | null>(null);
  const [manualRecoveryFailure, setManualRecoveryFailure] =
    useState<ManualSessionRecoveryFailure | null>(null);

  useEffect(() => {
    setBusyAction(null);
    setResumeError(null);
    setRestoreError(null);
    setBranchDetails(null);
    setGuardDetails(null);
    setManagedCloneRecoveryStamp(null);
    setLastFailedAction(null);
    setRecoveryNotice(null);
    setManualRecoveryFailure(null);
  }, [requestKey]);

  const recoveryError = combineRecoveryErrors(resumeError, restoreError, t);

  const handleRecoveryFailure = useCallback(
    (cause: unknown, operation: RecoveryOperation, action: SessionRecoveryAction) => {
      const guard = sessionRecoveryGuardDetails(cause);
      const managedClone = managedCloneRelocationRecoveryDetails(cause);
      const currentOperation = isCurrentOperation(operation);
      const currentRelocationRequirement =
        managedClone?.kind === "managed_clone_relocation_required" &&
        isCurrentSession(operation) &&
        (currentOperation || matchesLatestErrorStamp(managedClone.error_stamp));
      if (!currentOperation && !currentRelocationRequirement) return;
      if (managedClone?.kind === "managed_clone_relocation_required") {
        setManagedCloneRecoveryStamp(managedClone.error_stamp ?? errorStamp ?? null);
      } else if (managedClone?.kind === "managed_clone_relocation_stale") {
        setManagedCloneRecoveryStamp(null);
      }
      setResumeError(guardOrFallbackError(cause, guard, t, t("task:failedToResumeSession")));
      setRestoreError(null);
      setBranchDetails(guard ? null : branchRecoveryDetails(cause));
      setGuardDetails(guard);
      setLastFailedAction(action);
      setRecoveryNotice(null);
      setManualRecoveryFailure({ operation: "resume" });
    },
    [errorStamp, isCurrentOperation, isCurrentSession, matchesLatestErrorStamp, t],
  );

  const handleRecover = useCallback(
    async (action: SessionRecoveryAction) => {
      const release = claimSessionRecovery(pendingKey, action);
      if (!release) return false;
      const operation = beginOperation();
      setBusyAction(action);
      try {
        const recoveryStamp = managedCloneRecoveryStamp ?? errorStamp;
        if (action === "relocate_and_resume") {
          await requestSessionRecover(
            taskId,
            sessionId,
            action,
            t("task:failedToResumeSession"),
            recoveryStamp,
          );
        } else {
          await requestSessionRecover(taskId, sessionId, action, t("task:failedToResumeSession"));
        }
        if (!isCurrentOperation(operation)) return false;
        setResumeError(null);
        setRestoreError(null);
        setBranchDetails(null);
        setGuardDetails(null);
        setManagedCloneRecoveryStamp(null);
        setLastFailedAction(null);
        setRecoveryNotice(null);
        setManualRecoveryFailure(null);
      } catch (cause) {
        handleRecoveryFailure(cause, operation, action);
        return false;
      } finally {
        release();
        if (isCurrentOperation(operation)) setBusyAction(null);
      }
      return true;
    },
    [
      beginOperation,
      errorStamp,
      handleRecoveryFailure,
      isCurrentOperation,
      managedCloneRecoveryStamp,
      pendingKey,
      sessionId,
      taskId,
      t,
    ],
  );

  const handleRestore = useCallback(async () => {
    const release = claimSessionRecovery(pendingKey, "restore");
    if (!release) return;
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
      setManagedCloneRecoveryStamp(null);
      setLastFailedAction(null);
      setRecoveryNotice(t("task:resumeFailedWorkspaceReadOnly"));
      setManualRecoveryFailure(null);
    } catch (cause) {
      if (!isCurrentOperation(operation)) return;
      const guard = sessionRecoveryGuardDetails(cause);
      setRestoreError(guardOrFallbackError(cause, guard, t, t("task:failedToRestoreWorkspace")));
      setGuardDetails(guard ?? guardDetails);
      setRecoveryNotice(null);
      setManualRecoveryFailure({ operation: "restore_workspace" });
    } finally {
      release();
      if (isCurrentOperation(operation)) setBusyAction(null);
    }
  }, [beginOperation, guardDetails, isCurrentOperation, pendingKey, sessionId, taskId, t]);

  const handleRetry = useCallback(() => {
    return handleRecover(lastFailedAction ?? "resume");
  }, [handleRecover, lastFailedAction]);

  const handleNewBranch = useCallback(() => {
    return handleRecover("resume_new_branch");
  }, [handleRecover]);

  const handleManagedCloneRelocation = useCallback(() => {
    return handleRecover("relocate_and_resume");
  }, [handleRecover]);

  return {
    busyAction: sharedBusyAction ?? busyAction,
    recoveryError,
    branchDetails,
    guardDetails,
    managedCloneRecoveryStamp,
    lastFailedAction,
    recoveryNotice,
    manualRecoveryFailure,
    handleRecover,
    handleRestore,
    handleRetry,
    handleNewBranch,
    handleManagedCloneRelocation,
  };
}

export type SessionRecoveryActions = ReturnType<typeof useSessionRecoveryActions>;
