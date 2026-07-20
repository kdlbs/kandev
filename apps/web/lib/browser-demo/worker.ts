/// <reference lib="webworker" />

/* eslint-disable complexity, max-lines, max-lines-per-function, sonarjs/cognitive-complexity, sonarjs/no-duplicate-string */

import type { Message, Task, TaskPlan, TaskPlanRevision, TaskSession } from "@/lib/types/http";
import type {
  CumulativeDiff,
  FileInfo,
  SessionCommit,
} from "@/lib/state/slices/session-runtime/types";
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
  demoAgents,
  demoApiRepository,
  demoExecutors,
  demoGitHubPR,
  demoPRFeedback,
  demoRepository,
  demoSteps,
  demoSupportSteps,
  demoSupportWorkflow,
  demoUpgradePlan,
  demoWorkflows,
  makeMessage,
  makeSession,
  type DemoState,
} from "./scenario";
import { createDemoStats } from "./stats";
import { createDemoFiles } from "./demo-files";

const scope: DedicatedWorkerGlobalScope = self as never;
let state = createDemoState();
let files = createDemoFiles();
let plansByTask = createDemoPlans();
const fileReviewsBySession = new Map<
  string,
  Map<string, { reviewed: boolean; diffHash: string }>
>();
const socketUrls = new Map<string, string>();
const terminalInputBySocket = new Map<string, string>();

const DEMO_ENVIRONMENT_ID = "demo-environment";
const DEMO_TERMINAL_ID = "demo-terminal-1";
const DEMO_REPOSITORY_SCRIPTS = [
  {
    id: "demo-script-test",
    repository_id: DEMO_IDS.repository,
    name: "Run tests",
    command: "pnpm test",
    position: 0,
    created_at: "2026-07-18T12:00:00.000Z",
    updated_at: "2026-07-18T12:00:00.000Z",
  },
  {
    id: "demo-script-lint",
    repository_id: DEMO_IDS.repository,
    name: "Lint",
    command: "pnpm lint",
    position: 1,
    created_at: "2026-07-18T12:00:00.000Z",
    updated_at: "2026-07-18T12:00:00.000Z",
  },
];

