import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { launchSession } from "@/lib/services/session-launch-service";
import {
  buildResumeRequest,
  buildRestoreWorkspaceRequest,
} from "@/lib/services/session-launch-helpers";
import { useSessionRecoveryFeedback } from "./use-session-recovery-feedback";
import {
  buildGuardedSetters,
  isCurrentRequest,
  type SessionRequestIdentity,
} from "./use-session-resumption-request-guard";
import { resumeViaLaunch, resumeWithSilentFallback } from "./use-session-resumption-launch";
import { resolveRequestErrorMessage } from "@/lib/services/session-recovery-service";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type SessionId,
  type TaskId,
  type TaskSessionState,
} from "@/lib/types/http";
import { t } from "@/lib/i18n";

export type SessionStatus = {
  session_id: string;
  task_id: string;
  state: string;
  updated_at?: string;
  agent_profile_id?: string;
  is_agent_running: boolean;
  is_resumable: boolean;
  needs_resume: boolean;
  needs_workspace_restore?: boolean;
  resume_reason?: string;
  acp_session_id?: string;
  worktree_path?: string;
  worktree_branch?: string;
  executor_id?: string;
  executor_type?: string;
  executor_name?: string;
  runtime?: string;
  is_remote_executor?: boolean;
  remote_state?: string;
  remote_name?: string;
  remote_created_at?: string;
  remote_checked_at?: string;
  remote_status_error?: string;
  capabilities?: {
    embedded_vscode: boolean;
  };
  error?: string;
};

export type ResumptionState = "idle" | "checking" | "resuming" | "resumed" | "running" | "error";

export type ResumeStateSetter = {
  setResumptionState: (s: ResumptionState) => void;
  setError: (e: string | null) => void;
  setNotice?: (notice: string | null) => void;
  setWorktreePath: (p: string | null) => void;
  setWorktreeBranch: (p: string | null) => void;
  setTaskSession: (s: {
    id: SessionId;
    task_id: TaskId;
    state: TaskSessionState;
    started_at: string;
    updated_at: string;
  }) => void;
  setAgentctlReady?: (sessionId: string) => void;
  /** Records/clears the resume-skipped marker (prevent-auto-start-on-open). */
  setResumeSkipped?: (sessionId: string, skipped: boolean) => void;
  /** Reads the session row live from the store (monotonic hydration guard). */
  getLiveSession?: (sessionId: string) => SessionLike | null;
};

export type SessionLike = { started_at?: string; updated_at?: string; state?: string } | null;

type CheckAndResumeParams = {
  taskId: string;
  sessionId: string;
  session: SessionLike;
  setSessionStatus: (s: SessionStatus) => void;
  setters: ResumeStateSetter;
  /** True when the prevent-auto-start-on-open preference gates open-time resumes. */
  preventAutoStart: boolean;
};

const TERMINAL_STATES = new Set<TaskSessionState>(["FAILED", "CANCELLED", "COMPLETED"]);

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
};

async function refreshSessionStatus({
  client,
  taskId,
  sessionId,
  session,
  setSessionStatus,
  setters,
}: RefreshSessionStatusParams): Promise<void> {
  try {
    const status = await client.request<SessionStatus>("task.session.status", {
      task_id: taskId,
      session_id: sessionId,
    });
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
  } catch (err) {
    console.error("[refreshSessionStatus] failed to refresh session status", { sessionId, err });
  }
}

type ResumeAction = "running" | "skip" | "resume" | "restore" | "idle";

