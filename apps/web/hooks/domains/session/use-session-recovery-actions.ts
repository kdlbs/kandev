import { claimSessionRecovery, usePendingSessionRecovery } from "./session-recovery-pending";
import { useCallback, useEffect, useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { readAgentDeliveryRecovery } from "@/lib/session-agent-delivery-recovery";
import {
  asRecoveryError,
  branchRecoveryDetails,
  contextContinuationDetails,
  getWorkspaceRecoveryStatus,
  managedCloneRelocationRecoveryDetails,
  recoveryInspectionBusyDetails,
  recoveryInspectionBusyMessage,
  requestSessionRecover,
  restoreSessionWorkspace,
  sessionDeliveryRecoveryMessage,
  sessionRecoveryGuardDetails,
  sessionRecoveryGuardMessage,
  type BranchRecoveryDetails,
  type ContextContinuationDetails,
  type SessionRecoveryAction,
  type SessionDeliveryRecoveryResponse,
  type SessionRecoveryGuardDetails,
} from "@/lib/services/session-recovery-service";
import type { SessionRecoveryNoticeKind } from "./use-session-resumption";
import {
  useRecoveryOperationFence,
  currentRecoveryValue,
  isOlderWorkspaceRecoveryProjection,
  workspaceRecoveryMatchesFailure,
  type RecoveryOperation,
  type SessionRecoveryBusyAction,
  type WorkspaceRecoveryStatusCheck,
  type ManualSessionRecoveryFailure,
} from "./session-recovery-operation-fence";

export type {
  SessionRecoveryBusyAction,
  WorkspaceRecoveryStatusCheck,
  ManualSessionRecoveryFailure,
} from "./session-recovery-operation-fence";
const FAILED_TO_RESUME_MESSAGE_KEY = "task:failedToResumeSession";

type SessionRecoveryActionsOptions = {
  taskId: string;
  sessionId: string;
  errorStamp?: string | null;
  onDeliveryReconciled?: () => void;
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

type WorkspaceRecoveryStatusRead = {
  resolved: boolean;
  projection: import("@/lib/types/http").WorkspaceRecoveryProjection | null;
};

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

/** Owns shared manual recovery state while a failed session remains visible. */
// eslint-disable-next-line max-lines-per-function -- the hook owns one coherent recovery state machine.
export function useSessionRecoveryActions({
  taskId,
  sessionId,
  errorStamp,
  onDeliveryReconciled,
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
  const recoveryRevision = useAppStore(
    (state) => readAgentDeliveryRecovery(state.taskSessions.items[sessionId]?.metadata)?.revision,
  );
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
  const [continuationDetails, setContinuationDetails] = useState<ContextContinuationDetails | null>(
    null,
  );
  const [managedCloneRecoveryStamp, setManagedCloneRecoveryStamp] = useState<string | null>(null);
  const [lastFailedAction, setLastFailedAction] = useState<SessionRecoveryAction | null>(null);
  const [recoveryNotice, setRecoveryNotice] = useState<string | null>(null);
  const [deliveryRecoveryResult, setDeliveryRecoveryResult] =
    useState<SessionDeliveryRecoveryResponse | null>(null);
  const [deliveryRecoveryNotice, setDeliveryRecoveryNotice] = useState<string | null>(null);
  const [recoveryNoticeKind, setRecoveryNoticeKind] = useState<SessionRecoveryNoticeKind | null>(
    null,
  );
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
    setContinuationDetails(null);
    setManagedCloneRecoveryStamp(null);
    setLastFailedAction(null);
    setRecoveryNotice(null);
    setDeliveryRecoveryNotice(null);
    setDeliveryRecoveryResult(null);
    setRecoveryNoticeKind(null);
    setManualRecoveryFailure(null);
    setLocalResultRequestKey(null);
    setWorkspaceRecoveryStatusCheck("idle");
  }, [requestKey]);

  const localResultIsCurrent = localResultRequestKey === requestKey;
  const deliveryResultIsCurrent =
    localResultIsCurrent &&
    (!deliveryRecoveryResult?.allowed_actions?.length ||
      recoveryRevision === undefined ||
      deliveryRecoveryResult.recovery_revision === recoveryRevision);
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
      if (recoveryInspectionBusyDetails(cause)) {
        setLocalResultRequestKey(association.requestKey);
        setResumeError(null);
        setRestoreError(null);
        setBranchDetails(null);
        setGuardDetails(null);
        setLastFailedAction("resume");
        setRecoveryNotice(recoveryInspectionBusyMessage(t));
        setRecoveryNoticeKind("inspection_busy");
        setManualRecoveryFailure(null);
        return;
      }
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
      setContinuationDetails(contextContinuationDetails(cause));
      setLastFailedAction(action);
      setRecoveryNotice(null);
      setRecoveryNoticeKind(null);
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
    setContinuationDetails(null);
    setManagedCloneRecoveryStamp(null);
    setLastFailedAction(null);
    setRecoveryNotice(null);
    setRecoveryNoticeKind(null);
    setManualRecoveryFailure(null);
  }, []);

  const clearInspectionContentionNotice = useCallback(() => {
    if (recoveryNoticeKind !== "inspection_busy") return;
    setLocalResultRequestKey(requestKey);
    setRecoveryNotice(null);
    setRecoveryNoticeKind(null);
    setLastFailedAction(null);
  }, [recoveryNoticeKind, requestKey]);

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
        const result = await requestRecovery(action);
        if (!isCurrentOperation(operation)) return false;
        if (action === "retry_connection" && result) {
          setLocalResultRequestKey(operation.requestKey);
          setDeliveryRecoveryNotice(sessionDeliveryRecoveryMessage(result, t));
          setDeliveryRecoveryResult(result);
          onDeliveryReconciled?.();
        } else {
          setDeliveryRecoveryNotice(null);
          setDeliveryRecoveryResult(null);
          clearRecoveryResult(operation);
        }
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
      onDeliveryReconciled,
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
      setWorkspaceRecoveryStatusCheck("idle");
      setResumeError(null);
      setRestoreError(null);
      setBranchDetails(null);
      setGuardDetails(null);
      setContinuationDetails(null);
      setManagedCloneRecoveryStamp(null);
      setLastFailedAction(null);
      setRecoveryNotice(t("task:resumeFailedWorkspaceReadOnly"));
      setRecoveryNoticeKind("workspace_read_only");
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
      if (recoveryInspectionBusyDetails(cause)) {
        setResumeError(null);
        setRestoreError(null);
        setBranchDetails(null);
        setGuardDetails(null);
        setLastFailedAction("resume");
        setRecoveryNotice(recoveryInspectionBusyMessage(t));
        setRecoveryNoticeKind("inspection_busy");
        setManualRecoveryFailure(null);
        return;
      }
      const guard = sessionRecoveryGuardDetails(cause);
      if (managedClone?.kind === "managed_clone_relocation_required") {
        setManagedCloneRecoveryStamp(managedClone.error_stamp ?? null);
      } else if (managedClone?.kind === "managed_clone_relocation_stale") {
        setManagedCloneRecoveryStamp(null);
      }
      setRestoreError(guardOrFallbackError(cause, guard, t, t("task:failedToRestoreWorkspace")));
      setGuardDetails(guard ?? guardDetails);
      setRecoveryNotice(null);
      setRecoveryNoticeKind(null);
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

  const handleNewBranch = useCallback(() => handleRecover("resume_new_branch"), [handleRecover]);

  const handleManagedCloneRelocation = useCallback(() => {
    return handleRecover("relocate_and_resume");
  }, [handleRecover]);

  const handleContinueFromHistory = useCallback(
    () => handleRecover("continue_from_history"),
    [handleRecover],
  );

  return {
    busyAction: sharedBusyAction ?? busyAction,
    recoveryError,
    branchDetails: currentRecoveryValue(localResultIsCurrent, branchDetails),
    guardDetails: currentRecoveryValue(localResultIsCurrent, guardDetails),
    managedCloneRecoveryStamp: currentRecoveryValue(
      localResultIsCurrent,
      managedCloneRecoveryStamp,
    ),
    continuationDetails: currentRecoveryValue(localResultIsCurrent, continuationDetails),
    workspaceRecovery,
    workspaceRecoveryMatchesCurrentFailure: workspaceRecoveryMatchesFailure(
      workspaceRecovery,
      sessionId,
      errorStamp,
    ),
    workspaceRecoveryRepositoryName,
    workspaceRecoveryStatusCheck,
    checkWorkspaceRecoveryStatus,
    lastFailedAction: currentRecoveryValue(localResultIsCurrent, lastFailedAction),
    recoveryNotice: currentRecoveryValue(localResultIsCurrent, recoveryNotice),
    deliveryRecoveryNotice: currentRecoveryValue(deliveryResultIsCurrent, deliveryRecoveryNotice),
    deliveryRecoveryResult: currentRecoveryValue(deliveryResultIsCurrent, deliveryRecoveryResult),
    recoveryNoticeKind: currentRecoveryValue(localResultIsCurrent, recoveryNoticeKind),
    clearInspectionContentionNotice,
    manualRecoveryFailure: currentRecoveryValue(localResultIsCurrent, manualRecoveryFailure),
    providerRestoredResumeEligible,
    handleRecover,
    handleRestore,
    handleRetry,
    handleNewBranch,
    handleManagedCloneRelocation,
    handleContinueFromHistory,
  };
}

export type { SessionRecoveryActions } from "./session-recovery-operation-fence";
