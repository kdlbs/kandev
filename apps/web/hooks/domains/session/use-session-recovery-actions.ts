import { claimSessionRecovery, usePendingSessionRecovery } from "./session-recovery-pending";
import { useCallback, useEffect, useRef, useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
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
  sessionId: string;
  errorStamp: string | null;
  requestKey: string;
  operationId: number;
};

const FAILED_TO_RESUME_MESSAGE_KEY = "task:failedToResumeSession";

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

type RecoveryFailureAssociation = {
  managedClone: ReturnType<typeof managedCloneRelocationRecoveryDetails>;
  errorStamp: string | null;
  requestKey: string;
};

function recoveryFailureAssociation({
  cause,
  operation,
  errorStamp,
  taskId,
  sessionId,
  isCurrentOperation,
  isCurrentSession,
  matchesLatestErrorStamp,
}: {
  cause: unknown;
  operation: RecoveryOperation;
  errorStamp?: string | null;
  taskId: string;
  sessionId: string;
  isCurrentOperation: (operation: RecoveryOperation) => boolean;
  isCurrentSession: (operation: RecoveryOperation) => boolean;
  matchesLatestErrorStamp: (stamp: string | undefined) => boolean;
}): RecoveryFailureAssociation | null {
  const managedClone = managedCloneRelocationRecoveryDetails(cause);
  const currentOperation = isCurrentOperation(operation);
  const isSuccessorRelocation =
    managedClone?.kind === "managed_clone_relocation_required" &&
    isCurrentSession(operation) &&
    matchesLatestErrorStamp(managedClone.error_stamp);
  if (!currentOperation && !isSuccessorRelocation) return null;

  const associatedStamp = isSuccessorRelocation
    ? (managedClone.error_stamp ?? errorStamp ?? null)
    : (errorStamp ?? null);
  const requestKey = currentOperation
    ? operation.requestKey
    : `${taskId}\u0000${sessionId}\u0000${associatedStamp ?? ""}`;
  return { managedClone, errorStamp: associatedStamp, requestKey };
}

function hasNativeACPSessionToken(session: TaskSession): boolean {
  if (
    typeof session.downstream_acp_session_id === "string" &&
    session.downstream_acp_session_id.trim()
  ) {
    return true;
  }
  const acp = session.metadata?.acp;
  if (!acp || typeof acp !== "object" || Array.isArray(acp)) return false;
  const nativeSessionId = (acp as Record<string, unknown>).session_id;
  return typeof nativeSessionId === "string" && nativeSessionId.trim().length > 0;
}

