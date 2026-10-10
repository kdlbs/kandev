import { type Task } from "@/lib/types/http";
import { cancelSessionRun, startAgent } from "./agent-run-runtime";
import { routeDemoDiscovery } from "./discovery-runtime";
import {
  routeBootHttp,
  routeGitHubAccountHttp,
  routeGitHubSettingsHttp,
  routeTaskHttp,
  routeMovedTaskHttp,
  routeSessionHttp,
  routeWorkspaceHttp,
  routeRepositoryHttp,
  routeIntegrationSettingsHttp,
  routeGitHubRepositoryHttp,
} from "./http-routes";
import { type DemoHttpRequest, type DemoHttpResponse } from "./protocol";
import {
  empty,
  fileReviewsBySession,
  findTask,
  jiraRuntime,
  json,
  messageEvent,
  notify,
  parseBody,
  persist,
  plansByTask,
  state,
  systemRuntime,
  TASK_UPDATED_EVENT,
  taskEvent,
  workflowRuntime,
} from "./runtime-state";
import { createTaskFromInput } from "./scenario";

export async function handleHttp(request: DemoHttpRequest): Promise<DemoHttpResponse> {
  try {
    return await routeHttp(request);
  } catch (error) {
    return json(
      { error: error instanceof Error ? error.message : String(error), demo_mode: true },
      500,
    );
  }
}

// i18n-exempt: browser demo fixture data is intentionally literal demo content
export function createTask(input: Record<string, unknown>): DemoHttpResponse {
  const task = createTaskFromInput(state, input);
  state.tasks.push(task);
  if (input.start_agent) {
    const session = startAgent(task, String(input.description || "Build this task"));
    persist();
    notify("task.created", taskEvent(task));
    return json({
      ...task,
      session_id: session.id,
      agent_execution_id: `demo-execution-${task.id}`,
    });
  }
  persist();
  notify("task.created", taskEvent(task));
  return json(task, 201);
}

export function updateTask(task: Task, input: Record<string, unknown>): DemoHttpResponse {
  if (typeof input.title === "string") task.title = input.title;
  if (typeof input.description === "string") task.description = input.description;
  if (typeof input.workflow_step_id === "string") task.workflow_step_id = input.workflow_step_id;
  if (typeof input.state === "string") task.state = input.state as Task["state"];
  task.updated_at = new Date().toISOString();
  persist();
  notify(TASK_UPDATED_EVENT, taskEvent(task));
  return json(task);
}

export function removeTask(task: Task): DemoHttpResponse {
  for (const session of state.sessions.filter((item) => item.task_id === task.id)) {
    cancelSessionRun(session.id);
    delete state.messagesBySession[session.id];
    fileReviewsBySession.delete(session.id);
  }
  delete plansByTask[task.id];
  state.tasks = state.tasks.filter((item) => item.id !== task.id);
  state.sessions = state.sessions.filter((session) => session.task_id !== task.id);
  delete state.taskPRs[task.id];
  persist();
  notify("task.deleted", taskEvent(task));
  return empty();
}

export function answerClarification(
  pendingId: string,
  input: Record<string, unknown>,
): DemoHttpResponse {
  const messages = Object.values(state.messagesBySession)
    .flat()
    .filter((m) => m.type === "clarification_request" && m.metadata?.pending_id === pendingId);
  if (!messages.length) return json({ error: "Question not found" }, 404);
  if (!Array.isArray(input.answers) || !input.answers.length)
    return json({ error: "Answers are required" }, 400);
  for (const message of messages) {
    message.metadata = { ...message.metadata, status: "answered", answers: input.answers };
    message.requests_input = false;
    message.updated_at = new Date().toISOString();
    notify("session.message.updated", messageEvent(message));
  }
  const task = findTask(messages[0].task_id);
  if (task) {
    task.primary_session_pending_action = null;
    task.primary_session_state = "IDLE";
    notify(TASK_UPDATED_EVENT, taskEvent(task));
  }
  const session = state.sessions.find((s) => s.id === messages[0].session_id);
  if (session) {
    const oldState = session.state;
    session.state = "IDLE";
    notify("session.state_changed", {
      task_id: session.task_id,
      session_id: session.id,
      old_state: oldState,
      new_state: "IDLE",
    });
  }
  persist();
  return json({ success: true });
}

export async function routeHttp(request: DemoHttpRequest): Promise<DemoHttpResponse> {
  const url = new URL(request.path, "https://demo.kandev.com");
  const path = url.pathname;
  const method = request.method.toUpperCase();
  const input = parseBody(request.body);

  const discoveryResponse = routeDemoDiscovery(path, method, url.searchParams);
  if (discoveryResponse) return discoveryResponse;
  const jiraResponse = jiraRuntime.route(path, method, url.searchParams, input);
  if (jiraResponse) return jiraResponse;

  const systemResponse = systemRuntime.route({ path, method, input });
  if (systemResponse) return systemResponse;
  const workflowResponse = workflowRuntime.route({
    path,
    method,
    input,
    rawBody: request.body,
    searchParams: url.searchParams,
  });
  if (workflowResponse) return workflowResponse;
  for (const handler of [
    routeBootHttp,
    routeGitHubAccountHttp,
    routeRepositoryHttp,
    routeWorkspaceHttp,
    routeGitHubRepositoryHttp,
    routeIntegrationSettingsHttp,
    routeGitHubSettingsHttp,
    routeTaskHttp,
    routeMovedTaskHttp,
    routeSessionHttp,
  ]) {
    const response = handler(path, method, input, url);
    if (response) return response;
  }

  if (method === "GET") return json({ demo_mode: true, unsupported: path }, 501);
  return json({ error: "This action is disabled in the browser demo", demo_mode: true }, 501);
}
