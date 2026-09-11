import { useCallback, useEffect, useRef, useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import {
  asRecoveryError,
  branchRecoveryDetails,
  contextContinuationDetails,
  requestSessionRecover,
  restoreSessionWorkspace,
  type BranchRecoveryDetails,
  type ContextContinuationDetails,
  type SessionRecoveryAction,
} from "@/lib/services/session-recovery-service";

export type SessionRecoveryBusyAction = SessionRecoveryAction | "restore" | null;

type SessionRecoveryActionsOptions = {
  taskId: string;
  sessionId: string;
};

type RecoveryViewState = {
  busyAction: SessionRecoveryBusyAction;
  resumeError: Error | null;
  restoreError: Error | null;
  branchDetails: BranchRecoveryDetails | null;
  continuationDetails: ContextContinuationDetails | null;
  lastFailedAction: SessionRecoveryAction | null;
  recoveryNotice: string | null;
};

function createInitialRecoveryState(): RecoveryViewState {
  return {
    busyAction: null,
    resumeError: null,
    restoreError: null,
    branchDetails: null,
    continuationDetails: null,
    lastFailedAction: null,
    recoveryNotice: null,
  };
}

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
export function useSessionRecoveryActions({ taskId, sessionId }: SessionRecoveryActionsOptions) {
  const { t } = useTranslation();
  const requestKey = `${taskId}\u0000${sessionId}`;
  const activeRequestKeyRef = useRef(requestKey);
  const operationGenerationRef = useRef(0);
  if (activeRequestKeyRef.current !== requestKey) {
    activeRequestKeyRef.current = requestKey;
    operationGenerationRef.current += 1;
  }
  const [state, setState] = useState<RecoveryViewState>(createInitialRecoveryState);

  useEffect(() => {
    setState(createInitialRecoveryState);
  }, [requestKey]);

  const beginOperation = useCallback(
    (action: SessionRecoveryBusyAction) => {
      const operationId = ++operationGenerationRef.current;
      setState((current) => ({ ...current, busyAction: action }));
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

  const recoveryError = combineRecoveryErrors(state.resumeError, state.restoreError, t);

  const handleRecover = useCallback(
    async (action: SessionRecoveryAction) => {
      const operation = beginOperation(action);
      try {
        await requestSessionRecover(taskId, sessionId, action, t("task:failedToResumeSession"));
        if (!isCurrentOperation(operation)) return false;
        setState(createInitialRecoveryState);
      } catch (cause) {
        if (!isCurrentOperation(operation)) return false;
        setState({
          ...createInitialRecoveryState(),
          resumeError: asRecoveryError(cause, t("task:failedToResumeSession")),
          branchDetails: branchRecoveryDetails(cause),
          continuationDetails: contextContinuationDetails(cause),
          lastFailedAction: action,
        });
        return false;
      } finally {
        if (isCurrentOperation(operation))
          setState((current) => ({ ...current, busyAction: null }));
      }
      return true;
    },
    [beginOperation, isCurrentOperation, sessionId, taskId, t],
  );

  const handleRestore = useCallback(async () => {
    const operation = beginOperation("restore");
    setState((current) => ({ ...current, restoreError: null }));
    try {
      await restoreSessionWorkspace(taskId, sessionId, t("task:failedToRestoreWorkspace"));
      if (!isCurrentOperation(operation)) return;
      setState({
        ...createInitialRecoveryState(),
        recoveryNotice: t("task:resumeFailedWorkspaceReadOnly"),
      });
    } catch (cause) {
      if (!isCurrentOperation(operation)) return;
      setState({
        ...createInitialRecoveryState(),
        restoreError: asRecoveryError(cause, t("task:failedToRestoreWorkspace")),
      });
    } finally {
      if (isCurrentOperation(operation)) setState((current) => ({ ...current, busyAction: null }));
    }
  }, [beginOperation, isCurrentOperation, sessionId, taskId, t]);

  const handleRetry = useCallback(() => {
    return handleRecover(state.lastFailedAction ?? "resume");
  }, [handleRecover, state.lastFailedAction]);

  const handleNewBranch = useCallback(() => {
    return handleRecover("resume_new_branch");
  }, [handleRecover]);

  const handleContinueFromHistory = useCallback(() => {
    return handleRecover("continue_from_history");
  }, [handleRecover]);

  return {
    busyAction: state.busyAction,
    recoveryError,
    branchDetails: state.branchDetails,
    continuationDetails: state.continuationDetails,
    recoveryNotice: state.recoveryNotice,
    handleRecover,
    handleRestore,
    handleRetry,
    handleNewBranch,
    handleContinueFromHistory,
  };
}

export type SessionRecoveryActions = ReturnType<typeof useSessionRecoveryActions>;