function isProviderRestoredResumeEligible(
  state: AppState,
  taskId: string,
  sessionId: string,
): boolean {
  const session = state.taskSessions.items[sessionId];
  if (
    !session ||
    session.task_id !== taskId ||
    session.state !== "FAILED" ||
    session.is_passthrough ||
    !hasNativeACPSessionToken(session)
  ) {
    return false;
  }

  const task = state.kanban.tasks.find((candidate) => candidate.id === taskId);
  if (task?.isFromOffice) return false;
  const quickChatOwnsTaskSession = state.quickChat.sessions.some(
    (candidate) =>
      candidate.kind === "chat" && candidate.sessionId === sessionId && candidate.taskId === taskId,
  );
  if (!task && !quickChatOwnsTaskSession) return false;

  const profileId = session.execution_profile_id || session.agent_profile_id;
  if (!profileId) return false;
  const profile = state.agentProfiles.items.find((candidate) => candidate.id === profileId);
  return !!profile && !profile.cli_passthrough && profile.agent_name.toLowerCase() === "auggie";
}

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
  const providerRestoredResumeEligible = useAppStore((state) =>
    isProviderRestoredResumeEligible(state, taskId, sessionId),
  );
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
  const [localResultRequestKey, setLocalResultRequestKey] = useState<string | null>(null);

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
    setLocalResultRequestKey(null);
  }, [requestKey]);

  const localResultIsCurrent = localResultRequestKey === requestKey;
  const recoveryError = localResultIsCurrent
    ? combineRecoveryErrors(resumeError, restoreError, t)
    : null;

  const handleRecoveryFailure = useCallback(
    (cause: unknown, operation: RecoveryOperation, action: SessionRecoveryAction) => {
      const association = recoveryFailureAssociation({
        cause,
        operation,
        errorStamp,
        taskId,
        sessionId,
        isCurrentOperation,
        isCurrentSession,
        matchesLatestErrorStamp,
      });
      if (!association) return;
      const guard = sessionRecoveryGuardDetails(cause);
      const { managedClone, errorStamp: associatedStamp, requestKey } = association;
      setLocalResultRequestKey(requestKey);
      if (managedClone?.kind === "managed_clone_relocation_required") {
        setManagedCloneRecoveryStamp(managedClone.error_stamp ?? errorStamp ?? null);
      } else if (managedClone?.kind === "managed_clone_relocation_stale") {
        setManagedCloneRecoveryStamp(null);
      }
      setResumeError(guardOrFallbackError(cause, guard, t, t(FAILED_TO_RESUME_MESSAGE_KEY)));
      setRestoreError(null);
      setBranchDetails(guard ? null : branchRecoveryDetails(cause));
      setGuardDetails(guard);
      setLastFailedAction(action);
      setRecoveryNotice(null);
      setManualRecoveryFailure({
        operation: "resume",
        sessionId,
        errorStamp: associatedStamp,
        requestKey,
        operationId: operation.operationId,
      });
    },
    [
      errorStamp,
      isCurrentOperation,
      isCurrentSession,
      matchesLatestErrorStamp,
      sessionId,
      taskId,
      t,
    ],
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
          await requestSessionRecover({
            taskId,
            sessionId,
            action,
            failureMessage: t(FAILED_TO_RESUME_MESSAGE_KEY),
            errorStamp: recoveryStamp,
          });
        } else if (action === "resume" && providerRestoredResumeEligible) {
          await requestSessionRecover({
            taskId,
            sessionId,
            action,
            failureMessage: t(FAILED_TO_RESUME_MESSAGE_KEY),
            settingsPolicy: "provider_restored",
          });
        } else {
          await requestSessionRecover({
            taskId,
            sessionId,
            action,
            failureMessage: t(FAILED_TO_RESUME_MESSAGE_KEY),
          });
        }
        if (!isCurrentOperation(operation)) return false;
        setLocalResultRequestKey(operation.requestKey);
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
      providerRestoredResumeEligible,
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
      setLocalResultRequestKey(operation.requestKey);
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
      setLocalResultRequestKey(operation.requestKey);
      const guard = sessionRecoveryGuardDetails(cause);
      setRestoreError(guardOrFallbackError(cause, guard, t, t("task:failedToRestoreWorkspace")));
      setGuardDetails(guard ?? guardDetails);
      setRecoveryNotice(null);
      setManualRecoveryFailure({
        operation: "restore_workspace",
        sessionId,
        errorStamp: errorStamp ?? null,
        requestKey: operation.requestKey,
        operationId: operation.operationId,
      });
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
    branchDetails: localResultIsCurrent ? branchDetails : null,
    guardDetails: localResultIsCurrent ? guardDetails : null,
    managedCloneRecoveryStamp: localResultIsCurrent ? managedCloneRecoveryStamp : null,
    lastFailedAction: localResultIsCurrent ? lastFailedAction : null,
    recoveryNotice: localResultIsCurrent ? recoveryNotice : null,
    manualRecoveryFailure: localResultIsCurrent ? manualRecoveryFailure : null,
    providerRestoredResumeEligible,
    handleRecover,
    handleRestore,
    handleRetry,
    handleNewBranch,
    handleManagedCloneRelocation,
  };
}

export type SessionRecoveryActions = ReturnType<typeof useSessionRecoveryActions>;
