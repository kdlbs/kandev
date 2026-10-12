import { demoGitData, serializeFileReviews } from "./git-runtime";
import { handlePlanRequest } from "./plan-runtime";
import {
  fileReviewsBySession,
  findTask,
  messageEvent,
  notify,
  persist,
  respond,
  state,
  TASK_UPDATED_EVENT,
  taskEvent,
} from "./runtime-state";

export function routePermissionRespond(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "permission.respond") {
    const pendingId = String(payload.pending_id || "");
    const sessionId = String(payload.session_id || "");
    const permission = (state.messagesBySession[sessionId] ?? []).find(
      (message) =>
        message.type === "permission_request" &&
        (message.metadata as { pending_id?: string } | undefined)?.pending_id === pendingId,
    );
    if (!permission) {
      // i18n-exempt: demo protocol fixtures retain backend wire errors and seeded prompts
      respond(socketId, id, { message: "Permission request not found" }, true);
      return true;
    }

    let status = "approved";
    if (payload.cancelled) status = "expired";
    else if (payload.rejected) status = "rejected";
    const now = new Date().toISOString();
    permission.metadata = { ...permission.metadata, status };
    permission.requests_input = false;
    permission.updated_at = now;

    const session = state.sessions.find((candidate) => candidate.id === sessionId);
    const task = session ? findTask(session.task_id) : undefined;
    if (session) {
      const oldState = session.state;
      session.state = "IDLE";
      session.updated_at = now;
      notify("session.state_changed", {
        task_id: session.task_id,
        session_id: session.id,
        old_state: oldState,
        new_state: session.state,
        updated_at: now,
      });
    }
    if (task) {
      task.primary_session_state = "IDLE";
      task.primary_session_pending_action = null;
      task.updated_at = now;
      notify(TASK_UPDATED_EVENT, taskEvent(task));
    }

    persist();
    respond(socketId, id, { success: true, status });
    notify("session.message.updated", messageEvent(permission));
    return true;
  }
  return false;
}

export function routeTaskPlan(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action.startsWith("task.plan.")) {
    handlePlanRequest(socketId, id, action, payload);
    return true;
  }
  return false;
}

export function routeSessionGitCommits(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "session.git.commits") {
    const sessionId = String(payload.session_id || "");
    respond(socketId, id, { commits: demoGitData(sessionId).commits, ready: true });
    return true;
  }
  return false;
}

export function routeSessionCumulativeDiff(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "session.cumulative_diff") {
    const sessionId = String(payload.session_id || "");
    respond(socketId, id, { cumulative_diff: demoGitData(sessionId).cumulativeDiff, ready: true });
    return true;
  }
  return false;
}

export function routeSessionFileReviewGet(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "session.file_review.get") {
    const sessionId = String(payload.session_id || "");
    respond(socketId, id, { reviews: serializeFileReviews(sessionId) });
    return true;
  }
  return false;
}

export function routeSessionFileReviewUpdate(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "session.file_review.update") {
    const sessionId = String(payload.session_id || "");
    const filePath = String(payload.file_path || "");
    const reviews = fileReviewsBySession.get(sessionId) ?? new Map();
    reviews.set(filePath, {
      reviewed: payload.reviewed === true,
      diffHash: String(payload.diff_hash || ""),
    });
    fileReviewsBySession.set(sessionId, reviews);
    respond(socketId, id, { success: true });
    return true;
  }
  return false;
}

export function routeSessionFileReviewReset(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "session.file_review.reset") {
    fileReviewsBySession.delete(String(payload.session_id || ""));
    respond(socketId, id, { success: true });
    return true;
  }
  return false;
}