function decideResumeAction(status: SessionStatus, preventAutoStart: boolean): ResumeAction {
  if (status.is_agent_running) return "running";
  if (preventAutoStart && status.needs_resume && status.is_resumable) return "skip";
  if (status.needs_resume && status.is_resumable) return "resume";
  if (status.needs_workspace_restore) return "restore";
  return "idle";
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

async function checkAndResume({
  taskId,
  sessionId,
  session,
  setSessionStatus,
  setters,
  preventAutoStart,
}: CheckAndResumeParams): Promise<void> {
  const client = getWebSocketClient();
  if (!client) return;
  setters.setResumptionState("checking");
  setters.setError(null);
  setters.setNotice?.(null);
  try {
    const status = await client.request<SessionStatus>("task.session.status", {
      task_id: taskId,
      session_id: sessionId,
    });
    setSessionStatus(status);
    if (status.error) {
      setters.setResumptionState("error");
      setters.setError(status.error);
      return;
    }
    applyStatusToState(status, taskId, sessionId, session, setters);
    // Seed agentctl readiness from session status — the WS event may have
    // already been sent before we subscribed (page reload on running session).
    if (status.is_agent_running && setters.setAgentctlReady) {
      setters.setAgentctlReady(sessionId);
    }
    let resumed = false;
    switch (decideResumeAction(status, preventAutoStart)) {
      case "running":
        setters.setResumptionState("running");
        // A status response confirming the agent is running clears any stale
        // resume-skipped marker (the WS RUNNING transition may have been
        // missed, or the marker predates this page load).
        setters.setResumeSkipped?.(sessionId, false);
        break;
      case "skip":
        // The preference gates the open-time auto-resume: leave the session
        // stopped and record the skip so the Start agent button renders.
        // The record is guarded against a live STARTING/RUNNING row (a stale
        // status can race a running WS transition).
        recordResumeSkipIfStopped(setters, sessionId);
        setters.setResumptionState("idle");
        break;
      case "resume":
        resumed = await resumeWithSilentFallback(taskId, sessionId, session, setters);
        break;
      case "restore":
        resumed = await resumeViaLaunch(
          taskId,
          sessionId,
          session,
          setters,
          buildRestoreWorkspaceRequest,
        );
        break;
      default:
        setters.setResumptionState("idle");
    }
    if (resumed) {
      await refreshSessionStatus({ client, taskId, sessionId, session, setSessionStatus, setters });
    }
  } catch (err) {
    setters.setResumptionState("error");
    setters.setError(resolveRequestErrorMessage(err, t));
    setters.setNotice?.(null);
  }
}

interface UseSessionResumptionReturn {
  resumptionState: ResumptionState;
  sessionStatus: SessionStatus | null;
  error: string | null;
  notice: string | null;
  taskSessionState: TaskSessionState | null;
  worktreePath: string | null;
  worktreeBranch: string | null;
  resumeSession: () => Promise<boolean>;
}

/**
 * Hook for handling session resumption on page reload.
 * When a sessionId is provided (from URL), it checks the session status
 * and automatically resumes if needed.
 */
type SessionResetAndCheckResult = {
  sessionStatus: SessionStatus | null;
};

type ResetAndCheckParams = {
  taskId: string | null;
  sessionId: string | null;
  connectionStatus: string;
  session: SessionLike;
  setters: ResumeStateSetter;
  preventAutoStart: boolean;
};

const getSessionRequestKey = (taskId: string | null, sessionId: string | null) =>
  JSON.stringify([taskId, sessionId]);

/** Extracted effects: reset state on session/task change, auto-check/resume, and remote retry. */
function useSessionResetAndCheck({
  taskId,
  sessionId,
  connectionStatus,
  session,
  setters,
  preventAutoStart,
}: ResetAndCheckParams): SessionResetAndCheckResult {
  const requestKey = getSessionRequestKey(taskId, sessionId);
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
    setters.setWorktreePath(null);
    setters.setWorktreeBranch(null);
  }, [sessionId, taskId]); // eslint-disable-line react-hooks/exhaustive-deps -- intentional reset on dep change

  // Check session status and auto-resume if needed
  useEffect(() => {
    if (!taskId || !sessionId || connectionStatus !== "connected" || hasAttemptedResume.current)
      return;
    hasAttemptedResume.current = true;
    const capturedRequest = activeRequestRef.current;
    const guardedSetters = buildGuardedSetters(activeRequestRef, capturedRequest, setters);
    checkAndResume({
      taskId,
      sessionId,
      session,
      preventAutoStart,
      setSessionStatus: (s) => {
        if (isCurrentRequest(activeRequestRef.current, capturedRequest)) {
          setSessionStatus({ requestKey: capturedRequest.key, status: s });
        }
      },
      setters: guardedSetters,
    });
  }, [taskId, sessionId, connectionStatus, session, preventAutoStart]); // eslint-disable-line react-hooks/exhaustive-deps

  // Freshly created remote sessions may return status before runtime metadata is available.
  // Retry a few times so topbar/tooltips can show remote details without manual refresh.
  useEffect(() => {
    if (!taskId || !sessionId || connectionStatus !== "connected") return;
    if (!sessionStatus?.is_remote_executor) return;
    if (sessionStatus.remote_checked_at || sessionStatus.remote_status_error) return;
    if (remoteStatusRetryCount.current >= 3) return;
    const capturedRequest = activeRequestRef.current;

    const timer = window.setTimeout(async () => {
      const client = getWebSocketClient();
      if (!client) return;
      remoteStatusRetryCount.current += 1;
      try {
        const nextStatus = await client.request<SessionStatus>("task.session.status", {
          task_id: taskId,
          session_id: sessionId,
        });
        if (isCurrentRequest(activeRequestRef.current, capturedRequest)) {
          setSessionStatus({ requestKey: capturedRequest.key, status: nextStatus });
        }
      } catch {
        // Best-effort refresh only.
      }
    }, 1500);

    return () => window.clearTimeout(timer);
  }, [taskId, sessionId, connectionStatus, sessionStatus]);

  return { sessionStatus };
}

