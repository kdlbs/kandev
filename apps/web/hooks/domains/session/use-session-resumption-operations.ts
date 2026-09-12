import { launchSession, type LaunchSessionRequest } from "@/lib/services/session-launch-service";
import {
  buildResumeRequest,
  buildRestoreWorkspaceRequest,
} from "@/lib/services/session-launch-helpers";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type SessionId,
  type TaskId,
  type TaskSessionState,
} from "@/lib/types/http";
import { isLaunchStateRegression } from "@/lib/session-state";
import { t } from "@/lib/i18n";
import { WebSocketRequestError } from "@/lib/ws/client";

export type TaskArchiveState = boolean | null;

// i18n-exempt: backend conflict discriminator, not user-facing copy.
const TASK_ARCHIVED_KIND = "task_archived";

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

export type SessionRecoveryFailure =
  | {
      outcome: "workspace_read_only";
      resumeError: string;
    }
  | {
      outcome: "recovery_failed";
      resumeError: string;
      restoreError: string;
    }
  | {
      outcome: "status_unavailable";
      kind: "timeout" | "request";
      statusError: string;
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
    queue_incarnation_id?: string;
    resume_projection_id?: string;
  }) => void;
  /** Updates the captured session row even when the active view has changed. */
  setTaskSessionUnscoped?: ResumeStateSetter["setTaskSession"];
  setAgentctlReady?: (sessionId: string) => void;
  /** Records/clears the resume-skipped marker (prevent-auto-start-on-open). */
  setResumeSkipped?: (sessionId: string, skipped: boolean) => void;
  /** Reads the session row live from the store (monotonic hydration guard). */
  getLiveSession?: (sessionId: string) => SessionLike;
  /** Retains the causes from automatic resume and workspace-restore attempts. */
  setRecoveryFailure?: (failure: SessionRecoveryFailure | null) => void;
  /** Refreshes the owning task after a typed archive conflict. */
  onTaskArchiveConflict?: () => void;
};

export type SessionLike = {
  started_at?: string;
  updated_at?: string;
  state?: string;
  queue_incarnation_id?: string;
  resume_projection_id?: string;
} | null;

const TASK_SESSION_STATES = new Set<TaskSessionState>([
  "CREATED",
  "STARTING",
  "RUNNING",
  "IDLE",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
]);

function asTaskSessionState(state: string | undefined): TaskSessionState | null {
  return state && TASK_SESSION_STATES.has(state as TaskSessionState)
    ? (state as TaskSessionState)
    : null;
}

export type ResumeStartingProjection = {
  rollback: () => void;
};

let resumeProjectionSequence = 0;

/** Publish the persisted lifecycle state locally while the resume request is in flight. */
export function markSessionStarting(
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter,
): ResumeStartingProjection | null {
  const liveSession = setters.getLiveSession?.(sessionId);
  const previousSession = liveSession ?? session;
  if (
    previousSession?.state === "STARTING" ||
    isLaunchStateRegression(previousSession?.state, "STARTING")
  ) {
    return null;
  }

  // Resume targets are existing sessions. If hydration has not supplied a
  // prior state yet, FAILED is the safe recovery state after both launch
  // attempts fail; do not leave the optimistic STARTING row stranded.
  const previousState = asTaskSessionState(previousSession?.state) ?? "FAILED";
  const projectionId = `resume-projection-${++resumeProjectionSequence}`;
  const sessionIncarnationId = previousSession?.queue_incarnation_id;
  setters.setTaskSession({
    id: toSessionId(sessionId),
    task_id: toTaskId(taskId),
    state: "STARTING",
    started_at: previousSession?.started_at ?? "",
    updated_at: previousSession?.updated_at ?? "",
    ...(sessionIncarnationId ? { queue_incarnation_id: sessionIncarnationId } : {}),
    resume_projection_id: projectionId,
  });

  let rollbackAttempted = false;
  return {
    rollback: () => {
      if (rollbackAttempted) return;
      rollbackAttempted = true;
      const currentSession = setters.getLiveSession?.(sessionId);
      if (
        setters.getLiveSession &&
        (currentSession?.state !== "STARTING" ||
          currentSession.resume_projection_id !== projectionId ||
          currentSession.queue_incarnation_id !== sessionIncarnationId)
      ) {
        return;
      }
      (setters.setTaskSessionUnscoped ?? setters.setTaskSession)({
        id: toSessionId(sessionId),
        task_id: toTaskId(taskId),
        state: previousState,
        started_at: previousSession?.started_at ?? "",
        updated_at: previousSession?.updated_at ?? "",
        ...(sessionIncarnationId ? { queue_incarnation_id: sessionIncarnationId } : {}),
      });
    },
  };
}

type ResumeResponse = {
  success: boolean;
  state?: string;
  worktree_path?: string;
  worktree_branch?: string;
  error?: string;
};

type LaunchAttempt = { ok: true } | { ok: false; error: Error; archived?: boolean };
type FailedLaunchAttempt = Extract<LaunchAttempt, { ok: false }>;

export function isTaskArchivedConflict(error: unknown): boolean {
  return error instanceof WebSocketRequestError && error.details?.kind === TASK_ARCHIVED_KIND;
}

