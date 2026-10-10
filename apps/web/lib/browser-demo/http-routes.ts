import { type SidebarTaskQuery } from "@/lib/types/http";
import { type DemoHttpResponse } from "./protocol";
import {
  activeTasks,
  DEMO_JIRA_CONFIG,
  DEMO_LINEAR_CONFIG,
  DEMO_LINEAR_TEAMS,
  DEMO_REPOSITORY_SCRIPTS,
  DEMO_SENTRY_INSTANCES,
  DEMO_SLACK_CONFIG,
  DEMO_UTILITY_AGENTS,
  demoTerminal,
  demoWorkspace,
  empty,
  findTask,
  json,
  notify,
  persist,
  state,
  TASK_UPDATED_EVENT,
  taskEvent,
  withDemoEnvironment,
  workflowRuntime,
} from "./runtime-state";
import {
  createBootPayload,
  DEMO_IDS,
  demoAccessibleRepositories,
  demoAgents,
  demoApiRepository,
  demoExecutors,
  demoGitHubPR,
  demoPRFeedback,
  demoRepository,
  demoRepositoryBranches,
} from "./scenario";
import { createDemoSidebarPage } from "./sidebar-runtime";
import { createDemoStats } from "./stats";
import { answerClarification, createTask, removeTask, updateTask } from "./task-http-runtime";

export function routeBootHttp(
  path: string,
  _method: string,
  _input: Record<string, unknown>,
  _url: URL,
): DemoHttpResponse | null {
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
  return null;
}

export function routeRepositoryHttp(
  path: string,
  method: string,
  _input: Record<string, unknown>,
  _url: URL,
): DemoHttpResponse | null {
  if (
    path === `/api/v1/repositories/${DEMO_IDS.repository}/branches` ||
    path === `/api/v1/repositories/${DEMO_IDS.apiRepository}/branches`
  )
    return json({ branches: [{ name: "main", type: "local" }], total: 1, current_branch: "main" });
  if (path === `/api/v1/repositories/${DEMO_IDS.repository}/scripts`)
    return json({ scripts: DEMO_REPOSITORY_SCRIPTS, total: DEMO_REPOSITORY_SCRIPTS.length });
  if (path === `/api/v1/repositories/${DEMO_IDS.apiRepository}/scripts`)
    return json({ scripts: [], total: 0 });
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/repository-sets` && method === "GET")
    return json({ repository_sets: [], total: 0 });
  if (path === "/api/v1/agents") return json({ agents: demoAgents, total: demoAgents.length });
  if (path === "/api/v1/agents/available") return json({ agents: [], tools: [], total: 0 });
  return null;
}

export function routeWorkspaceHttp(
  path: string,
  method: string,
  input: Record<string, unknown>,
  _url: URL,
): DemoHttpResponse | null {
  if (path === "/api/v1/executors")
    return json({ executors: demoExecutors, total: demoExecutors.length });
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/tasks`)
    return json({ tasks: activeTasks(), total: activeTasks().length });
  if (path === `/api/v1/workspaces/${DEMO_IDS.workspace}/sidebar/query` && method === "POST") {
    return json(createDemoSidebarPage(state, input as SidebarTaskQuery));
  }
  const statsMatch = path.match(
    new RegExp(`^/api/v1/workspaces/${DEMO_IDS.workspace}/stats/([^/]+)$`),
  );
  if (statsMatch) {
    const stats = createDemoStats(statsMatch[1], state);
    if (stats !== undefined) return json(stats);
  }

  return null;
}

export function routeGitHubAccountHttp(
  path: string,
  _method: string,
  _input: Record<string, unknown>,
  _url: URL,
): DemoHttpResponse | null {
  if (path === "/api/v1/github/status")
    return json({
      configured: true,
      authenticated: true,
      auth_method: "gh_cli",
      username: "kandev-demo",
    });
  if (path === "/api/v1/github/user/prs")
    return json({ prs: [demoGitHubPR], total_count: 1, page: 1, per_page: 25 });
  return null;
}

export function routeGitHubRepositoryHttp(
  path: string,
  _method: string,
  _input: Record<string, unknown>,
  _url: URL,
): DemoHttpResponse | null {
  if (path === "/api/v1/github/user/issues")
    return json({ issues: [], total_count: 0, page: 1, per_page: 25 });
  if (path === "/api/v1/github/repos") return json({ repos: demoAccessibleRepositories });
  const remoteBranchesMatch = path.match(/^\/api\/v1\/github\/repos\/([^/]+)\/([^/]+)\/branches$/);
  if (remoteBranchesMatch) {
    const owner = decodeURIComponent(remoteBranchesMatch[1]);
    const repo = decodeURIComponent(remoteBranchesMatch[2]);
    const branches = demoRepositoryBranches[`${owner}/${repo}`];
    return branches ? json({ branches }) : json({ error: "Repository not found" }, 404);
  }
  return null;
}

export function routeIntegrationSettingsHttp(
  path: string,
  _method: string,
  _input: Record<string, unknown>,
  _url: URL,
): DemoHttpResponse | null {
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

export function routeGitHubSettingsHttp(
  path: string,
  _method: string,
  _input: Record<string, unknown>,
  _url: URL,
): DemoHttpResponse | null {
  if (path === "/api/v1/github/watches/issues") return json({ watches: [] });
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
  return null;
}

export function routeTaskHttp(
  path: string,
  method: string,
  input: Record<string, unknown>,
  _url: URL,
): DemoHttpResponse | null {
  const clarification = path.match(/^\/api\/v1\/clarification\/([^/]+)\/respond$/);
  if (clarification && method === "POST") return answerClarification(clarification[1], input);
  if (path === "/api/v1/tasks" && method === "POST") return createTask(input);

  const taskMatch = path.match(/^\/api\/v1\/tasks\/([^/]+)$/);
  if (taskMatch) {
    const task = findTask(taskMatch[1]);
    if (!task) return json({ error: "Task not found" }, 404);
    if (method === "GET") return json(task);
    if (method === "PATCH") return updateTask(task, input);
    if (method === "DELETE") return removeTask(task);
  }
  return null;
}

export function routeSessionHttp(
  path: string,
  _method: string,
  _input: Record<string, unknown>,
  _url: URL,
): DemoHttpResponse | null {
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
  return null;
}

export function routeMovedTaskHttp(
  path: string,
  method: string,
  input: Record<string, unknown>,
  _url: URL,
): DemoHttpResponse | null {
  const moveMatch = path.match(/^\/api\/v1\/tasks\/([^/]+)\/move$/);
  if (moveMatch && method === "POST") {
    const task = findTask(moveMatch[1]);
    if (!task) return json({ error: "Task not found" }, 404);
    task.workflow_step_id = String(input.workflow_step_id || task.workflow_step_id);
    task.position = Number(input.position ?? task.position);
    task.updated_at = new Date().toISOString();
    persist();
    notify(TASK_UPDATED_EVENT, taskEvent(task));
    return json({
      task,
      workflow_step: workflowRuntime
        .snapshot()
        .steps.find((step) => step.id === task.workflow_step_id),
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

  return null;
}