scope.onmessage = (event: MessageEvent<DemoWorkerRequest>) => {
  const message = event.data;
  if (message.kind === "init") {
    state = restoreState(message.persistedState);
    files = createDemoFiles();
    plansByTask = createDemoPlans();
    fileReviewsBySession.clear();
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
    socketUrls.set(message.socketId, message.url);
    post({ kind: "ws-event", socketId: message.socketId, event: "open" });
    if (isTerminalSocket(message.url)) {
      setTimeout(() => terminalOutput(message.socketId, terminalWelcome(message.url)), 25);
    }
    return;
  }
  if (message.kind === "ws-close") {
    socketUrls.delete(message.socketId);
    terminalInputBySocket.delete(message.socketId);
    post({ kind: "ws-event", socketId: message.socketId, event: "close" });
    return;
  }
  if (message.kind === "ws-send") {
    if (isTerminalSocket(socketUrls.get(message.socketId) ?? "")) {
      handleTerminalInput(message.socketId, message.data);
    } else if (typeof message.data === "string") {
      handleSocketRequest(message.socketId, message.data);
    }
  }
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

export async function handleHttp(request: DemoHttpRequest): Promise<DemoHttpResponse> {
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
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}`) return json(demoWorkspace());
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/repositories`)
    return json({
      repositories: [
        { ...demoRepository, scripts: DEMO_REPOSITORY_SCRIPTS },
        { ...demoApiRepository, scripts: [] },
      ],
      total: 2,
    });
  if (
    path === `/api/v1/repositories/${DEMO_IDS.repository}/branches` ||
    path === `/api/v1/repositories/${DEMO_IDS.apiRepository}/branches`
  )
    return json({ branches: [{ name: "main", type: "local" }], total: 1, current_branch: "main" });
  if (path === `/api/v1/repositories/${DEMO_IDS.repository}/scripts`)
    return json({ scripts: DEMO_REPOSITORY_SCRIPTS, total: DEMO_REPOSITORY_SCRIPTS.length });
  if (path === `/api/v1/repositories/${DEMO_IDS.apiRepository}/scripts`)
    return json({ scripts: [], total: 0 });
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/repositories/discover`)
    return json({ roots: [], repositories: [], total: 0 });
  if (path === "/api/v1/agents") return json({ agents: demoAgents, total: demoAgents.length });
  if (path === "/api/v1/agents/available") return json({ agents: [], tools: [], total: 0 });
  if (path === "/api/v1/executors")
    return json({ executors: demoExecutors, total: demoExecutors.length });
  if (path === "/api/v1/workflows")
    return json({ workflows: demoWorkflows, total: demoWorkflows.length });
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/workflows`)
    return json({ workflows: demoWorkflows, total: demoWorkflows.length });
  if (path === "/api/v1/workflow-templates") return json({ templates: [], total: 0 });
  const workflowSnapshotMatch = path.match(/^\/api\/v1\/workflows\/([^/]+)\/snapshot$/);
  if (workflowSnapshotMatch) {
    const workflow = demoWorkflows.find((item) => item.id === workflowSnapshotMatch[1]);
    if (!workflow) return json({ error: "Workflow not found" }, 404);
    const steps = workflow.id === demoSupportWorkflow.id ? demoSupportSteps : demoSteps;
    return json({
      workflow,
      steps,
      tasks: activeTasks().filter((task) => task.workflow_id === workflow.id),
    });
  }
  const workflowStepsMatch = path.match(/^\/api\/v1\/workflows\/([^/]+)\/workflow\/steps$/);
  if (workflowStepsMatch) {
    const steps = workflowStepsMatch[1] === demoSupportWorkflow.id ? demoSupportSteps : demoSteps;
    return json({ steps, total: steps.length });
  }
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/workflow-steps`)
    return json({
      steps: [...demoSteps, ...demoSupportSteps],
      total: demoSteps.length + demoSupportSteps.length,
    });
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/tasks`)
    return json({ tasks: activeTasks(), total: activeTasks().length });
  const statsMatch = path.match(
    new RegExp(`^/api/v1/workspaces/${DEMO_IDS.workspace}/stats/([^/]+)$`),
  );
  if (statsMatch) {
    const stats = createDemoStats(statsMatch[1], state);
    if (stats !== undefined) return json(stats);
  }
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
  if (path === "/api/v1/github/prs/kandev-demo/acme-web/142") return json(demoPRFeedback);
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
    const sessions = state.sessions
      .filter((session) => session.task_id === sessionsMatch[1])
      .map(withDemoEnvironment);
    return json({ sessions, total: sessions.length });
  }
  const terminalsMatch = path.match(/^\/api\/v1\/tasks\/([^/]+)\/terminals$/);
  if (terminalsMatch) return json({ terminals: [demoTerminal()], total: 1 });
  const sessionMatch = path.match(/^\/api\/v1\/task-sessions\/([^/]+)$/);
  if (sessionMatch) {
    const session = state.sessions.find((item) => item.id === sessionMatch[1]);
    return session
      ? json({ session: withDemoEnvironment(session) })
      : json({ error: "Session not found" }, 404);
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

export function handleSocketRequest(socketId: string, raw: string) {
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
    return;
  }
  if (action === "session.focus") {
    const sessionId = String(payload.session_id || "");
    respond(socketId, id, { success: true });
    notifyGitStatus(sessionId);
    return;
  }
  if (action === "session.unfocus") {
    respond(socketId, id, { success: true });
    return;
  }
  if (action.startsWith("task.plan.")) {
    handlePlanRequest(socketId, id, action, payload);
    return;
  }
  if (action === "session.git.commits") {
    const sessionId = String(payload.session_id || "");
    respond(socketId, id, { commits: demoGitData(sessionId).commits, ready: true });
    return;
  }
  if (action === "session.cumulative_diff") {
    const sessionId = String(payload.session_id || "");
    respond(socketId, id, { cumulative_diff: demoGitData(sessionId).cumulativeDiff, ready: true });
    return;
  }
  if (action === "session.file_review.get") {
    const sessionId = String(payload.session_id || "");
    respond(socketId, id, { reviews: serializeFileReviews(sessionId) });
    return;
  }
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
    return;
  }
  if (action === "session.file_review.reset") {
    fileReviewsBySession.delete(String(payload.session_id || ""));
    respond(socketId, id, { success: true });
    return;
  }
  if (action === "workspace.tree.get") {
    respond(socketId, id, { root: buildFileTree(String(payload.path || "")) });
    return;
  }
  if (action === "workspace.file.get" || action === "workspace.file.get_at_ref") {
    const path = normalizeFilePath(payload.path);
    const content = files[path];
    if (content === undefined) {
      respond(socketId, id, { message: `File not found: ${path}` }, true);
      return;
    }
    respond(socketId, id, { path, content, size: content.length, is_binary: false });
    return;
  }
  if (action === "workspace.files.search") {
    const query = String(payload.query || "").toLowerCase();
    const limit = Number(payload.limit ?? 20);
    const matches = Object.keys(files)
      .filter((path) => path.toLowerCase().includes(query))
      .slice(0, limit);
    respond(socketId, id, { files: matches });
    return;
  }
  if (action === "workspace.file.update") {
    const path = normalizeFilePath(payload.path);
    const desiredContent = payload.desired_content;
    if (typeof desiredContent !== "string") {
      respond(socketId, id, { message: "The demo editor requires desired_content" }, true);
      return;
    }
    files[path] = desiredContent;
    respond(socketId, id, {
      path,
      success: true,
      new_hash: simpleHash(desiredContent),
      resolution: "applied",
    });
    notifyFileChange(String(payload.session_id || ""), path, "write");
    return;
  }
  if (action === "workspace.file.create") {
    const path = normalizeFilePath(payload.path);
    files[path] = "";
    respond(socketId, id, { path, success: true });
    notifyFileChange(String(payload.session_id || ""), path, "create");
    return;
  }
  if (action === "workspace.file.delete") {
    const path = normalizeFilePath(payload.path);
    for (const filePath of Object.keys(files)) {
      if (filePath === path || filePath.startsWith(`${path}/`)) delete files[filePath];
    }
    respond(socketId, id, { path, success: true });
    notifyFileChange(String(payload.session_id || ""), path, "remove");
    return;
  }
  if (action === "workspace.file.rename") {
    const oldPath = normalizeFilePath(payload.old_path);
    const newPath = normalizeFilePath(payload.new_path);
    for (const filePath of Object.keys(files)) {
      if (filePath !== oldPath && !filePath.startsWith(`${oldPath}/`)) continue;
      const renamedPath = `${newPath}${filePath.slice(oldPath.length)}`;
      files[renamedPath] = files[filePath];
      delete files[filePath];
    }
    respond(socketId, id, { old_path: oldPath, new_path: newPath, success: true });
    notifyFileChange(String(payload.session_id || ""), newPath, "rename");
    return;
  }
  if (action === "user_shell.list") {
    respond(socketId, id, { shells: [demoTerminal()] });
    return;
  }
  if (action === "user_shell.create") {
    respond(socketId, id, {
      terminal_id: DEMO_TERMINAL_ID,
      kind: "ordinary",
      seq: 1,
      display_name: "Terminal 1",
      state: "open",
      pty_status: "running",
      closable: true,
    });
    return;
  }
  if (
    action === "user_shell.destroy" ||
    action === "user_shell.stop" ||
    action === "user_shell.rename" ||
    action === "user_shell.park" ||
    action === "user_shell.resume"
  ) {
    respond(socketId, id, { success: true });
    return;
  }

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

