/// <reference lib="webworker" />

/* eslint-disable max-lines -- The worker intentionally keeps the complete in-browser backend fixture in one module. */

import type { Message, Task, TaskPlan, TaskPlanRevision, TaskSession } from "@/lib/types/http";
import type { UtilityAgent } from "@/lib/api/domains/utility-api";
import type { JiraConfig } from "@/lib/types/jira";
import type { LinearConfig, LinearTeam } from "@/lib/types/linear";
import type { SentryConfig } from "@/lib/types/sentry";
import type { SlackConfig } from "@/lib/types/slack";
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
  demoAccessibleRepositories,
  demoAgents,
  demoApiRepository,
  demoExecutors,
  demoGitHubPR,
  demoPRFeedback,
  demoRepository,
  demoRepositoryBranches,
  demoSteps,
  demoUpgradePlan,
  makeMessage,
  makeSession,
  type DemoState,
} from "./scenario";
import { createDemoStats } from "./stats";
import { createDemoFiles } from "./demo-files";
import { createDemoMultiRepoFiles } from "./demo-multi-repo-files";
import { createDemoSystemRuntime } from "./system-runtime";
import { createDemoWorkflowRuntime } from "./workflow-runtime";

const scope: DedicatedWorkerGlobalScope = self as never;
let state = createDemoState();
let files = createDemoFiles();
let multiRepoFiles = createDemoMultiRepoFiles();
let plansByTask = createDemoPlans();
let systemRuntime = createDemoSystemRuntime();
let workflowRuntime = makeWorkflowRuntime();
const fileReviewsBySession = new Map<
  string,
  Map<string, { reviewed: boolean; diffHash: string }>
>();
const socketUrls = new Map<string, string>();
const terminalInputBySocket = new Map<string, string>();

const DEMO_ENVIRONMENT_ID = "demo-environment";
const DEMO_TERMINAL_ID = "demo-terminal-1";
const DEMO_TIMESTAMP = "2026-07-18T12:00:00.000Z";
const TASK_NOT_FOUND = "Task not found";
const PLAN_NOT_FOUND = "Task plan not found";
const TASK_UPDATED_EVENT = "task.updated";
const DEMO_SLACK_UTILITY_AGENT_ID = "demo-utility-triage";
const DEMO_JIRA_CONFIG: JiraConfig = {
  workspaceId: DEMO_IDS.workspace,
  siteUrl: "https://acme-platform.atlassian.net",
  email: "mira@acme.example",
  authMethod: "api_token",
  instanceType: "cloud",
  defaultProjectKey: "PLAT",
  hasSecret: true,
  secretExpiresAt: null,
  lastCheckedAt: DEMO_TIMESTAMP,
  lastOk: true,
  lastError: "",
  createdAt: DEMO_TIMESTAMP,
  updatedAt: DEMO_TIMESTAMP,
};
const DEMO_LINEAR_CONFIG: LinearConfig = {
  workspaceId: DEMO_IDS.workspace,
  authMethod: "api_key",
  defaultTeamKey: "ENG",
  hasSecret: true,
  orgSlug: "acme-platform",
  lastCheckedAt: DEMO_TIMESTAMP,
  lastOk: true,
  lastError: "",
  createdAt: DEMO_TIMESTAMP,
  updatedAt: DEMO_TIMESTAMP,
};
const DEMO_LINEAR_TEAMS: LinearTeam[] = [
  { id: "demo-linear-team-eng", key: "ENG", name: "Engineering" },
  { id: "demo-linear-team-plat", key: "PLAT", name: "Platform" },
];
const DEMO_SENTRY_INSTANCES: SentryConfig[] = [
  {
    id: "demo-sentry-production",
    workspaceId: DEMO_IDS.workspace,
    name: "Production",
    authMethod: "auth_token",
    url: "https://sentry.io",
    hasSecret: true,
    lastCheckedAt: DEMO_TIMESTAMP,
    lastOk: true,
    lastError: "",
    createdAt: DEMO_TIMESTAMP,
    updatedAt: DEMO_TIMESTAMP,
  },
];
const DEMO_SLACK_CONFIG: SlackConfig = {
  workspaceId: DEMO_IDS.workspace,
  authMethod: "cookie",
  commandPrefix: "!kandev",
  utilityAgentId: DEMO_SLACK_UTILITY_AGENT_ID,
  pollIntervalSeconds: 30,
  slackTeamId: "T0ACME",
  slackUserId: "U0KANDEV",
  lastSeenTs: "1721304000.000000",
  hasToken: true,
  hasCookie: true,
  lastCheckedAt: DEMO_TIMESTAMP,
  lastOk: true,
  lastError: "",
  createdAt: DEMO_TIMESTAMP,
  updatedAt: DEMO_TIMESTAMP,
};
const DEMO_UTILITY_AGENTS: UtilityAgent[] = [
  {
    id: DEMO_SLACK_UTILITY_AGENT_ID,
    name: "Slack task triage",
    description: "Turns Slack requests into scoped Kandev tasks.",
    prompt: "Route this Slack request to the correct workspace and workflow.",
    agent_id: DEMO_IDS.agent,
    model: "gpt-5",
    builtin: false,
    enabled: true,
    created_at: DEMO_TIMESTAMP,
    updated_at: DEMO_TIMESTAMP,
  },
];
const DEMO_REPOSITORY_SCRIPTS = [
  {
    id: "demo-script-test",
    repository_id: DEMO_IDS.repository,
    name: "Run tests",
    command: "pnpm test",
    position: 0,
    created_at: DEMO_TIMESTAMP,
    updated_at: DEMO_TIMESTAMP,
  },
  {
    id: "demo-script-lint",
    repository_id: DEMO_IDS.repository,
    name: "Lint",
    command: "pnpm lint",
    position: 1,
    created_at: DEMO_TIMESTAMP,
    updated_at: DEMO_TIMESTAMP,
  },
];

