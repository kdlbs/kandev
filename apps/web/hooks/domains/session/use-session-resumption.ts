/* eslint-disable max-lines -- session resumption coordinates one guarded lifecycle. */

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  isWebSocketRequestTimeoutError,
  SESSION_ENTRY_REQUEST_TIMEOUT_MS,
  SESSION_ENTRY_RETRY_DELAY_MS,
} from "@/lib/ws/client";
import { getWebSocketClient } from "@/lib/ws/connection";
import { launchSession } from "@/lib/services/session-launch-service";
import {
  buildResumeRequest,
  buildRestoreWorkspaceRequest,
} from "@/lib/services/session-launch-helpers";
import { useSessionRecoveryFeedback } from "./use-session-recovery-feedback";
import {
  clearArchiveRecovery,
  decideResumeAction,
  isTaskArchivedConflict,
  markSessionStarting,
  resumeViaLaunch,
  resumeWithSilentFallback,
  TASK_ARCHIVED_KIND,
} from "./use-session-resumption-operations";
import type {
  ResumptionState,
  ResumeStateSetter,
  SessionLike,
  SessionRecoveryFailure,
  SessionStatus,
  TaskArchiveState,
} from "./use-session-resumption-operations";
import {
  buildGuardedSetters,
  isCurrentRequest,
  type SessionRequestIdentity,
} from "./use-session-resumption-request-guard";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type TaskSessionState,
} from "@/lib/types/http";
import { t } from "@/lib/i18n";

export type {
  ResumptionState,
  ResumeStateSetter,
  ResumeStartingProjection,
  SessionLike,
  SessionRecoveryFailure,
  SessionStatus,
  TaskArchiveState,
} from "./use-session-resumption-operations";
export { markSessionStarting, resumeWithSilentFallback } from "./use-session-resumption-operations";
type CheckAndResumeParams = {
  taskId: string;
  sessionId: string;
  session: SessionLike;
  setSessionStatus: (s: SessionStatus) => void;
  setters: ResumeStateSetter;
  /** True when the prevent-auto-start-on-open preference gates open-time resumes. */
  preventAutoStart: boolean;
  taskArchiveState: TaskArchiveState;
  canContinue: () => boolean;
};

const TERMINAL_STATES = new Set<TaskSessionState>(["FAILED", "CANCELLED", "COMPLETED"]);
const MAX_SESSION_STATUS_ATTEMPTS = 2;

function sessionStatusFailure(
  error: unknown,
): Extract<SessionRecoveryFailure, { outcome: "status_unavailable" }> {
  return {
    outcome: "status_unavailable",
    kind: isWebSocketRequestTimeoutError(error) ? "timeout" : "request",
    statusError: error instanceof Error && error.message ? error.message : t("common:unknownError"),
  };
}

/** Apply permanent outcomes returned inside an otherwise successful status response. */
function applyStatusResponseOutcome(status: SessionStatus, setters: ResumeStateSetter): boolean {
  if (status.error) {
    setters.setRecoveryFailure?.(null);
    setters.setResumptionState("error");
    setters.setError(status.error);
    setters.setNotice?.(null);
    return true;
  }
  if (status.resume_reason === TASK_ARCHIVED_KIND) {
    clearArchiveRecovery(setters);
    return true;
  }
  return false;
}

type LiveSessionLike = (SessionLike & { state?: string }) | null;

/**
 * Monotonic status-hydration guard: a `task.session.status` response must not
 * downgrade a live STARTING/RUNNING/WAITING_FOR_INPUT session state (a stale
 * response can race a newer `session.state_changed` WS event and leave the UI
 * showing a stopped session while the agent runs — WAITING_FOR_INPUT means the
 * agent is alive and awaiting the next prompt, so an older response claiming
 * otherwise must not overwrite it), and must not overwrite a live TERMINAL
 * state (FAILED/CANCELLED/COMPLETED — which determines the recovery
 * affordances the UI shows) with an older or timestamp-less response. In both
 * cases the incoming status is accepted only when its timestamp is newer than
 * the live session's.
 */