function createDemoPlans(): Record<string, TaskPlan> {
  return { [demoUpgradePlan.task_id]: { ...demoUpgradePlan } };
}

function handlePlanRequest(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
) {
  const taskId = String(payload.task_id || "");
  const plan = plansByTask[taskId];
  if (action === "task.plan.get") {
    respond(socketId, id, plan ?? null);
    return;
  }
  if (action === "task.plan.revisions.list") {
    respond(socketId, id, { revisions: plan ? [demoPlanRevision(plan, false)] : [] });
    return;
  }
  if (action === "task.plan.revision.get") {
    const revision = Object.values(plansByTask)
      .map((candidate) => demoPlanRevision(candidate, true))
      .find((candidate) => candidate.id === payload.revision_id);
    respond(socketId, id, revision ?? null);
    return;
  }
  if (action === "task.plan.delete") {
    delete plansByTask[taskId];
    respond(socketId, id, { success: true });
    notify("task.plan.deleted", { task_id: taskId });
    return;
  }
  if (action === "task.plan.revert") {
    const revision = plan ? demoPlanRevision(plan, true) : null;
    respond(socketId, id, revision);
    return;
  }
  if (action === "task.plan.implementation_started") {
    if (!plan) {
      respond(socketId, id, { message: "Task plan not found" }, true);
      return;
    }
    const updated = {
      ...plan,
      implementation_started_at: new Date().toISOString(),
      implementation_started_session_id: String(payload.session_id || ""),
      implementation_started_by: String(payload.actor || "user"),
    };
    plansByTask[taskId] = updated;
    respond(socketId, id, updated);
    notify("task.plan.updated", updated);
    return;
  }
  if (action === "task.plan.create" || action === "task.plan.update") {
    const now = new Date().toISOString();
    const updated: TaskPlan = {
      id: plan?.id ?? `demo-plan-${taskId}`,
      task_id: taskId,
      title: String(payload.title || plan?.title || "Plan"),
      content: String(payload.content ?? plan?.content ?? ""),
      created_by: payload.created_by === "agent" ? "agent" : "user",
      created_at: plan?.created_at ?? now,
      updated_at: now,
    };
    plansByTask[taskId] = updated;
    respond(socketId, id, updated);
    notify(action === "task.plan.create" ? "task.plan.created" : "task.plan.updated", updated);
    return;
  }
  respond(socketId, id, { message: `Unsupported plan action: ${action}` }, true);
}