export function clearArchiveRecovery(setters: ResumeStateSetter): void {
  setters.setResumptionState("idle");
  setters.setError(null);
  setters.setNotice?.(null);
  setters.setRecoveryFailure?.(null);
  setters.onTaskArchiveConflict?.();
}

/** Apply a successful resume response to local state. */
function applyResumeResponse(
  resp: ResumeResponse,
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter,
): boolean {
  if (resp.success) {
    setters.setRecoveryFailure?.(null);
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
  setters.setRecoveryFailure?.(null);
  setters.setResumptionState("error");
  setters.setError(resp.error ?? t("task:failedToResumeSession"));
  return false;
}

type ResumeLaunchContext = {
  taskId: string;
  sessionId: string;
  session: SessionLike;
  setters: ResumeStateSetter;
  canContinue: () => boolean;
};

/** Launch a session via a request builder and apply the response. */
async function resumeViaLaunch(
  buildRequest: (taskId: string, sessionId: string) => { request: LaunchSessionRequest },
  context: ResumeLaunchContext,
): Promise<boolean> {
  const { taskId, sessionId, session, setters, canContinue } = context;
  if (!canContinue()) return false;
  setters.setResumptionState("resuming");
  setters.setRecoveryFailure?.(null);
  const { request } = buildRequest(taskId, sessionId);
  let launchResp: Awaited<ReturnType<typeof launchSession>>;
  try {
    launchResp = await launchSession(request);
  } catch (error) {
    if (isTaskArchivedConflict(error)) {
      clearArchiveRecovery(setters);
      return false;
    }
    throw error;
  }
  if (!canContinue()) return false;
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
  // otherwise-idle session. A successful response means the workspace and
  // agentctl are ready even when a reconnect dropped the readiness event.
  if (ok && request.intent === "restore_workspace" && setters.setAgentctlReady) {
    setters.setAgentctlReady(sessionId);
  }
  return ok;
}

async function restoreAfterResumeFailure(
  context: ResumeLaunchContext,
  resumeAttempt: FailedLaunchAttempt,
): Promise<boolean> {
  const { taskId, sessionId, setters, canContinue } = context;
  if (!canContinue()) return false;
  const restoreAttempt = await tryLaunch(
    buildRestoreWorkspaceRequest(taskId, sessionId).request,
    context,
  );
  if (!canContinue()) return false;
  if (restoreAttempt.ok) {
    setters.setError(null);
    setters.setNotice?.(t("task:resumeFailedWorkspaceReadOnly"));
    setters.setRecoveryFailure?.({
      outcome: "workspace_read_only",
      resumeError: resumeAttempt.error.message,
    });
    return true;
  }
  if (restoreAttempt.archived) {
    clearArchiveRecovery(setters);
    return false;
  }
  setters.setResumptionState("error");
  setters.setNotice?.(null);
  setters.setRecoveryFailure?.({
    outcome: "recovery_failed",
    resumeError: resumeAttempt.error.message,
    restoreError: restoreAttempt.error.message,
  });
  setters.setError(t("task:sessionRecoveryFailed"));
  return false;
}

/** Attempt resume, silently falling back to restore_workspace on any failure. */
export async function resumeWithSilentFallback(
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter,
  canContinue: () => boolean = () => true,
): Promise<boolean> {
  if (!canContinue()) return false;
  const startingProjection = markSessionStarting(taskId, sessionId, session, setters);
  setters.setResumptionState("resuming");
  setters.setRecoveryFailure?.(null);
  const context = { taskId, sessionId, session, setters, canContinue };
  const resumeAttempt = await tryLaunch(buildResumeRequest(taskId, sessionId).request, context);
  if (!canContinue()) {
    startingProjection?.rollback();
    return false;
  }
  if (resumeAttempt.ok) {
    setters.setNotice?.(null);
    return true;
  }
  if (resumeAttempt.archived) {
    clearArchiveRecovery(setters);
    startingProjection?.rollback();
    return false;
  }
  const restored = await restoreAfterResumeFailure(context, resumeAttempt);
  if (!restored) startingProjection?.rollback();
  return restored;
}

/** Run a single launch attempt and retain its failure for the fallback notice. */
async function tryLaunch(
  request: LaunchSessionRequest,
  context: ResumeLaunchContext,
): Promise<LaunchAttempt> {
  const { sessionId, session, setters, canContinue } = context;
  if (!canContinue()) return { ok: false, error: new Error() };
  try {
    const resp = await launchSession(request);
    if (!canContinue()) return { ok: false, error: new Error() };
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
      context.taskId,
      sessionId,
      session,
      setters,
    );
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
      archived: isTaskArchivedConflict(err),
    };
  }
}

export type ResumeAction = "running" | "skip" | "resume" | "restore" | "idle";

export function decideResumeAction(status: SessionStatus, preventAutoStart: boolean): ResumeAction {
  if (status.is_agent_running) return "running";
  // Completed sessions remain passive until the user explicitly chooses the
  // completed-chat Resume action. Open-time recovery must not revive them.
  if (status.state === "COMPLETED") return "idle";
  if (preventAutoStart && status.needs_resume && status.is_resumable) return "skip";
  if (status.needs_resume && status.is_resumable) return "resume";
  if (status.needs_workspace_restore) return "restore";
  return "idle";
}

export { resumeViaLaunch, TASK_ARCHIVED_KIND };