function shouldApplyStatusState(status: SessionStatus, live: LiveSessionLike): boolean {
  if (!status.state) return false;
  const liveState = live?.state;
  const liveIsProtected =
    liveState === "STARTING" ||
    liveState === "RUNNING" ||
    liveState === "WAITING_FOR_INPUT" ||
    TERMINAL_STATES.has(liveState as TaskSessionState);
  if (!liveIsProtected) return true;
  const liveUpdated = live?.updated_at ? Date.parse(live.updated_at) : Number.NaN;
  const incomingUpdated = status.updated_at ? Date.parse(status.updated_at) : Number.NaN;
  return (
    Number.isFinite(liveUpdated) &&
    Number.isFinite(incomingUpdated) &&
    incomingUpdated > liveUpdated
  );
}

/**
 * Apply session status fields to local state (guarded by
 * `shouldApplyStatusState` against stale downgrades).
 */
function applyStatusToState(
  status: SessionStatus,
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter,
): void {
  setters.setWorktreePath(status.worktree_path ?? null);
  setters.setWorktreeBranch(status.worktree_branch ?? null);
  if (!status.state) return;
  const live = setters.getLiveSession?.(sessionId) ?? session;
  if (!shouldApplyStatusState(status, live)) {
    return; // stale or non-terminal status must not downgrade a running session
  }
  setters.setTaskSession({
    id: toSessionId(sessionId),
    task_id: toTaskId(taskId),
    state: status.state as TaskSessionState,
    started_at: session?.started_at ?? "",
    updated_at: status.updated_at ?? session?.updated_at ?? "",
  });
}

type RefreshSessionStatusParams = {
  client: NonNullable<ReturnType<typeof getWebSocketClient>>;
  taskId: string;
  sessionId: string;
  session: SessionLike;
  setSessionStatus: (status: SessionStatus) => void;
  setters: ResumeStateSetter;
  canContinue: () => boolean;
};

function waitForSessionStatusRetry(canContinue: () => boolean): Promise<boolean> {
  return new Promise((resolve) => {
    const timer = window.setTimeout(() => resolve(canContinue()), SESSION_ENTRY_RETRY_DELAY_MS);
    if (!canContinue()) {
      window.clearTimeout(timer);
      resolve(false);
    }
  });
}

async function requestSessionStatusWithRetry({
  client,
  taskId,
  sessionId,
  canContinue,
}: Omit<
  RefreshSessionStatusParams,
  "session" | "setSessionStatus" | "setters"
>): Promise<SessionStatus | null> {
  for (let attempt = 0; attempt < MAX_SESSION_STATUS_ATTEMPTS; attempt += 1) {
    if (!canContinue()) return null;
    try {
      return await client.request<SessionStatus>(
        "task.session.status",
        { task_id: taskId, session_id: sessionId },
        SESSION_ENTRY_REQUEST_TIMEOUT_MS,
      );
    } catch (error) {
      const canRetry =
        isWebSocketRequestTimeoutError(error) && attempt + 1 < MAX_SESSION_STATUS_ATTEMPTS;
      if (!canRetry || !(await waitForSessionStatusRetry(canContinue))) throw error;
    }
  }
  return null;
}

async function refreshSessionStatus({
  client,
  taskId,
  sessionId,
  session,
  setSessionStatus,
  setters,
  canContinue,
}: RefreshSessionStatusParams): Promise<SessionStatus | null> {
  if (!canContinue()) return null;
  try {
    const status = await requestSessionStatusWithRetry({
      client,
      taskId,
      sessionId,
      canContinue,
    });
    if (!status || !canContinue()) return null;
    if (applyStatusResponseOutcome(status, setters)) return null;
    setSessionStatus(status);
    applyStatusToState(status, taskId, sessionId, session, setters);
    // A status response confirming the agent is running must clear any
    // stale resume-skipped marker (a delayed response can otherwise leave
    // a Start button beside a running agent).
    if (status.is_agent_running || status.state === "RUNNING") {
      setters.setResumeSkipped?.(sessionId, false);
    }
    if (status.is_agent_running && setters.setAgentctlReady) {
      setters.setAgentctlReady(sessionId);
    }
    return status;
  } catch (err) {
    if (isTaskArchivedConflict(err) || !canContinue()) {
      clearArchiveRecovery(setters);
      return null;
    }
    setters.setResumptionState("error");
    setters.setError(null);
    setters.setNotice?.(null);
    setters.setRecoveryFailure?.(sessionStatusFailure(err));
    return null;
  }
}

/**
 * Record the resume-skipped marker only while the live session row is not
 * STARTING/RUNNING (checked here with typed live-store access so a stale
 * status can never leave a Start button beside a running agent).
 */
