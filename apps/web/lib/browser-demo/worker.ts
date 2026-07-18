/// <reference lib="webworker" />

/* eslint-disable complexity, max-lines-per-function, sonarjs/cognitive-complexity, sonarjs/no-duplicate-string */

import type { Message, Task } from "@/lib/types/http";
import type {
  DemoHttpRequest,
  DemoHttpResponse,
  DemoWorkerRequest,
  DemoWorkerResponse,
} from "./protocol";
import {
  DEMO_IDS,
  DEMO_SCENARIO_VERSION,
  createBootPayload,
  createDemoState,
  createTaskFromInput,
  demoGitHubPR,
  demoRepository,
  demoSteps,
  demoWorkflow,
  makeMessage,
  makeSession,
  type DemoState,
} from "./scenario";

const scope: DedicatedWorkerGlobalScope = self as never;
let state = createDemoState();

scope.onmessage = (event: MessageEvent<DemoWorkerRequest>) => {
  const message = event.data;
  if (message.kind === "init") {
    state = restoreState(message.persistedState);
    post({ kind: "result", id: message.id, value: createBootPayload(state) });
    return;
  }
  if (message.kind === "http") {
    void handleHttp(message.request).then((response) =>
      post({ kind: "http-result", id: message.id, response }),
    );
    return;
  }
  if (message.kind === "ws-open") {
    post({ kind: "ws-event", socketId: message.socketId, event: "open" });
    return;
  }
  if (message.kind === "ws-close") {
    post({ kind: "ws-event", socketId: message.socketId, event: "close" });
    return;
  }
  if (message.kind === "ws-send") handleSocketRequest(message.socketId, message.data);
};

function restoreState(persisted?: string): DemoState {
  if (!persisted) return createDemoState();
  try {
    const parsed = JSON.parse(persisted) as DemoState;
    return parsed.version === DEMO_SCENARIO_VERSION ? parsed : createDemoState();
  } catch {
    return createDemoState();
  }
}

