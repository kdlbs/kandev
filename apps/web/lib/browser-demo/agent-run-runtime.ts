import { type Message, type Task, type TaskSession } from "@/lib/types/http";
import { makeAgentRunMessages } from "./agent-run-messages";
import { notifyGitStatus } from "./git-runtime";
import { makeDemoReplyMessages } from "./reply-messages";
import {
  acceptedQueueMessages,
  activeRuns,
  messageEvent,
  notify,
  pendingReplies,
  persist,
  queuedMessages,
  runTimers,
  state,
  TASK_UPDATED_EVENT,
  taskEvent,
  workflowRuntime,
} from "./runtime-state";
import { makeMessage, makeSession } from "./scenario";

export function startAgent(task: Task, prompt: string) {
  const sessionId = `demo-session-${task.id}`;
  const session = makeSession(sessionId, task.id, "RUNNING", task.repositories);
  state.sessions.push(session);
  task.primary_session_id = session.id;
  task.primary_session_state = "RUNNING";
  task.session_count = 1;
  task.state = "IN_PROGRESS";
  const steps = workflowRuntime
    .snapshot()
    .steps.filter((step) => step.workflow_id === task.workflow_id && step.stage_type === "work");
  task.workflow_step_id =
    (steps.find((step) => !step.is_start_step) ?? steps[0])?.id ?? task.workflow_step_id;
  const user = makeMessage(`${sessionId}-user`, sessionId, task.id, "user", prompt);
  state.messagesBySession[sessionId] = [];
  appendMessage(user);
  notify(TASK_UPDATED_EVENT, taskEvent(task));
  activeRuns.add(session.id);
  scheduleAgentRun(task, session);
  return session;
}

export function scheduleAgentRun(task: Task, session: TaskSession) {
  const messages = makeAgentRunMessages(task, session.id);
  messages.forEach((message, index) => {
    runTimers.schedule(
      session.id,
      () => {
        if (!state.tasks.includes(task) || !state.sessions.includes(session)) return;
        appendMessage(message);
        if (index === messages.length - 1) finishAgentRun(task, session);
        persist();
      },
      (index + 1) * 450,
    );
  });
}

export function finishAgentRun(task: Task, session: TaskSession) {
  activeRuns.delete(session.id);
  const pending = pendingReplies.get(session.id)?.shift();
  if (pending) {
    scheduleReply(task, session, pending);
    return;
  }
  const now = new Date().toISOString();
  const oldSessionState = session.state;
  session.state = "IDLE";
  session.updated_at = now;
  task.state = "REVIEW";
  task.workflow_step_id = completedRunStep(task);
  task.primary_session_state = "IDLE";
  task.review_status = "pending";
  task.updated_at = now;
  notify("session.state_changed", {
    task_id: task.id,
    session_id: session.id,
    old_state: oldSessionState,
    new_state: session.state,
    updated_at: now,
  });
  notify(TASK_UPDATED_EVENT, taskEvent(task));
  notifyGitStatus(session.id);
}

export function appendMessage(message: Message) {
  const messages = (state.messagesBySession[message.session_id] ??= []);
  const lastTime = Date.parse(messages.at(-1)?.created_at ?? "") || 0;
  const now = new Date(Math.max(Date.now(), lastTime + 1)).toISOString();
  message.created_at = now;
  message.updated_at = now;
  messages.push(message);
  notify("session.message.added", messageEvent(message));
}

export function cancelSessionRun(sessionId: string) {
  runTimers.cancel(sessionId);
  activeRuns.delete(sessionId);
  pendingReplies.delete(sessionId);
  for (const [id, entry] of acceptedQueueMessages) {
    if (entry.session_id !== sessionId) continue;
    queuedMessages.delete(id);
    acceptedQueueMessages.delete(id);
  }
}

export function enqueueReply(task: Task, session: TaskSession, user: Message) {
  if (activeRuns.has(session.id)) {
    const pending = pendingReplies.get(session.id) ?? [];
    pending.push(user);
    pendingReplies.set(session.id, pending);
  } else {
    scheduleReply(task, session, user);
  }
}

export function scheduleReply(task: Task, session: TaskSession, user: Message) {
  activeRuns.add(session.id);
  const oldState = session.state;
  session.state = "RUNNING";
  session.updated_at = new Date().toISOString();
  task.primary_session_state = "RUNNING";
  task.updated_at = session.updated_at;
  notify("session.state_changed", {
    session_id: session.id,
    task_id: task.id,
    old_state: oldState,
    new_state: "RUNNING",
    updated_at: session.updated_at,
  });
  notify(TASK_UPDATED_EVENT, taskEvent(task));
  const previous = state.messagesBySession[session.id]?.findLast(
    (message) => typeof message.metadata?.demo_reply_variant === "number",
  );
  const messages = makeDemoReplyMessages(
    task,
    session,
    user,
    previous?.metadata?.demo_reply_variant as number | undefined,
  );
  messages.forEach((message, index) => {
    runTimers.schedule(
      session.id,
      () => {
        if (!state.tasks.includes(task) || !state.sessions.includes(session)) return;
        appendMessage(message);
        if (index === messages.length - 1) finishAgentRun(task, session);
        persist();
      },
      [300, 900, 1800][index],
    );
  });
}

export function completedRunStep(task: Task): string {
  const steps = workflowRuntime
    .snapshot()
    .steps.filter((s) => s.workflow_id === task.workflow_id)
    .sort((a, b) => a.position - b.position);
  const current = steps.find((s) => s.id === task.workflow_step_id);
  const event = current?.events?.on_turn_complete?.find((e) =>
    ["move_to_step", "move_to_next", "move_to_previous"].includes(e.type),
  );
  if (event?.type === "move_to_step")
    return steps.find((s) => s.id === event.config?.step_id)?.id ?? task.workflow_step_id;
  if (event) {
    const index = steps.findIndex((s) => s.id === task.workflow_step_id);
    return steps[index + (event.type === "move_to_next" ? 1 : -1)]?.id ?? task.workflow_step_id;
  }
  return steps.find((s) => s.stage_type === "review")?.id ?? task.workflow_step_id;
}