function recordResumeSkipIfStopped(setters: ResumeStateSetter, sessionId: string): void {
  const liveState = setters.getLiveSession?.(sessionId)?.state;
  if (liveState !== "STARTING" && liveState !== "RUNNING") {
    setters.setResumeSkipped?.(sessionId, true);
  }
}

type ResumeActionParams = {
  status: SessionStatus;
  taskId: string;
  sessionId: string;
  session: SessionLike;
  setters: ResumeStateSetter;
  preventAutoStart: boolean;
  canContinue: () => boolean;
};

async function performResumeAction({
  status,
  taskId,
  sessionId,
  session,
  setters,
  preventAutoStart,
  canContinue,
}: ResumeActionParams): Promise<boolean> {
  switch (decideResumeAction(status, preventAutoStart)) {
    case "running":
      setters.setResumptionState("running");
      setters.setResumeSkipped?.(sessionId, false);
      return false;
    case "skip":
      recordResumeSkipIfStopped(setters, sessionId);
      setters.setResumptionState("idle");
      return false;
    case "resume":
      return resumeWithSilentFallback(taskId, sessionId, session, setters, canContinue);
    case "restore":
      return resumeViaLaunch(buildRestoreWorkspaceRequest, {
        taskId,
        sessionId,
        session,
        setters,
        canContinue,
      });
    default:
      setters.setResumptionState("idle");
      return false;
  }
}

type ProcessResumeStatusParams = {
  status: SessionStatus;
  taskId: string;
  sessionId: string;
  session: SessionLike;
  setters: ResumeStateSetter;
  preventAutoStart: boolean;
  canContinue: () => boolean;
};

async function processResumeStatus({
  status,
  taskId,
  sessionId,
  session,
  setters,
  preventAutoStart,
  canContinue,
}: ProcessResumeStatusParams): Promise<boolean> {
  if (applyStatusResponseOutcome(status, setters)) return false;
  applyStatusToState(status, taskId, sessionId, session, setters);
  if (status.is_agent_running && setters.setAgentctlReady) {
    setters.setAgentctlReady(sessionId);
  }
  return performResumeAction({
    status,
    taskId,
    sessionId,
    session,
    setters,
    preventAutoStart,
    canContinue,
  });
}

// eslint-disable-next-line complexity -- status, restore, archive, and stale-request outcomes share one guarded transition.
async function checkAndResume({
  taskId,
  sessionId,
  session,
  setSessionStatus,
  setters,
  preventAutoStart,
  taskArchiveState,
  canContinue,
}: CheckAndResumeParams): Promise<void> {
  const client = getWebSocketClient();
  if (!client) return;
  if (taskArchiveState !== false || !canContinue()) return;
  setters.setResumptionState("checking");
  setters.setError(null);
  setters.setNotice?.(null);
  setters.setRecoveryFailure?.(null);
  let status: SessionStatus | null;
  try {
    status = await requestSessionStatusWithRetry({
      client,
      taskId,
      sessionId,
      canContinue,
    });
  } catch (err) {
    if (isTaskArchivedConflict(err) || !canContinue()) {
      clearArchiveRecovery(setters);
      return;
    }
    setters.setResumptionState("error");
    setters.setError(null);
    setters.setNotice?.(null);
    setters.setRecoveryFailure?.(sessionStatusFailure(err));
    return;
  }
  if (!status || !canContinue()) return;
  if (applyStatusResponseOutcome(status, setters)) return;
  try {
    setSessionStatus(status);
    const resumed = await processResumeStatus({
      status,
      taskId,
      sessionId,
      session,
      setters,
      preventAutoStart,
      canContinue,
    });
    if (resumed && canContinue()) {
      await refreshSessionStatus({
        client,
        taskId,
        sessionId,
        session,
        setSessionStatus,
        setters,
        canContinue,
      });
    }
  } catch (err) {
    if (isTaskArchivedConflict(err) || !canContinue()) {
      clearArchiveRecovery(setters);
      return;
    }
    setters.setResumptionState("error");
    setters.setRecoveryFailure?.(null);
    setters.setError(err instanceof Error ? err.message : t("common:unknownError"));
    setters.setNotice?.(null);
  }
}