scope.onmessage = (event: MessageEvent<DemoWorkerRequest>) => {
  const message = event.data;
  if (message.kind === "init") {
    state = restoreState(message.persistedState);
    files = createDemoFiles();
    multiRepoFiles = createDemoMultiRepoFiles();
    plansByTask = createDemoPlans();
    systemRuntime = createDemoSystemRuntime();
    workflowRuntime = makeWorkflowRuntime();
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

type HttpRouteContext = {
  path: string;
  method: string;
  input: Record<string, unknown>;
  rawBody?: string;
  searchParams: URLSearchParams;
};

type HttpRouter = (context: HttpRouteContext) => DemoHttpResponse | null;

const HTTP_ROUTERS: HttpRouter[] = [
  routeCoreHttp,
  (context) => systemRuntime.route(context),
  routeRepositoryHttp,
  (context) => workflowRuntime.route(context),
  routeStatsHttp,
  routeGitHubHttp,
  routeIntegrationsHttp,
  routeTaskHttp,
];

export async function handleHttp(request: DemoHttpRequest): Promise<DemoHttpResponse> {
  const url = new URL(request.path, "https://demo.kandev.com");
  const context: HttpRouteContext = {
    path: url.pathname,
    method: request.method.toUpperCase(),
    input: parseBody(request.body),
    rawBody: request.body,
    searchParams: url.searchParams,
  };
  for (const router of HTTP_ROUTERS) {
    const response = router(context);
    if (response) return response;
  }
  if (context.method === "GET") {
    return json({ demo_mode: true, unsupported: context.path }, 501);
  }
  return json({ error: "This action is disabled in the browser demo", demo_mode: true }, 501);
}

function routeCoreHttp({ path }: HttpRouteContext): DemoHttpResponse | null {
  if (path === "/health") return json({ status: "ok", mode: "browser-demo" });
  if (path === "/api/v1/features") return json({ office: false, plugins: false });
  if (path === "/api/v1/app-state") return json(createBootPayload(state));
  if (path === "/api/v1/agents") return json({ agents: demoAgents, total: demoAgents.length });
  if (path === "/api/v1/agents/available") return json({ agents: [], tools: [], total: 0 });
  if (path === "/api/v1/executors") {
    return json({ executors: demoExecutors, total: demoExecutors.length });
  }
  return null;
}

function routeRepositoryHttp({ path }: HttpRouteContext): DemoHttpResponse | null {
  if (path === "/api/v1/workspaces") {
    const workspaces = createBootPayload(state).initialState?.workspaces?.items ?? [];
    return json({ workspaces, total: workspaces.length });
  }
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}`) return json(demoWorkspace());
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/repositories`) {
    return json({
      repositories: [
        { ...demoRepository, scripts: DEMO_REPOSITORY_SCRIPTS },
        { ...demoApiRepository, scripts: [] },
      ],
      total: 2,
    });
  }
  const branchPaths = [
    `/api/v1/repositories/${DEMO_IDS.repository}/branches`,
    `/api/v1/repositories/${DEMO_IDS.apiRepository}/branches`,
  ];
  if (branchPaths.includes(path)) {
    return json({ branches: [{ name: "main", type: "local" }], total: 1, current_branch: "main" });
  }
  if (path === `/api/v1/repositories/${DEMO_IDS.repository}/scripts`) {
    return json({ scripts: DEMO_REPOSITORY_SCRIPTS, total: DEMO_REPOSITORY_SCRIPTS.length });
  }
  if (path === `/api/v1/repositories/${DEMO_IDS.apiRepository}/scripts`) {
    return json({ scripts: [], total: 0 });
  }
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/repositories/discover`) {
    return json({ roots: [], repositories: [], total: 0 });
  }
  return null;
}

function routeStatsHttp({ path }: HttpRouteContext): DemoHttpResponse | null {
  const match = path.match(new RegExp(`^/api/v1/workspaces/${DEMO_IDS.workspace}/stats/([^/]+)$`));
  if (!match) return null;
  const stats = createDemoStats(match[1], state);
  return stats === undefined ? null : json(stats);
}

function routeGitHubHttp({ path }: HttpRouteContext): DemoHttpResponse | null {
  if (path === "/api/v1/github/status") {
    return json({
      configured: true,
      authenticated: true,
      auth_method: "gh_cli",
      username: "kandev-demo",
    });
  }
  if (path === "/api/v1/github/user/prs") {
    return json({ prs: [demoGitHubPR], total_count: 1, page: 1, per_page: 25 });
  }
  if (path === "/api/v1/github/user/issues") {
    return json({ issues: [], total_count: 0, page: 1, per_page: 25 });
  }
  if (path === "/api/v1/github/repos") {
    return json({ repos: demoAccessibleRepositories });
  }
  const branchesMatch = path.match(/^\/api\/v1\/github\/repos\/([^/]+)\/([^/]+)\/branches$/);
  if (branchesMatch) {
    const owner = decodeURIComponent(branchesMatch[1]);
    const repo = decodeURIComponent(branchesMatch[2]);
    const branches = demoRepositoryBranches[`${owner}/${repo}`];
    return branches ? json({ branches }) : json({ error: "Repository not found" }, 404);
  }
  if (path === "/api/v1/github/workspace-settings") return githubWorkspaceSettings();
  if (path === "/api/v1/github/action-presets") {
    return json({ workspace_id: DEMO_IDS.workspace, pr: [], issue: [] });
  }
  if (path === "/api/v1/github/task-prs") return json({ task_prs: state.taskPRs });
  if (path === "/api/v1/github/prs/kandev-demo/acme-web/142") return json(demoPRFeedback);
  if (
    [
      "/api/v1/github/watches/pr",
      "/api/v1/github/watches/review",
      "/api/v1/github/watches/issues",
    ].includes(path)
  ) {
    return json({ watches: [] });
  }
  return null;
}

function githubWorkspaceSettings(): DemoHttpResponse {
  return json({
    workspace_id: DEMO_IDS.workspace,
    repo_scope_mode: "repos",
    repo_scope_orgs: [],
    repo_scope_repos: [{ owner: "kandev-demo", name: "acme-web" }],
    saved_presets: [],
    default_query_presets: null,
    created_at: DEMO_TIMESTAMP,
    updated_at: DEMO_TIMESTAMP,
  });
}

function routeIntegrationsHttp({ path, method }: HttpRouteContext): DemoHttpResponse | null {
  if (method !== "GET") return null;
  if (path === "/api/v1/jira/config") return json(DEMO_JIRA_CONFIG);
  if (path === "/api/v1/jira/watches/issue") return json({ watches: [] });
  if (path === "/api/v1/linear/config") return json(DEMO_LINEAR_CONFIG);
  if (path === "/api/v1/linear/teams") return json({ teams: DEMO_LINEAR_TEAMS });
  if (path === "/api/v1/linear/watches/issue") return json({ watches: [] });
  if (path === "/api/v1/sentry/instances") return json({ instances: DEMO_SENTRY_INSTANCES });
  if (path === "/api/v1/sentry/watches/issue") return json({ watches: [] });
  if (path === "/api/v1/slack/config") return json(DEMO_SLACK_CONFIG);
  if (path === "/api/v1/utility/agents") return json({ agents: DEMO_UTILITY_AGENTS });
  return null;
}

function routeTaskHttp(context: HttpRouteContext): DemoHttpResponse | null {
  const { path, method, input } = context;
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/tasks` && method === "GET") {
    const tasks = activeTasks();
    return json({ tasks, total: tasks.length });
  }
  if (path === "/api/v1/tasks" && method === "POST") return createTask(input);
  const taskMatch = path.match(/^\/api\/v1\/tasks\/([^/]+)$/);
  if (taskMatch) return taskResponse(taskMatch[1], method, input);
  const moveMatch = path.match(/^\/api\/v1\/tasks\/([^/]+)\/move$/);
  if (moveMatch && method === "POST") return moveTask(moveMatch[1], input);
  const archiveMatch = path.match(/^\/api\/v1\/tasks\/([^/]+)\/archive$/);
  if (archiveMatch && method === "POST") return archiveTask(archiveMatch[1]);
  return routeTaskSessionHttp(path);
}

