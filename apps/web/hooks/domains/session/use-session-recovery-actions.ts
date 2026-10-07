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
  getWorkspaceRecoveryStatus,
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
export type WorkspaceRecoveryStatusCheck = "idle" | "checking" | "unresolved";

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
type WorkspaceRecoveryStatusRead = {
  resolved: boolean;
  projection: import("@/lib/types/http").WorkspaceRecoveryProjection | null;
};

type WorkspaceRecoveryProjection = import("@/lib/types/http").WorkspaceRecoveryProjection;

function compareDecimalIdentity(left: string, right: string): number | null {
  if (!/^\d+$/.test(left) || !/^\d+$/.test(right)) return left === right ? 0 : null;
  const normalizedLeft = left.replace(/^0+(?=\d)/, "");
  const normalizedRight = right.replace(/^0+(?=\d)/, "");
  if (normalizedLeft.length !== normalizedRight.length)
    return normalizedLeft.length < normalizedRight.length ? -1 : 1;
  if (normalizedLeft === normalizedRight) return 0;
  return normalizedLeft < normalizedRight ? -1 : 1;
}

function isOlderWorkspaceRecoveryProjection(
  incoming: WorkspaceRecoveryProjection,
  current: WorkspaceRecoveryProjection | null,
): boolean {
  if (!current || incoming.environment_id !== current.environment_id) return false;
  const generationOrder = compareDecimalIdentity(
    incoming.ownership_generation,
    current.ownership_generation,
  );
  if (generationOrder !== null && generationOrder !== 0) return generationOrder < 0;
  if (incoming.ownership_generation !== current.ownership_generation) return true;
  const revisionOrder = compareDecimalIdentity(incoming.revision, current.revision);
  const sameAttempt =
    incoming.operation_id === current.operation_id && incoming.attempt_id === current.attempt_id;
  if (!sameAttempt) return revisionOrder === null || revisionOrder <= 0;
  return revisionOrder !== null && revisionOrder < 0;
}

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
  const taskEnvironmentId = useAppStore(
    (state) =>
      state.taskSessions.items[sessionId]?.task_environment_id ??
      state.taskSessions.items[sessionId]?.environment_id,
  );
  const workspaceRecovery = useAppStore((state) => {
    const session = state.taskSessions.items[sessionId];
    const projection = session?.workspace_recovery;
    const selectedEnvironmentId = session?.task_environment_id ?? session?.environment_id;
    if (
      !session ||
      session.task_id !== taskId ||
      projection?.task_id !== taskId ||
      !selectedEnvironmentId ||
      projection.environment_id !== selectedEnvironmentId
    ) {
      return null;
    }
    return projection;
  });
  const workspaceRecoveryRepositoryName = useAppStore((state) => {
    const repositoryId = state.taskSessions.items[sessionId]?.workspace_recovery?.repository_id;
    if (!repositoryId) return null;
    return (
      Object.values(state.repositories?.itemsByWorkspaceId ?? {})
        .flat()
        .find((repository) => repository.id === repositoryId)?.name ?? null
    );
  });
  const setWorkspaceRecoveryProjection = useAppStore(
    (state) => state.setWorkspaceRecoveryProjection,
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
  const [workspaceRecoveryStatusCheck, setWorkspaceRecoveryStatusCheck] =
    useState<WorkspaceRecoveryStatusCheck>("idle");

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
    setWorkspaceRecoveryStatusCheck("idle");
  }, [requestKey]);

  const localResultIsCurrent = localResultRequestKey === requestKey;
  const recoveryError = localResultIsCurrent
    ? combineRecoveryErrors(resumeError, restoreError, t)
    : null;

  const reconcileWorkspaceRecoveryStatus = useCallback(
    async (operation: RecoveryOperation): Promise<WorkspaceRecoveryStatusRead> => {
      setWorkspaceRecoveryStatusCheck("checking");
      try {
        const projection = await getWorkspaceRecoveryStatus(
          taskId,
          sessionId,
          t("task:workspaceRecoveryStatusUnavailable"),
        );
        if (!isCurrentOperation(operation)) return { resolved: false, projection: null };
        const matchesBinding =
          projection?.task_id === taskId &&
          Boolean(projection.session_id) &&
          Boolean(taskEnvironmentId) &&
          projection.environment_id === taskEnvironmentId;
        if (
          projection &&
          (!matchesBinding || isOlderWorkspaceRecoveryProjection(projection, workspaceRecovery))
        ) {
          setWorkspaceRecoveryStatusCheck("unresolved");
          return { resolved: false, projection: null };
        }
        const current = matchesBinding ? projection : null;
        if (current) setWorkspaceRecoveryProjection([sessionId], current);
        setWorkspaceRecoveryStatusCheck("idle");
        return { resolved: true, projection: current };
      } catch {
        if (isCurrentOperation(operation)) setWorkspaceRecoveryStatusCheck("unresolved");
        return { resolved: false, projection: null };
      }
    },
    [
      isCurrentOperation,
      sessionId,
      setWorkspaceRecoveryProjection,
      t,
      taskEnvironmentId,
      taskId,
      workspaceRecovery,
    ],
  );

  useEffect(() => {
    if (workspaceRecovery) setWorkspaceRecoveryStatusCheck("idle");
  }, [workspaceRecovery]);

  const checkWorkspaceRecoveryStatus = useCallback(() => {
    const operation = beginOperation();
    return reconcileWorkspaceRecoveryStatus(operation);
  }, [beginOperation, reconcileWorkspaceRecoveryStatus]);

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

  const requestRecovery = useCallback(
    (action: SessionRecoveryAction) => {
      const request = {
        taskId,
        sessionId,
        action,
        failureMessage: t(FAILED_TO_RESUME_MESSAGE_KEY),
      };
      if (action === "relocate_and_resume") {
        return requestSessionRecover({
          ...request,
          errorStamp: managedCloneRecoveryStamp ?? errorStamp,
        });
      }
      if (action === "resume" && providerRestoredResumeEligible) {
        return requestSessionRecover({ ...request, settingsPolicy: "provider_restored" });
      }
      return requestSessionRecover(request);
    },
    [errorStamp, managedCloneRecoveryStamp, providerRestoredResumeEligible, sessionId, taskId, t],
  );

  const clearRecoveryResult = useCallback((operation: RecoveryOperation) => {
    setLocalResultRequestKey(operation.requestKey);
    setResumeError(null);
    setRestoreError(null);
    setBranchDetails(null);
    setGuardDetails(null);
    setManagedCloneRecoveryStamp(null);
    setLastFailedAction(null);
    setRecoveryNotice(null);
    setManualRecoveryFailure(null);
  }, []);

  const reconcileFailedRelocation = useCallback(
    async (operation: RecoveryOperation): Promise<boolean> => {
      const status = await reconcileWorkspaceRecoveryStatus(operation);
      if (!status.resolved) return true;
      const projection = status.projection;
      const currentErrorStamp = managedCloneRecoveryStamp ?? errorStamp;
      if (
        !projection ||
        projection.session_id !== sessionId ||
        !currentErrorStamp ||
        projection.error_stamp !== currentErrorStamp ||
        (!projection.runner_live &&
          projection.state !== "interrupted" &&
          !projection.workspace_complete)
      ) {
        return false;
      }
      if (isCurrentOperation(operation)) clearRecoveryResult(operation);
      return true;
    },
    [
      clearRecoveryResult,
      errorStamp,
      isCurrentOperation,
      managedCloneRecoveryStamp,
      reconcileWorkspaceRecoveryStatus,
      sessionId,
    ],
  );

  const handleRecover = useCallback(
    async (action: SessionRecoveryAction) => {
      const release = claimSessionRecovery(pendingKey, action);
      if (!release) return false;
      const operation = beginOperation();
      setWorkspaceRecoveryStatusCheck("idle");
      setBusyAction(action);
      try {
        await requestRecovery(action);
        if (!isCurrentOperation(operation)) return false;
        clearRecoveryResult(operation);
      } catch (cause) {
        if (action === "relocate_and_resume" && (await reconcileFailedRelocation(operation)))
          return false;
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
      clearRecoveryResult,
      handleRecoveryFailure,
      isCurrentOperation,
      pendingKey,
      reconcileFailedRelocation,
      requestRecovery,
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
      setWorkspaceRecoveryStatusCheck("idle");
      setResumeError(null);
      setRestoreError(null);
      setBranchDetails(null);
      setGuardDetails(null);
      setManagedCloneRecoveryStamp(null);
      setLastFailedAction(null);
      setRecoveryNotice(t("task:resumeFailedWorkspaceReadOnly"));
      setManualRecoveryFailure(null);
    } catch (cause) {
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
      const { managedClone, errorStamp: associatedStamp, requestKey } = association;
      setLocalResultRequestKey(requestKey);
      const guard = sessionRecoveryGuardDetails(cause);
      if (managedClone?.kind === "managed_clone_relocation_required") {
        setManagedCloneRecoveryStamp(managedClone.error_stamp ?? null);
      } else if (managedClone?.kind === "managed_clone_relocation_stale") {
        setManagedCloneRecoveryStamp(null);
      }
      setRestoreError(guardOrFallbackError(cause, guard, t, t("task:failedToRestoreWorkspace")));
      setGuardDetails(guard ?? guardDetails);
      setRecoveryNotice(null);
      setManualRecoveryFailure({
        operation: "restore_workspace",
        sessionId,
        errorStamp: associatedStamp,
        requestKey,
        operationId: operation.operationId,
      });
    } finally {
      release();
      if (isCurrentOperation(operation)) setBusyAction(null);
    }
  }, [
    beginOperation,
    errorStamp,
    guardDetails,
    isCurrentOperation,
    isCurrentSession,
    matchesLatestErrorStamp,
    pendingKey,
    sessionId,
    taskId,
    t,
  ]);

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
    workspaceRecovery,
    workspaceRecoveryMatchesCurrentFailure: Boolean(
      workspaceRecovery?.session_id === sessionId &&
      errorStamp &&
      workspaceRecovery.error_stamp === errorStamp &&
      !workspaceRecovery.workspace_complete &&
      !workspaceRecovery.agent_ready,
    ),
    workspaceRecoveryRepositoryName,
    workspaceRecoveryStatusCheck,
    checkWorkspaceRecoveryStatus,
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

export type SessionRecoveryActions = Omit<
  ReturnType<typeof useSessionRecoveryActions>,
  "workspaceRecoveryMatchesCurrentFailure"
> & { workspaceRecoveryMatchesCurrentFailure?: boolean };