interface UseSessionResumptionReturn {
  resumptionState: ResumptionState;
  sessionStatus: SessionStatus | null;
  error: string | null;
  notice: string | null;
  recoveryFailure: SessionRecoveryFailure | null;
  taskSessionState: TaskSessionState | null;
  worktreePath: string | null;
  worktreeBranch: string | null;
  resumeSession: () => Promise<boolean>;
  retrySessionStatus: () => Promise<void>;
}

/**
 * Hook for handling session resumption on page reload.
 * When a sessionId is provided (from URL), it checks the session status
 * and automatically resumes if needed.
 */
type SessionResetAndCheckResult = {
  sessionStatus: SessionStatus | null;
  captureRequest: () => SessionRequestIdentity;
  buildGuardedSettersFor: (capturedRequest: SessionRequestIdentity) => ResumeStateSetter;
  retryStatus: () => Promise<void>;
};

type ResetAndCheckParams = {
  taskId: string | null;
  sessionId: string | null;
  connectionStatus: string;
  session: SessionLike;
  setters: ResumeStateSetter;
  preventAutoStart: boolean;
  taskArchiveState: TaskArchiveState;
};

const getSessionRequestKey = (
  taskId: string | null,
  sessionId: string | null,
  taskArchiveState: TaskArchiveState,
) => JSON.stringify([taskId, sessionId, taskArchiveState]);

/** Extracted effects: reset state on session/task change, auto-check/resume, and remote retry. */
// eslint-disable-next-line max-lines-per-function -- one hook owns the request identity and lifecycle effects.
function useSessionResetAndCheck({
  taskId,
  sessionId,
  connectionStatus,
  session,
  setters,
  preventAutoStart,
  taskArchiveState,
}: ResetAndCheckParams): SessionResetAndCheckResult {
  const requestKey = getSessionRequestKey(taskId, sessionId, taskArchiveState);
  const [sessionStatusState, setSessionStatus] = useState<{
    requestKey: string;
    status: SessionStatus | null;
  }>({ requestKey, status: null });
  const sessionStatus =
    sessionStatusState.requestKey === requestKey ? sessionStatusState.status : null;
  const hasAttemptedResume = useRef(false);
  const remoteStatusRetryCount = useRef(0);
  const requestGenerationRef = useRef(0);
  const activeRequestRef = useRef<SessionRequestIdentity>({ key: requestKey, generation: 0 });

  // Publish the new identity during commit so callbacks from the previous
  // request are rejected before passive effects or queued promise handlers run.
  useLayoutEffect(() => {
    requestGenerationRef.current += 1;
    activeRequestRef.current = {
      key: requestKey,
      generation: requestGenerationRef.current,
    };
  }, [requestKey]);

  // Reset all local state when session or task changes to prevent stale data
  // from a previous session leaking into the new one (e.g. topbar branch).
  useEffect(() => {
    hasAttemptedResume.current = false;
    remoteStatusRetryCount.current = 0;
    setters.setResumptionState("idle");
    setters.setError(null);
    setters.setNotice?.(null);
    setters.setRecoveryFailure?.(null);
    setters.setWorktreePath(null);
    setters.setWorktreeBranch(null);
  }, [sessionId, taskId, taskArchiveState]); // eslint-disable-line react-hooks/exhaustive-deps -- intentional reset on dep change

  // Check session status and auto-resume if needed
  useEffect(() => {
    if (
      !taskId ||
      !sessionId ||
      connectionStatus !== "connected" ||
      taskArchiveState !== false ||
      hasAttemptedResume.current
    )
      return;
    hasAttemptedResume.current = true;
    const capturedRequest = activeRequestRef.current;
    const guardedSetters = buildGuardedSetters(activeRequestRef, capturedRequest, setters);
    const canContinue = () => isCurrentRequest(activeRequestRef.current, capturedRequest);
    checkAndResume({
      taskId,
      sessionId,
      session,
      preventAutoStart,
      taskArchiveState,
      canContinue,
      setSessionStatus: (s) => {
        if (isCurrentRequest(activeRequestRef.current, capturedRequest)) {
          setSessionStatus({ requestKey: capturedRequest.key, status: s });
        }
      },
      setters: guardedSetters,
    });
  }, [taskId, sessionId, connectionStatus, session, preventAutoStart, taskArchiveState]); // eslint-disable-line react-hooks/exhaustive-deps

  // Freshly created remote sessions may return status before runtime metadata is available.
  // Retry a few times so topbar/tooltips can show remote details without manual refresh.
  useEffect(() => {
    if (!taskId || !sessionId || connectionStatus !== "connected" || taskArchiveState !== false)
      return;
    if (!sessionStatus?.is_remote_executor) return;
    if (sessionStatus.remote_checked_at || sessionStatus.remote_status_error) return;
    if (remoteStatusRetryCount.current >= 3) return;
    const capturedRequest = activeRequestRef.current;

    const timer = window.setTimeout(async () => {
      if (!isCurrentRequest(activeRequestRef.current, capturedRequest)) return;
      const client = getWebSocketClient();
      if (!client) return;
      remoteStatusRetryCount.current += 1;
      try {
        const nextStatus = await client.request<SessionStatus>(
          "task.session.status",
          {
            task_id: taskId,
            session_id: sessionId,
          },
          SESSION_ENTRY_REQUEST_TIMEOUT_MS,
        );
        if (isCurrentRequest(activeRequestRef.current, capturedRequest)) {
          setSessionStatus({ requestKey: capturedRequest.key, status: nextStatus });
        }
      } catch {
        // Best-effort refresh only.
      }
    }, 1500);

    return () => window.clearTimeout(timer);
  }, [taskId, sessionId, connectionStatus, sessionStatus, taskArchiveState]);

  const retryStatus = useCallback(async () => {
    if (!taskId || !sessionId || connectionStatus !== "connected" || taskArchiveState !== false) {
      return;
    }
    const capturedRequest = activeRequestRef.current;
    const guardedSetters = buildGuardedSetters(activeRequestRef, capturedRequest, setters);
    const canContinue = () => isCurrentRequest(activeRequestRef.current, capturedRequest);
    guardedSetters.setResumptionState("checking");
    guardedSetters.setError(null);
    guardedSetters.setNotice?.(null);
    guardedSetters.setRecoveryFailure?.(null);
    const client = getWebSocketClient();
    if (!client) return;
    const status = await refreshSessionStatus({
      client,
      taskId,
      sessionId,
      session,
      setSessionStatus: (nextStatus) => {
        if (canContinue()) {
          setSessionStatus({ requestKey: capturedRequest.key, status: nextStatus });
        }
      },
      setters: guardedSetters,
      canContinue,
    });
    if (!status || !canContinue()) return;
    guardedSetters.setError(null);
    guardedSetters.setNotice?.(null);
    guardedSetters.setRecoveryFailure?.(null);
    guardedSetters.setResumptionState(
      status.is_agent_running || status.state === "RUNNING" ? "running" : "idle",
    );
  }, [connectionStatus, session, sessionId, setters, taskArchiveState, taskId]);

  return {
    sessionStatus,
    captureRequest: () => activeRequestRef.current,
    buildGuardedSettersFor: (capturedRequest) =>
      buildGuardedSetters(activeRequestRef, capturedRequest, setters),
    retryStatus,
  };
}

