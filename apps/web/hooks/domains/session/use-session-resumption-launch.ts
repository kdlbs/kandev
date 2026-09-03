import { launchSession } from "@/lib/services/session-launch-service";
import {
  buildResumeRequest,
  buildRestoreWorkspaceRequest,
} from "@/lib/services/session-launch-helpers";
import {
  sessionRecoveryGuardDetails,
  sessionRecoveryGuardMessage,
} from "@/lib/services/session-recovery-service";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type TaskSessionState,
} from "@/lib/types/http";
import { t } from "@/lib/i18n";
import type { ResumeStateSetter, SessionLike } from "./use-session-resumption";

type ResumeResponse = {
  success: boolean;
  state?: string;
  worktree_path?: string;
  worktree_branch?: string;
  error?: string;
};

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
export async function resumeViaLaunch(
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
  // The startup recovery guard refuses every launch for this session, so a
  // restore_workspace fallback would fail identically. Skip it and show the
  // guard's own distinct, retryable-or-not message instead of the generic
  // "resume and restore both failed" combination.
  const resumeGuardDetails = sessionRecoveryGuardDetails(resumeAttempt.error);
  if (resumeGuardDetails) {
    setters.setResumptionState("error");
    setters.setNotice?.(null);
    setters.setError(sessionRecoveryGuardMessage(resumeGuardDetails, t));
    return false;
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