async function handleHttp(request: DemoHttpRequest): Promise<DemoHttpResponse> {
  const url = new URL(request.path, "https://demo.kandev.com");
  const path = url.pathname;
  const method = request.method.toUpperCase();
  const input = parseBody(request.body);

  if (path === "/health") return json({ status: "ok", mode: "browser-demo" });
  if (path === "/api/v1/features") return json({ office: false, plugins: false });
  if (path === "/api/v1/app-state") return json(createBootPayload(state));
  if (path === "/api/v1/workspaces")
    return json({
      workspaces: createBootPayload(state).initialState?.workspaces?.items ?? [],
      total: 1,
    });
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/repositories`)
    return json({ repositories: [demoRepository], total: 1 });
  if (path === `/api/v1/repositories/${DEMO_IDS.repository}/branches`)
    return json({ branches: [{ name: "main", type: "local" }], total: 1, current_branch: "main" });
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/repositories/discover`)
    return json({ roots: [], repositories: [], total: 0 });
  if (path === "/api/v1/workflows") return json({ workflows: [demoWorkflow], total: 1 });
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/workflows`)
    return json({ workflows: [demoWorkflow], total: 1 });
  if (path === `/api/v1/workflows/${DEMO_IDS.workflow}/snapshot`)
    return json({ workflow: demoWorkflow, steps: demoSteps, tasks: activeTasks() });
  if (path === `/api/v1/workflows/${DEMO_IDS.workflow}/workflow/steps`)
    return json({ steps: demoSteps, total: demoSteps.length });
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/workflow-steps`)
    return json({ steps: demoSteps, total: demoSteps.length });
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/tasks`)
    return json({ tasks: activeTasks(), total: activeTasks().length });
  if (path === "/api/v1/github/status")
    return json({
      configured: true,
      authenticated: true,
      auth_method: "gh_cli",
      username: "kandev-demo",
    });
  if (path === "/api/v1/github/user/prs")
    return json({ prs: [demoGitHubPR], total_count: 1, page: 1, per_page: 25 });
  if (path === "/api/v1/github/user/issues")
    return json({ issues: [], total_count: 0, page: 1, per_page: 25 });
  if (path === "/api/v1/github/workspace-settings") {
    return json({
      workspace_id: DEMO_IDS.workspace,
      repo_scope_mode: "repos",
      repo_scope_orgs: [],
      repo_scope_repos: [{ owner: "kandev-demo", name: "acme-web" }],
      saved_presets: [],
      default_query_presets: null,
      created_at: "2026-07-18T12:00:00.000Z",
      updated_at: "2026-07-18T12:00:00.000Z",
    });
  }
  if (path === "/api/v1/github/action-presets") {
    return json({ workspace_id: DEMO_IDS.workspace, pr: [], issue: [] });
  }
  if (path === "/api/v1/github/task-prs") return json({ task_prs: state.taskPRs });
  if (path === "/api/v1/github/watches/pr") return json({ watches: [] });
  if (path === "/api/v1/github/watches/review") return json({ watches: [] });
  if (path === "/api/v1/github/watches/issues") return json({ watches: [] });
  if (path === "/api/v1/tasks" && method === "POST") return createTask(input);

  const taskMatch = path.match(/^\/api\/v1\/tasks\/([^/]+)$/);
  if (taskMatch) {
    const task = findTask(taskMatch[1]);
    if (!task) return json({ error: "Task not found" }, 404);
    if (method === "GET") return json(task);
    if (method === "PATCH") return updateTask(task, input);
    if (method === "DELETE") return removeTask(task);
  }
  const moveMatch = path.match(/^\/api\/v1\/tasks\/([^/]+)\/move$/);
  if (moveMatch && method === "POST") {
    const task = findTask(moveMatch[1]);
    if (!task) return json({ error: "Task not found" }, 404);
    task.workflow_step_id = String(input.workflow_step_id || task.workflow_step_id);
    task.position = Number(input.position ?? task.position);
    task.updated_at = new Date().toISOString();
    persist();
    notify("task.updated", taskEvent(task));
    return json({
      task,
      workflow_step: demoSteps.find((step) => step.id === task.workflow_step_id),
    });
  }
  const archiveMatch = path.match(/^\/api\/v1\/tasks\/([^/]+)\/archive$/);
  if (archiveMatch && method === "POST") {
    const task = findTask(archiveMatch[1]);
    if (!task) return json({ error: "Task not found" }, 404);
    task.archived_at = new Date().toISOString();
    persist();
    notify("task.deleted", taskEvent(task));
    return empty();
  }
  const sessionsMatch = path.match(/^\/api\/v1\/tasks\/([^/]+)\/sessions$/);
  if (sessionsMatch) {
    const sessions = state.sessions.filter((session) => session.task_id === sessionsMatch[1]);
    return json({ sessions, total: sessions.length });
  }
  const sessionMatch = path.match(/^\/api\/v1\/task-sessions\/([^/]+)$/);
  if (sessionMatch) {
    const session = state.sessions.find((item) => item.id === sessionMatch[1]);
    return session ? json({ session }) : json({ error: "Session not found" }, 404);
  }
  const messagesMatch = path.match(/^\/api\/v1\/task-sessions\/([^/]+)\/messages$/);
  if (messagesMatch)
    return json({ messages: state.messagesBySession[messagesMatch[1]] ?? [], has_more: false });
  const turnsMatch = path.match(/^\/api\/v1\/task-sessions\/([^/]+)\/turns$/);
  if (turnsMatch) return json({ turns: [], total: 0 });

  if (method === "GET") return json({ demo_mode: true, unsupported: path }, 501);
  return json({ error: "This action is disabled in the browser demo", demo_mode: true }, 501);
}

function createTask(input: Record<string, unknown>): DemoHttpResponse {
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

function updateTask(task: Task, input: Record<string, unknown>): DemoHttpResponse {
  if (typeof input.title === "string") task.title = input.title;
  if (typeof input.description === "string") task.description = input.description;
  if (typeof input.workflow_step_id === "string") task.workflow_step_id = input.workflow_step_id;
  if (typeof input.state === "string") task.state = input.state as Task["state"];
  task.updated_at = new Date().toISOString();
  persist();
  notify("task.updated", taskEvent(task));
  return json(task);
}

function removeTask(task: Task): DemoHttpResponse {
  state.tasks = state.tasks.filter((item) => item.id !== task.id);
  state.sessions = state.sessions.filter((session) => session.task_id !== task.id);
  delete state.taskPRs[task.id];
  persist();
  notify("task.deleted", taskEvent(task));
  return empty();
}

function handleSocketRequest(socketId: string, raw: string) {
  let request: { id?: string; action?: string; payload?: Record<string, unknown> };
  try {
    request = JSON.parse(raw);
  } catch {
    return;
  }
  const id = request.id;
  const action = request.action ?? "";
  const payload = request.payload ?? {};
  if (!id) return;

  if (action === "message.list") {
    respond(socketId, id, {
      messages: [...(state.messagesBySession[String(payload.session_id)] ?? [])].reverse(),
      has_more: false,
    });
    return;
  }
  if (action === "message.search") {
    const query = String(payload.query || "").toLowerCase();
    const messages = state.messagesBySession[String(payload.session_id)] ?? [];
    const hits = messages
      .filter((message) => message.content.toLowerCase().includes(query))
      .map((message) => ({
        id: message.id,
        author_type: message.author_type,
        type: message.type,
        snippet: message.content,
        created_at: message.created_at,
      }));
    respond(socketId, id, { hits, total: hits.length });
    return;
  }
  if (action === "session.ensure" || action === "session.launch") {
    const task = findTask(String(payload.task_id));
    if (!task) {
      respond(socketId, id, { message: "Task not found" }, true);
      return;
    }
    const existing = state.sessions.find((session) => session.task_id === task.id);
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
    return;
  }
  respond(socketId, id, { success: true, demo_mode: true });
}

function startAgent(task: Task, prompt: string) {
  const sessionId = `demo-session-${task.id}`;
  const session = makeSession(sessionId, task.id, "RUNNING");
  state.sessions.push(session);
  task.primary_session_id = session.id;
  task.primary_session_state = "RUNNING";
  task.session_count = 1;
  task.state = "IN_PROGRESS";
  task.workflow_step_id = DEMO_IDS.steps.progress;
  const user = makeMessage(`${sessionId}-user`, sessionId, task.id, "user", prompt);
  state.messagesBySession[sessionId] = [user];
  notify("task.updated", taskEvent(task));
  notify("session.message.added", messageEvent(user));
  setTimeout(() => {
    const agent = makeMessage(
      `${sessionId}-agent`,
      sessionId,
      task.id,
      "agent",
      "I have mapped the relevant code paths. Next I will implement the change, update focused tests, and prepare the diff for review.",
    );
    state.messagesBySession[sessionId].push(agent);
    persist();
    notify("session.message.added", messageEvent(agent));
  }, 900);
  return session;
}

function activeTasks() {
  return state.tasks.filter((task) => !task.archived_at);
}

function findTask(id: string) {
  return state.tasks.find((task) => task.id === id);
}

function parseBody(body?: string): Record<string, unknown> {
  if (!body) return {};
  try {
    return JSON.parse(body) as Record<string, unknown>;
  } catch {
    return {};
  }
}

function taskEvent(task: Task) {
  return {
    task_id: task.id,
    workflow_id: task.workflow_id,
    workflow_step_id: task.workflow_step_id,
    title: task.title,
    description: task.description,
    state: task.state,
    priority: task.priority,
    position: task.position,
    repositories: task.repositories,
    primary_session_id: task.primary_session_id,
    primary_session_state: task.primary_session_state,
    session_count: task.session_count,
    review_status: task.review_status,
    archived_at: task.archived_at,
    updated_at: task.updated_at,
    is_ephemeral: false,
  };
}

function messageEvent(message: Message) {
  return {
    message_id: message.id,
    session_id: message.session_id,
    task_id: message.task_id,
    author_type: message.author_type,
    author_id: message.author_id,
    content: message.content,
    type: message.type,
    created_at: message.created_at,
    updated_at: message.updated_at,
  };
}

function respond(socketId: string, id: string, payload: unknown, error = false) {
  socketMessage(socketId, {
    id,
    type: error ? "error" : "response",
    action: "demo.response",
    payload,
  });
}

function notify(action: string, payload: unknown) {
  socketMessage("*", {
    type: "notification",
    action,
    payload,
    timestamp: new Date().toISOString(),
  });
}

function socketMessage(socketId: string, value: unknown) {
  post({ kind: "ws-event", socketId, event: "message", data: JSON.stringify(value) });
}

function persist() {
  post({ kind: "persist", state: JSON.stringify(state) });
}

function json(body: unknown, status = 200): DemoHttpResponse {
  return { status, headers: { "Content-Type": "application/json" }, body };
}

function empty(): DemoHttpResponse {
  return { status: 204 };
}

function post(message: DemoWorkerResponse) {
  scope.postMessage(message);
}
