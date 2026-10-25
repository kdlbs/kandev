import { demoQueueStatus } from "./agent-queue-runtime";
import { cancelSessionRun, startAgent } from "./agent-run-runtime";
import { notifyGitStatus } from "./git-runtime";
import {
  DEMO_ENVIRONMENT_ID,
  findTask,
  notify,
  persist,
  respond,
  state,
  TASK_UPDATED_EVENT,
  taskEvent,
} from "./runtime-state";

export function routeTaskSubscribe(
  socketId: string,
  id: string,
  action: string,
  _payload: Record<string, unknown>,
): boolean {
  if (
    [
      "task.subscribe",
      "task.unsubscribe",
      "user.subscribe",
      "user.unsubscribe",
      "session.unsubscribe",
      "run.subscribe",
      "run.unsubscribe",
      "system.metrics.subscribe",
      "system.metrics.unsubscribe",
    ].includes(action)
  ) {
    respond(socketId, id, { success: true });
    return true;
  }
  return false;
}

export function routeSessionStop(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (["session.stop", "agent.cancel", "orchestrator.stop"].includes(action)) {
    const sessionId =
      action === "orchestrator.stop"
        ? findTask(String(payload.task_id))?.primary_session_id
        : payload.session_id;
    const session = state.sessions.find((item) => item.id === sessionId);
    if (!session) {
      // i18n-exempt: demo protocol fixtures retain backend wire errors and seeded prompts
      respond(socketId, id, { message: "Session not found" }, true);
      return true;
    }
    cancelSessionRun(session.id);
    const oldState = session.state;
    session.state = "IDLE";
    session.updated_at = new Date().toISOString();
    const task = findTask(session.task_id);
    if (task?.primary_session_id === session.id) {
      task.primary_session_state = "IDLE";
      task.updated_at = session.updated_at;
      notify(TASK_UPDATED_EVENT, taskEvent(task));
    }
    notify("session.state_changed", {
      task_id: session.task_id,
      session_id: session.id,
      old_state: oldState,
      new_state: session.state,
      updated_at: session.updated_at,
    });
    notify("message.queue.status_changed", demoQueueStatus(session));
    persist();
    respond(socketId, id, { success: true });
    return true;
  }
  return false;
}

export function routeSessionSubscribe(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "session.subscribe") {
    const sessionId = String(payload.session_id || "");
    const session = state.sessions.find((item) => item.id === sessionId);
    respond(socketId, id, { success: true });
    if (session) {
      notify("session.agentctl_ready", {
        task_id: session.task_id,
        session_id: session.id,
        task_environment_id: session.task_environment_id ?? DEMO_ENVIRONMENT_ID,
        agent_execution_id: `demo-execution-${session.task_id}`,
        worktree_id: `demo-worktree-${session.task_id}`,
        worktree_path: session.worktree_path,
        worktree_branch: session.worktree_branch,
      });
      notifyGitStatus(session.id);
    }
    return true;
  }
  return false;
}

export function routeSessionFocus(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "session.focus") {
    const sessionId = String(payload.session_id || "");
    respond(socketId, id, { success: true });
    notifyGitStatus(sessionId);
    return true;
  }
  return false;
}

export function routeSessionUnfocus(
  socketId: string,
  id: string,
  action: string,
  _payload: Record<string, unknown>,
): boolean {
  if (action === "session.unfocus") {
    respond(socketId, id, { success: true });
    return true;
  }
  return false;
}

export function routeSessionEnsure(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "session.ensure" || action === "session.launch") {
    const task = findTask(String(payload.task_id));
    if (!task) {
      // i18n-exempt: demo protocol fixtures retain backend wire errors and seeded prompts
      respond(socketId, id, { message: "Task not found" }, true);
      return true;
    }
    const existing = state.sessions.find((session) => session.task_id === task.id);
    // i18n-exempt: demo agent prompt uses seeded English fixture content
    const session =
      existing ??
      startAgent(task, String(payload.prompt || task.description || "Implement this task"));
    respond(socketId, id, {
      success: true,
      task_id: task.id,
      session_id: session.id,
      agent_execution_id: `demo-execution-${task.id}`,
      state: session.state,
      source: existing ? "existing_primary" : "created_start",
      newly_created: !existing,
    });
    if (!existing) persist();
    return true;
  }
  return false;
}
