import { useCallback, useEffect, useState, useRef } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { launchSession } from "@/lib/services/session-launch-service";
import {
  buildResumeRequest,
  buildRestoreWorkspaceRequest,
} from "@/lib/services/session-launch-helpers";
import { useAppStore } from "@/components/state-provider";
import type { TaskSessionState } from "@/lib/types/http";

export type SessionStatus = {
  session_id: string;
  task_id: string;
  state: string;
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
  error?: string;
};

export type ResumptionState = "idle" | "checking" | "resuming" | "resumed" | "running" | "error";

type ResumeResponse = {
  success: boolean;
  state?: string;
  worktree_path?: string;
  worktree_branch?: string;
  error?: string;
};

export type ResumeStateSetter = {
  setResumptionState: (s: ResumptionState) => void;
  setError: (e: string | null) => void;
  setWorktreePath: (p: string | null) => void;
  setWorktreeBranch: (b: string | null) => void;
  setTaskSession: (s: {
    id: string;
    task_id: string;
    state: TaskSessionState;
    started_at: string;
    updated_at: string;
  }) => void;
  setAgentctlReady?: (sessionId: string) => void;
};

type SessionLike = { started_at?: string; updated_at?: string } | null;

/** Apply a successful resume response to local state. */
function applyResumeResponse(
  resp: ResumeResponse,
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter,
): boolean {
  if (resp.success) {
    setters.setResumptionState("resumed");
    if (resp.state) {
      setters.setTaskSession({
        id: sessionId,
        task_id: taskId,
        state: resp.state as TaskSessionState,
        started_at: session?.started_at ?? "",
        updated_at: session?.updated_at ?? "",
      });
    }
    if (resp.worktree_path) setters.setWorktreePath(resp.worktree_path);
    if (resp.worktree_branch) setters.setWorktreeBranch(resp.worktree_branch);
    return true;
  }
  setters.setResumptionState("error");
  setters.setError(resp.error ?? "Failed to resume session");
  return false;
}

/** Launch a session via a request builder and apply the response. */
async function resumeViaLaunch(
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter,
  buildRequest: (
    taskId: string,
    sessionId: string,
  ) => { request: import("@/lib/services/session-launch-service").LaunchSessionRequest },
): Promise<void> {
  setters.setResumptionState("resuming");
  const { request } = buildRequest(taskId, sessionId);
  const launchResp = await launchSession(request);
  applyResumeResponse(
    {
      success: launchResp.success,
      state: launchResp.state,
      worktree_path: launchResp.worktree_path,
      worktree_branch: launchResp.worktree_branch,
    },
    taskId,
    sessionId,
    session,
    setters,
  );
}

/** Attempt resume, silently falling back to restore_workspace on any failure.
 *  Used for sessions where the backend reports needs_resume=true — typically
 *  WAITING_FOR_INPUT after restart, or FAILED with a resumable token. The user
 *  only sees an error banner if BOTH attempts fail; otherwise they just see
 *  the session reload (resumed) or the workspace come back read-only.
 *  Exported for unit tests. */
export async function resumeWithSilentFallback(
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter,
): Promise<void> {
  setters.setResumptionState("resuming");
  if (
    await tryLaunch(
      buildResumeRequest(taskId, sessionId).request,
      taskId,
      sessionId,
      session,
      setters,
    )
  ) {
    return;
  }
  // Resume failed (returned success=false OR threw). Fall back to read-only
  // workspace restore so the user keeps file/terminal/git access.
  if (
    await tryLaunch(
      buildRestoreWorkspaceRequest(taskId, sessionId).request,
      taskId,
      sessionId,
      session,
      setters,
    )
  ) {
    return;
  }
  setters.setResumptionState("error");
  setters.setError("Failed to resume session — workspace restore also unavailable");
}

/** Run a single launch attempt; returns true on success, false on any failure.
 *  Logs caught errors to the console so silent fallback paths remain debuggable
 *  (errors otherwise vanish into the implicit `false` return). */