function demoPlanRevision(plan: TaskPlan, includeContent: boolean): TaskPlanRevision {
  return {
    id: `${plan.id}-revision-1`,
    task_id: plan.task_id,
    revision_number: 1,
    title: plan.title,
    ...(includeContent ? { content: plan.content } : {}),
    author_kind: plan.created_by,
    author_name: plan.created_by === "agent" ? "Mock agent" : "Demo user",
    created_at: plan.created_at,
    updated_at: plan.updated_at,
  };
}

type DemoGitData = {
  status: {
    branch: string;
    remote_branch: string;
    modified: string[];
    added: string[];
    deleted: string[];
    untracked: string[];
    renamed: string[];
    ahead: number;
    behind: number;
    files: Record<string, FileInfo>;
    branch_additions: number;
    branch_deletions: number;
  };
  commits: SessionCommit[];
  cumulativeDiff: CumulativeDiff | null;
};

function demoGitData(sessionId: string): DemoGitData {
  const task = state.tasks.find((candidate) => candidate.primary_session_id === sessionId);
  if (task?.id === "demo-task-checkout") return checkoutGitData(sessionId);
  if (task?.id === "demo-task-audit") return auditGitData(sessionId);
  return emptyGitData(sessionId, task?.id ?? "task");
}

function checkoutGitData(sessionId: string): DemoGitData {
  const files: Record<string, FileInfo> = {
    "src/checkout/complete-order.ts": {
      path: "src/checkout/complete-order.ts",
      status: "modified",
      staged: false,
      additions: 8,
      deletions: 4,
      diff: "@@ -42,7 +42,11 @@ export async function completeOrder(order: Order) {\n-  await withOrderLock(order.id, completeOrder);\n+  await completePayment(order);\n+  await withOrderLock(order.id, async () => {\n+    await reserveInventory(order);\n+  });",
    },
    "tests/checkout/concurrent-inventory.test.ts": {
      path: "tests/checkout/concurrent-inventory.test.ts",
      status: "added",
      staged: true,
      additions: 34,
      deletions: 0,
      diff: '@@ -0,0 +1,5 @@\n+describe("concurrent checkout", () => {\n+  it("does not hold the order lock during payment", async () => {\n+    await expect(runConcurrentCheckout()).resolves.toEqual(["paid", "reserved"]);\n+  });\n+});',
    },
  };
  return {
    status: makeGitStatus("kandev/fix-checkout-timeout", files, 1, 0),
    commits: [
      makeCommit(sessionId, {
        sha: "8f73b42",
        message: "test: cover concurrent checkout",
        filesChanged: 1,
        insertions: 34,
        deletions: 0,
      }),
    ],
    cumulativeDiff: makeCumulativeDiff(sessionId, files, 1),
  };
}