export function useSessionResumption(
  taskId: string | null,
  sessionId: string | null,
): UseSessionResumptionReturn {
  const [resumptionState, setResumptionState] = useState<ResumptionState>("idle");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
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
    setAgentctlReady: (sid: string) => setSessionAgentctlStatus(sid, { status: "ready" }),
    setResumeSkipped,
    getLiveSession: (sid: string) => storeApi.getState().taskSessions.items[sid] ?? null,
  };

  useSessionRecoveryFeedback(sessionId, session?.state, error, notice, setters);

  const { sessionStatus } = useSessionResetAndCheck({
    taskId,
    sessionId,
    connectionStatus,
    session,
    setters,
    preventAutoStart: preventAutoStartAgentOnOpen,
  });

  // Manual resume function
  const resumeSession = useCallback(async (): Promise<boolean> => {
    if (!taskId || !sessionId) return false;
    setResumptionState("resuming");
    setError(null);
    setNotice(null);
    try {
      const { request } = buildResumeRequest(taskId, sessionId);
      const response = await launchSession(request);
      if (response.success) {
        setResumptionState("resumed");
        setNotice(null);
        if (response.state) {
          setTaskSession({
            id: toSessionId(sessionId),
            task_id: toTaskId(taskId),
            state: response.state as TaskSessionState,
            started_at: session?.started_at ?? "",
            updated_at: session?.updated_at ?? "",
          });
        }
        // A successful resume commonly returns state STARTING (launch
        // accepted, not agent running). Only confirmed RUNNING clears the
        // resume-skipped marker; a STARTING response keeps it so the Start
        // button stays until the WS RUNNING transition (or a later status)
        // confirms the agent. Failed resumes keep it as a retry affordance.
        if (response.state === "RUNNING") {
          setResumeSkipped(sessionId, false);
        }
        if (response.worktree_path) setWorktreePath(response.worktree_path);
        if (response.worktree_branch) setWorktreeBranch(response.worktree_branch);
        return true;
      }
      setResumptionState("error");
      setError(response.error ?? t("task:failedToResumeSession"));
      return false;
    } catch (err) {
      setResumptionState("error");
      setError(resolveRequestErrorMessage(err, t));
      return false;
    }
  }, [
    taskId,
    sessionId,
    session,
    setTaskSession,
    setNotice,
    setWorktreePath,
    setWorktreeBranch,
    setResumeSkipped,
  ]);

  return {
    resumptionState,
    sessionStatus,
    error,
    notice,
    taskSessionState: session?.state ?? null,
    worktreePath,
    worktreeBranch,
    resumeSession,
  };
}