async function tryLaunch(
  request: import("@/lib/services/session-launch-service").LaunchSessionRequest,
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter,
): Promise<boolean> {
  try {
    const resp = await launchSession(request);
    if (!resp.success) return false;
    applyResumeResponse(
      {
        success: true,
        state: resp.state,
        worktree_path: resp.worktree_path,
        worktree_branch: resp.worktree_branch,
      },
      taskId,
      sessionId,
      session,
      setters,
    );
    return true;
  } catch (err) {
    console.error("[tryLaunch] session launch failed", {
      intent: request.intent,
      sessionId,
      err,
    });
    return false;
  }
}

type CheckAndResumeParams = {
  taskId: string;
  sessionId: string;
  session: SessionLike;
  setSessionStatus: (s: SessionStatus) => void;
  setters: ResumeStateSetter;
};

/** Apply session status fields to local state. */
function applyStatusToState(
  status: SessionStatus,
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter,
): void {
  setters.setWorktreePath(status.worktree_path ?? null);
  setters.setWorktreeBranch(status.worktree_branch ?? null);
  if (status.state) {
    setters.setTaskSession({
      id: sessionId,
      task_id: taskId,
      state: status.state as TaskSessionState,
      started_at: session?.started_at ?? "",
      updated_at: session?.updated_at ?? "",
    });
  }
}

async function checkAndResume({
  taskId,
  sessionId,
  session,
  setSessionStatus,
  setters,
}: CheckAndResumeParams): Promise<void> {
  const client = getWebSocketClient();
  if (!client) return;
  setters.setResumptionState("checking");
  setters.setError(null);
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
    if (status.is_agent_running) {
      setters.setResumptionState("running");
    } else if (status.needs_resume && status.is_resumable) {
      await resumeWithSilentFallback(taskId, sessionId, session, setters);
    } else if (status.needs_workspace_restore) {
      await resumeViaLaunch(taskId, sessionId, session, setters, buildRestoreWorkspaceRequest);
    } else {
      setters.setResumptionState("idle");
    }
  } catch (err) {
    setters.setResumptionState("error");
    setters.setError(err instanceof Error ? err.message : "Unknown error");
  }
}

