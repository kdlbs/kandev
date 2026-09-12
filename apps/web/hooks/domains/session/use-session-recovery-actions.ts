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

export type ManualSessionRecoveryFailure = {
  operation: "resume" | "restore_workspace";
};

type SessionRecoveryActionsOptions = {
  taskId: string;
  sessionId: string;
  errorStamp?: string | null;
};

type RecoveryViewState = {
  busyAction: SessionRecoveryBusyAction;
  resumeError: Error | null;
  restoreError: Error | null;
  branchDetails: BranchRecoveryDetails | null;
  continuationDetails: ContextContinuationDetails | null;
  lastFailedAction: SessionRecoveryAction | null;
  recoveryNotice: string | null;
  manualRecoveryFailure: ManualSessionRecoveryFailure | null;
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
    manualRecoveryFailure: null,
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
          manualRecoveryFailure: { operation: "resume" },
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
      setState((current) => ({
        ...current,
        recoveryNotice: null,
        restoreError: asRecoveryError(cause, t("task:failedToRestoreWorkspace")),
        manualRecoveryFailure: { operation: "restore_workspace" },
      }));
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
    manualRecoveryFailure: state.manualRecoveryFailure,
    handleRecover,
    handleRestore,
    handleRetry,
    handleNewBranch,
    handleContinueFromHistory,
  };
}

export type SessionRecoveryActions = ReturnType<typeof useSessionRecoveryActions>;