type ManualResumeResponse = Awaited<ReturnType<typeof launchSession>>;

function applyManualResumeResponse(
  response: ManualResumeResponse,
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter,
): boolean {
  if (!response.success) {
    setters.setResumptionState("error");
    setters.setRecoveryFailure?.(null);
    setters.setError(response.error ?? t("task:failedToResumeSession"));
    return false;
  }
  setters.setResumptionState("resumed");
  setters.setNotice?.(null);
  if (response.state) {
    setters.setTaskSession({
      id: toSessionId(sessionId),
      task_id: toTaskId(taskId),
      state: response.state as TaskSessionState,
      started_at: session?.started_at ?? "",
      updated_at: session?.updated_at ?? "",
    });
  }
  // A STARTING response keeps the resume-skipped marker until a later RUNNING
  // event confirms that the agent is active.
  if (response.state === "RUNNING") setters.setResumeSkipped?.(sessionId, false);
  if (response.worktree_path) setters.setWorktreePath(response.worktree_path);
  if (response.worktree_branch) setters.setWorktreeBranch(response.worktree_branch);
  return true;
}

function handleManualResumeError(
  error: unknown,
  setters: ResumeStateSetter,
  canContinue: () => boolean,
): boolean {
  if (isTaskArchivedConflict(error)) {
    clearArchiveRecovery(setters);
    return false;
  }
  if (!canContinue()) return false;
  setters.setResumptionState("error");
  setters.setRecoveryFailure?.(null);
  setters.setError(error instanceof Error ? error.message : t("common:unknownError"));
  return false;
}