function auditGitData(sessionId: string): DemoGitData {
  const files: Record<string, FileInfo> = {
    "src/audit/record-event.ts": {
      path: "src/audit/record-event.ts",
      status: "modified",
      staged: false,
      additions: 12,
      deletions: 5,
      diff: "@@ -44,7 +44,9 @@ export async function recordEvent(input: AuditInput) {\n-    actorIp: input.ip,\n+    actorRegion: await privacyFilter.regionFor(input.ip),\n+    retentionDays: 90,",
    },
    "src/pages/admin/activity-page.tsx": {
      path: "src/pages/admin/activity-page.tsx",
      status: "added",
      staged: true,
      additions: 68,
      deletions: 0,
      diff: "@@ -0,0 +1,6 @@\n+export function ActivityFeed({ events }: Props) {\n+  return events.map((event) => (\n+    <AuditEventRow key={event.id} event={event} />\n+  ));\n+}",
    },
  };
  return {
    status: makeGitStatus("kandev/audit-logging", files, 2, 0),
    commits: [
      makeCommit(sessionId, {
        sha: "c24e18a",
        message: "feat: persist privileged audit events",
        filesChanged: 4,
        insertions: 116,
        deletions: 19,
      }),
      makeCommit(sessionId, {
        sha: "41ac09d",
        message: "feat: add admin activity feed",
        filesChanged: 3,
        insertions: 68,
        deletions: 8,
      }),
    ],
    cumulativeDiff: makeCumulativeDiff(sessionId, files, 2),
  };
}

function emptyGitData(sessionId: string, taskId: string): DemoGitData {
  return {
    status: makeGitStatus(`kandev/${taskId}`, {}, 0, 0),
    commits: [],
    cumulativeDiff: null,
  };
}

function makeGitStatus(
  branch: string,
  files: Record<string, FileInfo>,
  ahead: number,
  behind: number,
) {
  const values = Object.values(files);
  return {
    branch,
    remote_branch: `origin/${branch}`,
    modified: values.filter((file) => file.status === "modified").map((file) => file.path),
    added: values.filter((file) => file.status === "added").map((file) => file.path),
    deleted: values.filter((file) => file.status === "deleted").map((file) => file.path),
    untracked: values.filter((file) => file.status === "untracked").map((file) => file.path),
    renamed: values.filter((file) => file.status === "renamed").map((file) => file.path),
    ahead,
    behind,
    files,
    branch_additions: values.reduce((total, file) => total + (file.additions ?? 0), 0),
    branch_deletions: values.reduce((total, file) => total + (file.deletions ?? 0), 0),
  };
}

function makeCommit(
  sessionId: string,
  input: {
    sha: string;
    message: string;
    filesChanged: number;
    insertions: number;
    deletions: number;
  },
): SessionCommit {
  return {
    id: `demo-commit-${input.sha}`,
    session_id: sessionId,
    commit_sha: input.sha,
    parent_sha: "6c12a90",
    author_name: "Kandev Demo",
    author_email: "demo@kandev.com",
    commit_message: input.message,
    committed_at: "2026-07-18T11:45:00.000Z",
    files_changed: input.filesChanged,
    insertions: input.insertions,
    deletions: input.deletions,
    created_at: "2026-07-18T11:45:00.000Z",
    pushed: false,
  };
}

function makeCumulativeDiff(
  sessionId: string,
  files: Record<string, FileInfo>,
  totalCommits: number,
): CumulativeDiff {
  return {
    session_id: sessionId,
    base_commit: "6c12a90",
    head_commit: "8f73b42",
    total_commits: totalCommits,
    files,
  };
}

function notifyGitStatus(sessionId: string) {
  const session = state.sessions.find((candidate) => candidate.id === sessionId);
  if (!session) return;
  notify("session.git.event", {
    type: "status_update",
    session_id: sessionId,
    task_id: session.task_id,
    agent_id: DEMO_IDS.agent,
    timestamp: new Date().toISOString(),
    status: demoGitData(sessionId).status,
  });
}

