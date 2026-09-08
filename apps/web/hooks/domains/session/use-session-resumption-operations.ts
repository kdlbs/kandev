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

export type SessionRecoveryFailure = {
  resumeError: string;
  restoreError: string;
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
  getLiveSession?: (sessionId: string) => SessionLike;
  /** Retains the causes from automatic resume and workspace-restore attempts. */
  setRecoveryFailure?: (failure: SessionRecoveryFailure | null) => void;
  /** Refreshes the owning task after a typed archive conflict. */
  onTaskArchiveConflict?: () => void;
};

export type SessionLike = { started_at?: string; updated_at?: string; state?: string } | null;

type ResumeResponse = {
  success: boolean;
  state?: string;
  worktree_path?: string;
  worktree_branch?: string;
  error?: string;
};

type LaunchAttempt = { ok: true } | { ok: false; error: Error; archived?: boolean };

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

/** Attempt resume, silently falling back to restore_workspace on any failure. */
export async function resumeWithSilentFallback(
  taskId: string,
  sessionId: string,
  session: SessionLike,
  setters: ResumeStateSetter,
  canContinue: () => boolean = () => true,
): Promise<boolean> {
  if (!canContinue()) return false;
  setters.setResumptionState("resuming");
  setters.setRecoveryFailure?.(null);
  const context = { taskId, sessionId, session, setters, canContinue };
  const resumeAttempt = await tryLaunch(buildResumeRequest(taskId, sessionId).request, context);
  if (!canContinue()) return false;
  if (resumeAttempt.ok) {
    setters.setNotice?.(null);
    return true;
  }
  if (resumeAttempt.archived) {
    clearArchiveRecovery(setters);
    return false;
  }
  if (!canContinue()) return false;
  const restoreAttempt = await tryLaunch(
    buildRestoreWorkspaceRequest(taskId, sessionId).request,
    context,
  );
  if (!canContinue()) return false;
  if (restoreAttempt.ok) {
    setters.setError(null);
    setters.setNotice?.(
      t("task:resumeFailedWorkspaceReadOnly", { error: resumeAttempt.error.message }),
    );
    return true;
  }
  if (restoreAttempt.archived) {
    clearArchiveRecovery(setters);
    return false;
  }
  setters.setResumptionState("error");
  setters.setNotice?.(null);
  setters.setRecoveryFailure?.({
    resumeError: resumeAttempt.error.message,
    restoreError: restoreAttempt.error.message,
  });
  setters.setError(t("task:sessionRecoveryFailed"));
  return false;
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
  if (preventAutoStart && status.needs_resume && status.is_resumable) return "skip";
  if (status.needs_resume && status.is_resumable) return "resume";
  if (status.needs_workspace_restore) return "restore";
  return "idle";
}

export { resumeViaLaunch, TASK_ARCHIVED_KIND };