type ManualResumeParams = {
  taskId: string | null;
  sessionId: string | null;
  taskArchiveState: TaskArchiveState;
  session: SessionLike;
  captureRequest: () => SessionRequestIdentity;
  buildGuardedSettersFor: (capturedRequest: SessionRequestIdentity) => ResumeStateSetter;
};

function useManualResumeSession({
  taskId,
  sessionId,
  taskArchiveState,
  session,
  captureRequest,
  buildGuardedSettersFor,
}: ManualResumeParams): () => Promise<boolean> {
  return useCallback(async (): Promise<boolean> => {
    if (!taskId || !sessionId || taskArchiveState !== false) return false;
    const capturedRequest = captureRequest();
    const canContinue = () => isCurrentRequest(captureRequest(), capturedRequest);
    const guardedSetters = buildGuardedSettersFor(capturedRequest);
    if (!canContinue()) return false;
    const startingProjection = markSessionStarting(taskId, sessionId, session, guardedSetters);
    guardedSetters.setResumptionState("resuming");
    guardedSetters.setError(null);
    guardedSetters.setNotice?.(null);
    guardedSetters.setRecoveryFailure?.(null);
    try {
      const response = await launchSession(buildResumeRequest(taskId, sessionId).request);
      if (!canContinue()) {
        startingProjection?.rollback();
        return false;
      }
      const resumed = applyManualResumeResponse(
        response,
        taskId,
        sessionId,
        session,
        guardedSetters,
      );
      if (!resumed) startingProjection?.rollback();
      return resumed;
    } catch (error) {
      const handled = handleManualResumeError(error, guardedSetters, canContinue);
      if (!handled) startingProjection?.rollback();
      return handled;
    }
  }, [taskId, sessionId, taskArchiveState, session, captureRequest, buildGuardedSettersFor]);
}

export type SessionResumptionOptions = {
  onTaskArchiveConflict?: () => void;
};

export function useSessionResumption(
  taskId: string | null,
  sessionId: string | null,
  taskArchiveState: TaskArchiveState = false,
  options: SessionResumptionOptions = {},
): UseSessionResumptionReturn {
  const [resumptionState, setResumptionState] = useState<ResumptionState>("idle");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [recoveryFailure, setRecoveryFailure] = useState<SessionRecoveryFailure | null>(null);
  const [worktreePath, setWorktreePath] = useState<string | null>(null);
  const [worktreeBranch, setWorktreeBranch] = useState<string | null>(null);
  const connectionStatus = useAppStore((state) => state.connection.status);
  const preventAutoStartAgentOnOpen = useAppStore(
    (state) => state.userSettings.preventAutoStartAgentOnOpen,
  );
  const session = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  const setSessionAgentctlStatus = useAppStore((state) => state.setSessionAgentctlStatus);
  const setResumeSkipped = useAppStore((state) => state.setResumeSkipped);
  const storeApi = useAppStoreApi();

  const setters: ResumeStateSetter = {
    setResumptionState,
    setError,
    setNotice,
    setWorktreePath,
    setWorktreeBranch,
    setTaskSession,
    setTaskSessionUnscoped: setTaskSession,
    setAgentctlReady: (sid: string) => setSessionAgentctlStatus(sid, { status: "ready" }),
    setResumeSkipped,
    getLiveSession: (sid: string) => storeApi.getState().taskSessions.items[sid] ?? null,
    setRecoveryFailure,
    onTaskArchiveConflict: options.onTaskArchiveConflict,
  };

  useSessionRecoveryFeedback(sessionId, session?.state, error, notice, setters);

  const { sessionStatus, captureRequest, buildGuardedSettersFor, retryStatus } =
    useSessionResetAndCheck({
      taskId,
      sessionId,
      connectionStatus,
      session,
      setters,
      preventAutoStart: preventAutoStartAgentOnOpen,
      taskArchiveState,
    });

  const resumeSession = useManualResumeSession({
    taskId,
    sessionId,
    taskArchiveState,
    session,
    captureRequest,
    buildGuardedSettersFor,
  });

  return {
    resumptionState,
    sessionStatus,
    error,
    notice,
    recoveryFailure,
    taskSessionState: session?.state ?? null,
    worktreePath,
    worktreeBranch,
    resumeSession,
    retrySessionStatus: retryStatus,
  };
}