interface UseSessionResumptionReturn {
  resumptionState: ResumptionState;
  sessionStatus: SessionStatus | null;
  error: string | null;
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

/** Extracted effects: reset state on session/task change, auto-check/resume, and remote retry. */
function useSessionResetAndCheck(
  taskId: string | null,
  sessionId: string | null,
  connectionStatus: string,
  session: { started_at?: string; updated_at?: string } | null,
  setters: ResumeStateSetter,
): SessionResetAndCheckResult {
  const [sessionStatusState, setSessionStatus] = useState<{
    id: string | null;
    status: SessionStatus | null;
  }>({ id: sessionId, status: null });
  // Reset sessionStatus when sessionId changes (derived, not in effect)
  const sessionStatus = sessionStatusState.id === sessionId ? sessionStatusState.status : null;
  const hasAttemptedResume = useRef(false);
  const remoteStatusRetryCount = useRef(0);
  const activeSessionRef = useRef(sessionId);

  // Reset all local state when session or task changes to prevent stale data
  // from a previous session leaking into the new one (e.g. topbar branch).
  useEffect(() => {
    activeSessionRef.current = sessionId;
    hasAttemptedResume.current = false;
    remoteStatusRetryCount.current = 0;
    setters.setResumptionState("idle");
    setters.setError(null);
    setters.setWorktreePath(null);
    setters.setWorktreeBranch(null);
  }, [sessionId, taskId]); // eslint-disable-line react-hooks/exhaustive-deps -- intentional reset on dep change

  // Check session status and auto-resume if needed
  useEffect(() => {
    if (!taskId || !sessionId || connectionStatus !== "connected" || hasAttemptedResume.current)
      return;
    hasAttemptedResume.current = true;
    const guardedSetters = buildGuardedSetters(activeSessionRef, sessionId, setters);
    checkAndResume({
      taskId,
      sessionId,
      session,
      setSessionStatus: (s) => {
        if (activeSessionRef.current === sessionId) setSessionStatus({ id: sessionId, status: s });
      },
      setters: guardedSetters,
    });
  }, [taskId, sessionId, connectionStatus, session]); // eslint-disable-line react-hooks/exhaustive-deps

  // Freshly created remote sessions may return status before runtime metadata is available.
  // Retry a few times so topbar/tooltips can show remote details without manual refresh.
  useEffect(() => {
    if (!taskId || !sessionId || connectionStatus !== "connected") return;
    if (!sessionStatus?.is_remote_executor) return;
    if (sessionStatus.remote_checked_at || sessionStatus.remote_status_error) return;
    if (remoteStatusRetryCount.current >= 3) return;

    const timer = window.setTimeout(async () => {
      const client = getWebSocketClient();
      if (!client) return;
      remoteStatusRetryCount.current += 1;
      try {
        const nextStatus = await client.request<SessionStatus>("task.session.status", {
          task_id: taskId,
          session_id: sessionId,
        });
        setSessionStatus({ id: sessionId, status: nextStatus });
      } catch {
        // Best-effort refresh only.
      }
    }, 1500);

    return () => window.clearTimeout(timer);
  }, [taskId, sessionId, connectionStatus, sessionStatus]);

  return { sessionStatus };
}

/** Wrap state setters with a guard that prevents stale async callbacks from updating state. */
function buildGuardedSetters(
  activeSessionRef: React.RefObject<string | null>,
  capturedSessionId: string,
  setters: ResumeStateSetter,
): ResumeStateSetter {
  const guard = () => activeSessionRef.current === capturedSessionId;
  return {
    ...setters,
    setResumptionState: (s) => {
      if (guard()) setters.setResumptionState(s);
    },
    setError: (e) => {
      if (guard()) setters.setError(e);
    },
    setWorktreePath: (p) => {
      if (guard()) setters.setWorktreePath(p);
    },
    setWorktreeBranch: (b) => {
      if (guard()) setters.setWorktreeBranch(b);
    },
  };
}

export function useSessionResumption(
  taskId: string | null,
  sessionId: string | null,
): UseSessionResumptionReturn {
  const [resumptionState, setResumptionState] = useState<ResumptionState>("idle");
  const [error, setError] = useState<string | null>(null);
  const [worktreePath, setWorktreePath] = useState<string | null>(null);
  const [worktreeBranch, setWorktreeBranch] = useState<string | null>(null);
  const connectionStatus = useAppStore((state) => state.connection.status);
  const session = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  const setSessionAgentctlStatus = useAppStore((state) => state.setSessionAgentctlStatus);

  const setters: ResumeStateSetter = {
    setResumptionState,
    setError,
    setWorktreePath,
    setWorktreeBranch,
    setTaskSession,
    setAgentctlReady: (sid: string) => setSessionAgentctlStatus(sid, { status: "ready" }),
  };

  const { sessionStatus } = useSessionResetAndCheck(
    taskId,
    sessionId,
    connectionStatus,
    session,
    setters,
  );

  // Manual resume function
  const resumeSession = useCallback(async (): Promise<boolean> => {
    if (!taskId || !sessionId) return false;
    setResumptionState("resuming");
    setError(null);
    try {
      const { request } = buildResumeRequest(taskId, sessionId);
      const response = await launchSession(request);
      if (response.success) {
        setResumptionState("resumed");
        if (response.state) {
          setTaskSession({
            id: sessionId,
            task_id: taskId,
            state: response.state as TaskSessionState,
            started_at: session?.started_at ?? "",
            updated_at: session?.updated_at ?? "",
          });
        }
        if (response.worktree_path) setWorktreePath(response.worktree_path);
        if (response.worktree_branch) setWorktreeBranch(response.worktree_branch);
        return true;
      }
      setResumptionState("error");
      setError("Failed to resume session");
      return false;
    } catch (err) {
      setResumptionState("error");
      setError(err instanceof Error ? err.message : "Unknown error");
      return false;
    }
  }, [taskId, sessionId, session, setTaskSession, setWorktreePath, setWorktreeBranch]);

  return {
    resumptionState,
    sessionStatus,
    error,
    taskSessionState: session?.state ?? null,
    worktreePath,
    worktreeBranch,
    resumeSession,
  };
}