function taskResponse(id: string, method: string, input: Record<string, unknown>) {
  const task = findTask(id);
  if (!task) return json({ error: TASK_NOT_FOUND }, 404);
  if (method === "GET") return json(task);
  if (method === "PATCH") return updateTask(task, input);
  if (method === "DELETE") return removeTask(task);
  return null;
}

function moveTask(id: string, input: Record<string, unknown>): DemoHttpResponse {
  const task = findTask(id);
  if (!task) return json({ error: TASK_NOT_FOUND }, 404);
  task.workflow_step_id = String(input.workflow_step_id || task.workflow_step_id);
  task.position = Number(input.position ?? task.position);
  task.updated_at = new Date().toISOString();
  persist();
  notify(TASK_UPDATED_EVENT, taskEvent(task));
  const workflowStep = demoSteps.find((step) => step.id === task.workflow_step_id);
  return json({ task, workflow_step: workflowStep });
}

function archiveTask(id: string): DemoHttpResponse {
  const task = findTask(id);
  if (!task) return json({ error: TASK_NOT_FOUND }, 404);
  task.archived_at = new Date().toISOString();
  persist();
  notify("task.deleted", taskEvent(task));
  return empty();
}

function routeTaskSessionHttp(path: string): DemoHttpResponse | null {
  const sessionsMatch = path.match(/^\/api\/v1\/tasks\/([^/]+)\/sessions$/);
  if (sessionsMatch) {
    const sessions = state.sessions
      .filter((session) => session.task_id === sessionsMatch[1])
      .map(withDemoEnvironment);
    return json({ sessions, total: sessions.length });
  }
  if (/^\/api\/v1\/tasks\/[^/]+\/terminals$/.test(path)) {
    return json({ terminals: [demoTerminal()], total: 1 });
  }
  const sessionMatch = path.match(/^\/api\/v1\/task-sessions\/([^/]+)$/);
  if (sessionMatch) {
    const session = state.sessions.find((item) => item.id === sessionMatch[1]);
    return session
      ? json({ session: withDemoEnvironment(session) })
      : json({ error: "Session not found" }, 404);
  }
  const messagesMatch = path.match(/^\/api\/v1\/task-sessions\/([^/]+)\/messages$/);
  if (messagesMatch) {
    return json({ messages: state.messagesBySession[messagesMatch[1]] ?? [], has_more: false });
  }
  if (/^\/api\/v1\/task-sessions\/[^/]+\/turns$/.test(path)) {
    return json({ turns: [], total: 0 });
  }
  return null;
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
  notify(TASK_UPDATED_EVENT, taskEvent(task));
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
  const context = { socketId, id, action, payload };
  if (SOCKET_ROUTERS.some((router) => router(context))) return;
  respond(socketId, id, { success: true, demo_mode: true });
}

