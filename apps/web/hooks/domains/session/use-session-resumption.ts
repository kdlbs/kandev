import { useCallback, useEffect, useRef, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { launchSession } from "@/lib/services/session-launch-service";
import {
  buildResumeRequest,
  buildRestoreWorkspaceRequest,
} from "@/lib/services/session-launch-helpers";
import { useSessionRecoveryFeedback } from "./use-session-recovery-feedback";
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

type SessionLike = { started_at?: string; updated_at?: string; state?: string } | null;

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
        id: toSessionId(sessionId),
        task_id: toTaskId(taskId),
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
  setters.setError(resp.error ?? t("task:failedToResumeSession"));
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
): Promise<boolean> {
  setters.setResumptionState("resuming");
  const { request } = buildRequest(taskId, sessionId);
  const launchResp = await launchSession(request);
  const ok = applyResumeResponse(
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
  // restore_workspace's whole purpose is to bring up agentctl HTTP for an
  // otherwise-idle session — when it returns success the workspace+agentctl
  // is up by definition. The backend's cached agentctl status snapshot uses
  // "workspace stream attached" as its readiness signal, which is wrong on
  // WS reconnect (stream detaches but agentctl HTTP keeps running) and the
  // existing execution does not re-emit agentctl_ready, so the FileBrowser
  // would otherwise stay stuck on "Preparing workspace".
  if (ok && request.intent === "restore_workspace" && setters.setAgentctlReady) {
    setters.setAgentctlReady(sessionId);
  }
  return ok;
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
): Promise<boolean> {
  setters.setResumptionState("resuming");
  const resumeAttempt = await tryLaunch(
    buildResumeRequest(taskId, sessionId).request,
    taskId,
    sessionId,
    session,
    setters,
  );
  if (resumeAttempt.ok) {
    setters.setNotice?.(null);
    return true;
  }
  // Resume failed (returned success=false OR threw). Fall back to read-only
  // workspace restore so the user keeps file/terminal/git access.
  const restoreAttempt = await tryLaunch(
    buildRestoreWorkspaceRequest(taskId, sessionId).request,
    taskId,
    sessionId,
    session,
    setters,
  );
  if (restoreAttempt.ok) {
    setters.setError(null);
    setters.setNotice?.(
      t("task:resumeFailedWorkspaceReadOnly", { error: resumeAttempt.error.message }),
    );
    return true;
  }
  setters.setResumptionState("error");
  setters.setNotice?.(null);
  setters.setError(
    t("task:resumeAndRestoreFailed", {
      resumeError: resumeAttempt.error.message,
      restoreError: restoreAttempt.error.message,
    }),
  );
  return false;
}

type LaunchAttempt = { ok: true } | { ok: false; error: Error };

/** Run a single launch attempt and retain its failure for the fallback notice.
 *  Logs caught errors to the console so silent fallback paths remain debuggable
 *  (errors otherwise vanish into the fallback state). */
async function tryLaunch(
  request: import("@/lib/services/session-launch-service").LaunchSessionRequest,
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter,
): Promise<LaunchAttempt> {
  try {
    const resp = await launchSession(request);
    if (!resp.success) {
      return {
        ok: false,
        error: new Error(
          resp.error ??
            (request.intent === "restore_workspace"
              ? t("task:failedToRestoreWorkspace")
              : t("task:failedToResumeSession")),
        ),
      };
    }
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
    // See comment in resumeViaLaunch — restore_workspace success implies
    // agentctl HTTP is ready, but the existing execution may not re-emit the
    // agentctl_ready WS event after WS reconnect.
    if (request.intent === "restore_workspace" && setters.setAgentctlReady) {
      setters.setAgentctlReady(sessionId);
    }
    return { ok: true };
  } catch (err) {
    console.error("[tryLaunch] session launch failed", {
      intent: request.intent,
      sessionId,
      err,
    });
    return {
      ok: false,
      error: err instanceof Error ? err : new Error(t("common:unknownError")),
    };
  }
}

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
    setters.setError(err instanceof Error ? err.message : t("common:unknownError"));
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

/** Extracted effects: reset state on session/task change, auto-check/resume, and remote retry. */
function useSessionResetAndCheck({
  taskId,
  sessionId,
  connectionStatus,
  session,
  setters,
  preventAutoStart,
}: ResetAndCheckParams): SessionResetAndCheckResult {
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
    setters.setNotice?.(null);
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
      preventAutoStart,
      setSessionStatus: (s) => {
        if (activeSessionRef.current === sessionId) setSessionStatus({ id: sessionId, status: s });
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
    setNotice: (notice) => {
      if (guard()) setters.setNotice?.(notice);
    },
    setWorktreePath: (p) => {
      if (guard()) setters.setWorktreePath(p);
    },
    setWorktreeBranch: (b) => {
      if (guard()) setters.setWorktreeBranch(b);
    },
    // The remaining setters write store state keyed by session id, but a
    // stale async callback completing after navigation can still touch the
    // previous session's row (e.g. re-mark it resume-skipped or overwrite its
    // status). Guard them all so a switched-away session is never mutated.
    setTaskSession: (s) => {
      if (guard()) setters.setTaskSession(s);
    },
    setAgentctlReady: (sid) => {
      if (guard()) setters.setAgentctlReady?.(sid);
    },
    setResumeSkipped: (sid, skipped) => {
      if (guard()) setters.setResumeSkipped?.(sid, skipped);
    },
  };
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
      setError(err instanceof Error ? err.message : t("common:unknownError"));
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