function serializeFileReviews(sessionId: string) {
  const now = "2026-07-18T12:00:00.000Z";
  return Array.from(fileReviewsBySession.get(sessionId) ?? [], ([filePath, review], index) => ({
    id: `demo-review-${index + 1}`,
    session_id: sessionId,
    file_path: filePath,
    reviewed: review.reviewed,
    diff_hash: review.diffHash,
    reviewed_at: review.reviewed ? now : null,
    created_at: now,
    updated_at: now,
  }));
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

function demoWorkspace() {
  return createBootPayload(state).initialState?.workspaces?.items[0];
}

function withDemoEnvironment(session: TaskSession): TaskSession {
  return { ...session, task_environment_id: session.task_environment_id ?? DEMO_ENVIRONMENT_ID };
}

function demoTerminal() {
  return {
    id: DEMO_TERMINAL_ID,
    terminal_id: DEMO_TERMINAL_ID,
    kind: "ordinary",
    seq: 1,
    display_name: "Terminal 1",
    custom_name: null,
    state: "open",
    pty_status: "running",
    label: "Terminal 1",
    closable: true,
  };
}

function buildFileTree(path: string) {
  const normalizedPath = normalizeFilePath(path);
  const prefix = normalizedPath ? `${normalizedPath}/` : "";
  const childNames = new Set<string>();
  for (const filePath of Object.keys(files)) {
    if (!filePath.startsWith(prefix)) continue;
    const relative = filePath.slice(prefix.length);
    if (relative) childNames.add(relative.split("/")[0]);
  }
  const children = Array.from(childNames, (name) => {
    const childPath = prefix + name;
    const isDirectory = Object.keys(files).some((filePath) => filePath.startsWith(`${childPath}/`));
    return {
      name,
      path: childPath,
      is_dir: isDirectory,
      size: isDirectory ? undefined : (files[childPath]?.length ?? 0),
    };
  });
  return {
    name: normalizedPath.split("/").pop() || demoRepository.name,
    path: normalizedPath,
    is_dir: true,
    children,
  };
}

function normalizeFilePath(value: unknown) {
  return String(value || "")
    .replaceAll("\\", "/")
    .replace(/^\.\//, "")
    .replace(/^\/+|\/+$/g, "");
}

function simpleHash(content: string) {
  let hash = 0;
  for (let index = 0; index < content.length; index += 1) {
    hash = (hash * 31 + content.charCodeAt(index)) | 0;
  }
  return `demo-${Math.abs(hash).toString(16)}`;
}

function notifyFileChange(sessionId: string, path: string, operation: string) {
  notify("session.workspace.file.changes", {
    session_id: sessionId,
    changes: [
      {
        session_id: sessionId,
        task_id: state.sessions.find((session) => session.id === sessionId)?.task_id ?? "",
        agent_id: DEMO_IDS.agent,
        timestamp: new Date().toISOString(),
        path,
        operation,
      },
    ],
  });
}

function isTerminalSocket(url: string) {
  try {
    return new URL(url, "https://demo.kandev.com").pathname.includes("/terminal/");
  } catch {
    return false;
  }
}

function terminalWelcome(url: string) {
  const isAgent = url.includes("/terminal/session/");
  const heading = isAgent ? "Mock agent terminal" : "Acme Platform workspace";
  return (
    `\u001b[2J\u001b[H\u001b[1;36m${heading}\u001b[0m\r\n` +
    "Browser demo shell. Commands run against simulated workspace data.\r\n\r\n" +
    "demo@acme-web ~/acme-web $ "
  );
}

function handleTerminalInput(socketId: string, data: string | ArrayBuffer) {
  const bytes = typeof data === "string" ? null : new Uint8Array(data);
  if (bytes?.[0] === 0x01) return;
  const input = typeof data === "string" ? data : new TextDecoder().decode(new Uint8Array(data));
  const previous = terminalInputBySocket.get(socketId) ?? "";
  let current = previous;
  for (const character of input) {
    if (character === "\r" || character === "\n") {
      terminalOutput(socketId, `\r\n${terminalCommandResult(current)}demo@acme-web ~/acme-web $ `);
      current = "";
    } else if (character === "\u007f") {
      current = current.slice(0, -1);
    } else {
      current += character;
      terminalOutput(socketId, character);
    }
  }
  terminalInputBySocket.set(socketId, current);
}

function terminalCommandResult(command: string) {
  const normalized = command.trim();
  if (!normalized) return "";
  if (normalized === "pwd") return "/demo/acme-web\r\n";
  if (normalized === "ls") return "README.md  package.json  src  tests\r\n";
  if (normalized === "git status")
    return "On branch kandev/audit-logging\r\nChanges not staged for commit:\r\n  modified: src/api/audit.ts\r\n";
  if (normalized === "pnpm test")
    return "✓ tests/audit-log.test.tsx (1 test)\r\nTest Files  1 passed (1)\r\n";
  if (normalized.startsWith("cat ")) {
    const content = files[normalizeFilePath(normalized.slice(4))];
    return content === undefined
      ? `cat: file not found\r\n`
      : `${content.replaceAll("\n", "\r\n")}\r\n`;
  }
  return `command not available in browser demo: ${normalized}\r\n`;
}

function terminalOutput(socketId: string, data: string) {
  post({ kind: "ws-event", socketId, event: "message", data });
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
  const message = {
    type: "notification",
    action,
    payload,
    timestamp: new Date().toISOString(),
  };
  for (const [socketId, url] of socketUrls) {
    if (!isTerminalSocket(url)) socketMessage(socketId, message);
  }
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