type SocketRouteContext = {
  socketId: string;
  id: string;
  action: string;
  payload: Record<string, unknown>;
};

type SocketRouter = (context: SocketRouteContext) => boolean;

const SOCKET_ROUTERS: SocketRouter[] = [
  routeSessionSocket,
  routePermissionSocket,
  routePlanReviewSocket,
  routeWorkspaceReadSocket,
  routeWorkspaceMutationSocket,
  routeShellSocket,
  routeMessageSocket,
];

function routePermissionSocket(context: SocketRouteContext): boolean {
  const { socketId, id, action, payload } = context;
  if (action !== "permission.respond") return false;

  const pendingId = String(payload.pending_id || "");
  const sessionId = String(payload.session_id || "");
  const permission = (state.messagesBySession[sessionId] ?? []).find(
    (message) =>
      message.type === "permission_request" &&
      (message.metadata as { pending_id?: string } | undefined)?.pending_id === pendingId,
  );
  if (!permission) {
    respond(socketId, id, { message: "Permission request not found" }, true);
    return true;
  }

  const status = permissionResponseStatus(payload);
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

function permissionResponseStatus(payload: Record<string, unknown>) {
  if (payload.cancelled) return "expired";
  return payload.rejected ? "rejected" : "approved";
}

function routeSessionSocket(context: SocketRouteContext): boolean {
  const { socketId, id, action, payload } = context;
  const sessionId = String(payload.session_id || "");
  if (action === "session.subscribe") return subscribeSession(socketId, id, sessionId);
  if (action === "session.focus") {
    respond(socketId, id, { success: true });
    notifyGitStatus(sessionId);
    return true;
  }
  if (action === "session.unfocus") {
    respond(socketId, id, { success: true });
    return true;
  }
  if (action === "session.git.commits") {
    respond(socketId, id, { commits: demoGitData(sessionId).commits, ready: true });
    return true;
  }
  if (action === "session.cumulative_diff") {
    respond(socketId, id, { cumulative_diff: demoGitData(sessionId).cumulativeDiff, ready: true });
    return true;
  }
  if (action === "session.ensure" || action === "session.launch") {
    ensureSession(socketId, id, payload);
    return true;
  }
  return false;
}

function subscribeSession(socketId: string, id: string, sessionId: string): true {
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

function ensureSession(socketId: string, id: string, payload: Record<string, unknown>) {
  const task = findTask(String(payload.task_id));
  if (!task) {
    respond(socketId, id, { message: TASK_NOT_FOUND }, true);
    return;
  }
  const existing = state.sessions.find((session) => session.task_id === task.id);
  const prompt = String(payload.prompt || task.description || "Implement this task");
  const session = existing ?? startAgent(task, prompt);
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
}

function routePlanReviewSocket(context: SocketRouteContext): boolean {
  const { socketId, id, action, payload } = context;
  if (action.startsWith("task.plan.")) {
    handlePlanRequest(socketId, id, action, payload);
    return true;
  }
  const sessionId = String(payload.session_id || "");
  if (action === "session.file_review.get") {
    respond(socketId, id, { reviews: serializeFileReviews(sessionId) });
    return true;
  }
  if (action === "session.file_review.update") {
    updateFileReview(sessionId, payload);
    respond(socketId, id, { success: true });
    return true;
  }
  if (action === "session.file_review.reset") {
    fileReviewsBySession.delete(sessionId);
    respond(socketId, id, { success: true });
    return true;
  }
  return false;
}

function updateFileReview(sessionId: string, payload: Record<string, unknown>) {
  const reviews = fileReviewsBySession.get(sessionId) ?? new Map();
  reviews.set(String(payload.file_path || ""), {
    reviewed: payload.reviewed === true,
    diffHash: String(payload.diff_hash || ""),
  });
  fileReviewsBySession.set(sessionId, reviews);
}

function routeWorkspaceReadSocket(context: SocketRouteContext): boolean {
  const { socketId, id, action, payload } = context;
  if (action === "workspace.tree.get") {
    const sessionId = String(payload.session_id || "");
    respond(socketId, id, {
      root: buildFileTree(String(payload.path || ""), workspaceFilesForSession(sessionId)),
    });
    return true;
  }
  if (action === "workspace.file.get" || action === "workspace.file.get_at_ref") {
    readWorkspaceFile(socketId, id, payload);
    return true;
  }
  if (action === "workspace.files.search") {
    const query = String(payload.query || "").toLowerCase();
    const limit = Number(payload.limit ?? 20);
    const workspaceFiles = workspaceFilesForSession(String(payload.session_id || ""));
    const matches = Object.keys(workspaceFiles)
      .filter((path) => path.toLowerCase().includes(query))
      .slice(0, limit);
    respond(socketId, id, { files: matches });
    return true;
  }
  return false;
}

function readWorkspaceFile(socketId: string, id: string, payload: Record<string, unknown>) {
  const path = workspaceFilePath(payload);
  const content = workspaceFilesForSession(String(payload.session_id || ""))[path];
  if (content === undefined) {
    respond(socketId, id, { message: `File not found: ${path}` }, true);
    return;
  }
  respond(socketId, id, { path, content, size: content.length, is_binary: false });
}

function routeWorkspaceMutationSocket(context: SocketRouteContext): boolean {
  const { socketId, id, action, payload } = context;
  if (action === "workspace.file.update") return updateWorkspaceFile(socketId, id, payload);
  if (action === "workspace.file.create") return createWorkspaceFile(socketId, id, payload);
  if (action === "workspace.file.delete") return deleteWorkspaceFile(socketId, id, payload);
  if (action === "workspace.file.rename") return renameWorkspaceFile(socketId, id, payload);
  return false;
}

function updateWorkspaceFile(socketId: string, id: string, payload: Record<string, unknown>): true {
  const path = workspaceFilePath(payload);
  const workspaceFiles = workspaceFilesForSession(String(payload.session_id || ""));
  const content = payload.desired_content;
  if (typeof content !== "string") {
    respond(socketId, id, { message: "The demo editor requires desired_content" }, true);
    return true;
  }
  workspaceFiles[path] = content;
  respond(socketId, id, {
    path,
    success: true,
    new_hash: simpleHash(content),
    resolution: "applied",
  });
  notifyFileChange(String(payload.session_id || ""), path, "write");
  return true;
}

function createWorkspaceFile(socketId: string, id: string, payload: Record<string, unknown>): true {
  const path = workspaceFilePath(payload);
  workspaceFilesForSession(String(payload.session_id || ""))[path] = "";
  respond(socketId, id, { path, success: true });
  notifyFileChange(String(payload.session_id || ""), path, "create");
  return true;
}

function deleteWorkspaceFile(socketId: string, id: string, payload: Record<string, unknown>): true {
  const path = workspaceFilePath(payload);
  const workspaceFiles = workspaceFilesForSession(String(payload.session_id || ""));
  for (const filePath of Object.keys(workspaceFiles)) {
    if (filePath === path || filePath.startsWith(`${path}/`)) delete workspaceFiles[filePath];
  }
  respond(socketId, id, { path, success: true });
  notifyFileChange(String(payload.session_id || ""), path, "remove");
  return true;
}

function renameWorkspaceFile(socketId: string, id: string, payload: Record<string, unknown>): true {
  const repositoryPrefix = normalizeFilePath(payload.repo);
  const oldPath = withRepositoryPrefix(normalizeFilePath(payload.old_path), repositoryPrefix);
  const newPath = withRepositoryPrefix(normalizeFilePath(payload.new_path), repositoryPrefix);
  const workspaceFiles = workspaceFilesForSession(String(payload.session_id || ""));
  for (const filePath of Object.keys(workspaceFiles)) {
    if (filePath !== oldPath && !filePath.startsWith(`${oldPath}/`)) continue;
    workspaceFiles[`${newPath}${filePath.slice(oldPath.length)}`] = workspaceFiles[filePath];
    delete workspaceFiles[filePath];
  }
  respond(socketId, id, { old_path: oldPath, new_path: newPath, success: true });
  notifyFileChange(String(payload.session_id || ""), newPath, "rename");
  return true;
}

function routeShellSocket({ socketId, id, action }: SocketRouteContext): boolean {
  if (action === "user_shell.list") {
    respond(socketId, id, { shells: [demoTerminal()] });
    return true;
  }
  if (action === "user_shell.create") {
    respond(socketId, id, { ...demoTerminal(), terminal_id: DEMO_TERMINAL_ID });
    return true;
  }
  const mutations = new Set([
    "user_shell.destroy",
    "user_shell.stop",
    "user_shell.rename",
    "user_shell.park",
    "user_shell.resume",
  ]);
  if (!mutations.has(action)) return false;
  respond(socketId, id, { success: true });
  return true;
}

function routeMessageSocket(context: SocketRouteContext): boolean {
  const { socketId, id, action, payload } = context;
  const sessionId = String(payload.session_id || "");
  const messages = state.messagesBySession[sessionId] ?? [];
  if (action === "message.list") {
    respond(socketId, id, { messages: [...messages].reverse(), has_more: false });
    return true;
  }
  if (action !== "message.search") return false;
  const query = String(payload.query || "").toLowerCase();
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
  return true;
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
  const handler = PLAN_REQUEST_HANDLERS[action];
  if (handler) handler({ socketId, id, payload });
  else respond(socketId, id, { message: `Unsupported plan action: ${action}` }, true);
}

type PlanRequestContext = Omit<SocketRouteContext, "action">;
type PlanRequestHandler = (context: PlanRequestContext) => void;

const PLAN_REQUEST_HANDLERS: Record<string, PlanRequestHandler> = {
  "task.plan.get": getPlan,
  "task.plan.revisions.list": listPlanRevisions,
  "task.plan.revision.get": getPlanRevision,
  "task.plan.delete": deletePlan,
  "task.plan.revert": revertPlan,
  "task.plan.implementation_started": startPlanImplementation,
  "task.plan.create": (context) => savePlan(context, "task.plan.created"),
  "task.plan.update": (context) => savePlan(context, "task.plan.updated"),
};

function getPlan({ socketId, id, payload }: PlanRequestContext) {
  respond(socketId, id, plansByTask[String(payload.task_id || "")] ?? null);
}

function listPlanRevisions({ socketId, id, payload }: PlanRequestContext) {
  const plan = plansByTask[String(payload.task_id || "")];
  respond(socketId, id, { revisions: plan ? [demoPlanRevision(plan, false)] : [] });
}

function getPlanRevision({ socketId, id, payload }: PlanRequestContext) {
  const revision = Object.values(plansByTask)
    .map((plan) => demoPlanRevision(plan, true))
    .find((candidate) => candidate.id === payload.revision_id);
  respond(socketId, id, revision ?? null);
}

function deletePlan({ socketId, id, payload }: PlanRequestContext) {
  const taskId = String(payload.task_id || "");
  delete plansByTask[taskId];
  respond(socketId, id, { success: true });
  notify("task.plan.deleted", { task_id: taskId });
}

function revertPlan({ socketId, id, payload }: PlanRequestContext) {
  const plan = plansByTask[String(payload.task_id || "")];
  respond(socketId, id, plan ? demoPlanRevision(plan, true) : null);
}

function startPlanImplementation({ socketId, id, payload }: PlanRequestContext) {
  const taskId = String(payload.task_id || "");
  const plan = plansByTask[taskId];
  if (!plan) {
    respond(socketId, id, { message: PLAN_NOT_FOUND }, true);
    return;
  }
  const updated: TaskPlan = {
    ...plan,
    implementation_started_at: new Date().toISOString(),
    implementation_started_session_id: String(payload.session_id || ""),
    implementation_started_by: String(payload.actor || "user"),
  };
  plansByTask[taskId] = updated;
  respond(socketId, id, updated);
  notify("task.plan.updated", updated);
}

function savePlan(
  { socketId, id, payload }: PlanRequestContext,
  event: "task.plan.created" | "task.plan.updated",
) {
  const taskId = String(payload.task_id || "");
  const previous = plansByTask[taskId];
  const now = new Date().toISOString();
  const plan: TaskPlan = {
    id: previous?.id ?? `demo-plan-${taskId}`,
    task_id: taskId,
    title: String(payload.title || previous?.title || "Plan"),
    content: String(payload.content ?? previous?.content ?? ""),
    created_by: payload.created_by === "agent" ? "agent" : "user",
    created_at: previous?.created_at ?? now,
    updated_at: now,
  };
  plansByTask[taskId] = plan;
  respond(socketId, id, plan);
  notify(event, plan);
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
  if (
    task?.state === "REVIEW" &&
    state.messagesBySession[sessionId]?.some((message) => message.id === `${sessionId}-summary`)
  ) {
    return completedDemoTaskGitData(sessionId, task.id);
  }
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

function completedDemoTaskGitData(sessionId: string, taskId: string): DemoGitData {
  const files: Record<string, FileInfo> = {
    "src/pages/dashboard-page.tsx": {
      path: "src/pages/dashboard-page.tsx",
      status: "modified",
      staged: false,
      additions: 7,
      deletions: 1,
      diff: "@@ -8,1 +8,1 @@\n-return <main><h1>Operations</h1></main>;\n+return <main><h1>Operations</h1><ServiceHealthSummary /></main>;",
    },
    "tests/dashboard.test.tsx": {
      path: "tests/dashboard.test.tsx",
      status: "added",
      staged: false,
      additions: 28,
      deletions: 0,
      diff: '@@ -0,0 +1,5 @@\n+describe("service health", () => {\n+  it("shows current service details", () => {\n+    expect(renderDashboard()).toHaveTextContent("12 services healthy");\n+  });\n+});',
    },
  };
  return {
    status: makeGitStatus(`kandev/${taskId}`, files, 0, 0),
    commits: [],
    cumulativeDiff: makeCumulativeDiff(sessionId, files, 0),
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
  const session = makeSession(sessionId, task.id, "RUNNING", task.repositories);
  state.sessions.push(session);
  task.primary_session_id = session.id;
  task.primary_session_state = "RUNNING";
  task.session_count = 1;
  task.state = "IN_PROGRESS";
  task.workflow_step_id = DEMO_IDS.steps.progress;
  const user = makeMessage(`${sessionId}-user`, sessionId, task.id, "user", prompt);
  state.messagesBySession[sessionId] = [user];
  notify(TASK_UPDATED_EVENT, taskEvent(task));
  notify("session.message.added", messageEvent(user));
  scheduleAgentRun(task, session);
  return session;
}

function scheduleAgentRun(task: Task, session: TaskSession) {
  const messages = makeAgentRunMessages(task, session.id);
  messages.forEach((message, index) => {
    setTimeout(
      () => {
        if (!state.tasks.includes(task) || !state.sessions.includes(session)) return;
        state.messagesBySession[session.id]?.push(message);
        notify("session.message.added", messageEvent(message));
        if (index === messages.length - 1) finishAgentRun(task, session);
        persist();
      },
      (index + 1) * 450,
    );
  });
}

function finishAgentRun(task: Task, session: TaskSession) {
  const now = new Date().toISOString();
  const oldSessionState = session.state;
  session.state = "IDLE";
  session.updated_at = now;
  task.state = "REVIEW";
  task.workflow_step_id = DEMO_IDS.steps.review;
  task.primary_session_state = "IDLE";
  task.primary_session_pending_action = null;
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

// The detailed tool metadata is intentionally colocated so the streamed turn stays coherent.
// eslint-disable-next-line max-lines-per-function
function makeAgentRunMessages(task: Task, sessionId: string): Message[] {
  const turnId = `${sessionId}-implementation`;
  const worktree = `/demo/worktrees/${task.id}`;
  return [
    makeMessage(
      `${sessionId}-thinking`,
      sessionId,
      task.id,
      "agent",
      "I will trace the user-facing path first, then make the smallest implementation change and verify it with a focused regression test.",
      {
        type: "thinking",
        turnId,
        metadata: {
          thinking:
            "The task touches an existing workflow, so I should understand its entry point and current coverage before editing.",
        },
      },
    ),
    makeMessage(
      `${sessionId}-search`,
      sessionId,
      task.id,
      "agent",
      "Searched the application for the relevant feature path",
      {
        type: "tool_search",
        turnId,
        metadata: {
          status: "complete",
          normalized: {
            code_search: {
              query: task.title,
              path: "src",
              output: {
                files: ["src/app.tsx", "src/pages/dashboard-page.tsx", "tests/dashboard.test.tsx"],
                file_count: 3,
              },
            },
          },
        },
      },
    ),
    makeMessage(
      `${sessionId}-read`,
      sessionId,
      task.id,
      "agent",
      "Read src/pages/dashboard-page.tsx and its focused tests",
      {
        type: "tool_read",
        turnId,
        metadata: {
          status: "complete",
          normalized: {
            read_file: {
              file_path: `${worktree}/src/pages/dashboard-page.tsx`,
              offset: 1,
              limit: 160,
              output: { line_count: 42, language: "tsx", truncated: false },
            },
          },
        },
      },
    ),
    makeMessage(
      `${sessionId}-edit`,
      sessionId,
      task.id,
      "agent",
      "Updated the dashboard behavior and added regression coverage",
      {
        type: "tool_edit",
        turnId,
        metadata: {
          status: "complete",
          normalized: {
            modify_file: {
              file_path: `${worktree}/src/pages/dashboard-page.tsx`,
              mutations: [
                {
                  type: "patch",
                  old_content: "return <main><h1>Operations</h1></main>;",
                  new_content: "return <main><h1>Operations</h1><ServiceHealthSummary /></main>;",
                  diff: "@@ -8,1 +8,1 @@\n-return <main><h1>Operations</h1></main>;\n+return <main><h1>Operations</h1><ServiceHealthSummary /></main>;",
                  start_line: 8,
                  end_line: 8,
                },
              ],
            },
          },
        },
      },
    ),
    makeMessage(
      `${sessionId}-test`,
      sessionId,
      task.id,
      "agent",
      "pnpm test tests/dashboard.test.tsx --runInBand",
      {
        type: "tool_execute",
        turnId,
        metadata: {
          status: "complete",
          normalized: {
            shell_exec: {
              command: "pnpm test tests/dashboard.test.tsx --runInBand",
              work_dir: worktree,
              description: "Run focused dashboard regression tests",
              output: { exit_code: 0, has_output: true, stdout_bytes: 986, stderr_bytes: 0 },
            },
          },
        },
      },
    ),
    makeMessage(
      `${sessionId}-summary`,
      sessionId,
      task.id,
      "agent",
      `Implemented **${task.title}** and added focused coverage for the updated behavior.\n\n\`\`\`text\nPASS tests/dashboard.test.tsx\nTests: 4 passed, 4 total\n\`\`\`\n\nThe task is ready for review.`,
      { turnId },
    ),
  ];
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

function buildFileTree(path: string, workspaceFiles: Record<string, string>) {
  const normalizedPath = normalizeFilePath(path);
  const prefix = normalizedPath ? `${normalizedPath}/` : "";
  const childNames = new Set<string>();
  for (const filePath of Object.keys(workspaceFiles)) {
    if (!filePath.startsWith(prefix)) continue;
    const relative = filePath.slice(prefix.length);
    if (relative) childNames.add(relative.split("/")[0]);
  }
  const children = Array.from(childNames, (name) => {
    const childPath = prefix + name;
    const isDirectory = Object.keys(workspaceFiles).some((filePath) =>
      filePath.startsWith(`${childPath}/`),
    );
    return {
      name,
      path: childPath,
      is_dir: isDirectory,
      size: isDirectory ? undefined : (workspaceFiles[childPath]?.length ?? 0),
    };
  });
  return {
    name: normalizedPath.split("/").pop() || demoRepository.name,
    path: normalizedPath,
    is_dir: true,
    children,
  };
}

function workspaceFilesForSession(sessionId: string) {
  const session = state.sessions.find((candidate) => candidate.id === sessionId);
  const task = session ? findTask(session.task_id) : undefined;
  return (task?.repositories?.length ?? 0) > 1 ? multiRepoFiles : files;
}

function workspaceFilePath(payload: Record<string, unknown>) {
  return withRepositoryPrefix(normalizeFilePath(payload.path), normalizeFilePath(payload.repo));
}

function withRepositoryPrefix(path: string, repositoryPrefix: string) {
  if (!repositoryPrefix || path === repositoryPrefix || path.startsWith(`${repositoryPrefix}/`)) {
    return path;
  }
  return `${repositoryPrefix}/${path}`;
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
    primary_session_pending_action: task.primary_session_pending_action,
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
    raw_content: message.raw_content,
    type: message.type,
    metadata: message.metadata,
    requests_input: message.requests_input,
    turn_id: message.turn_id,
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

function makeWorkflowRuntime() {
  return createDemoWorkflowRuntime({
    snapshot: state.workflowRuntime,
    getTasks: activeTasks,
    onChange(snapshot) {
      state.workflowRuntime = snapshot;
      persist();
    },
    notify,
  });
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
