/// <reference lib="webworker" />
import { type UtilityAgent } from "@/lib/api/domains/utility-api";
import { type QueuedMessage } from "@/lib/state/slices/session/types";
import { type Message, type Task, type TaskPlan, type TaskSession } from "@/lib/types/http";
import { type JiraConfig } from "@/lib/types/jira";
import { type LinearConfig, type LinearTeam } from "@/lib/types/linear";
import { type SentryConfig } from "@/lib/types/sentry";
import { createDemoConversationRuntime } from "./conversation-runtime";
import { createDemoJiraRuntime } from "./jira-runtime";
import { type DemoHttpResponse, type DemoWorkerRequest, type DemoWorkerResponse } from "./protocol";
import { createDemoRunTimers } from "./run-timers";
import {
  type DemoState,
  createBootPayload,
  createDemoState,
  DEMO_IDS,
  DEMO_SCENARIO_VERSION,
  demoUpgradePlan,
} from "./scenario";
import { createDemoSystemRuntime } from "./system-runtime";
import { removeTask } from "./task-http-runtime";
import { isTerminalSocket } from "./terminal-runtime";
import { createDemoWorkflowRuntime } from "./workflow-runtime";

export function restoreState(persisted?: string): DemoState {
  if (!persisted) return createDemoState();
  try {
    const parsed = JSON.parse(persisted) as DemoState;
    if (parsed.version !== DEMO_SCENARIO_VERSION) return createDemoState();
    for (const session of parsed.sessions) {
      if (session.state !== "RUNNING" && session.state !== "STARTING") continue;
      session.state = "IDLE";
      session.updated_at = new Date().toISOString();
      const task = parsed.tasks.find((item) => item.primary_session_id === session.id);
      if (task) {
        task.primary_session_state = "IDLE";
        task.updated_at = session.updated_at;
      }
    }
    return parsed;
  } catch {
    return createDemoState();
  }
}

export function createDemoPlans(): Record<string, TaskPlan> {
  return { [demoUpgradePlan.task_id]: { ...demoUpgradePlan } };
}

export function activeTasks() {
  return state.tasks.filter((task) => !task.archived_at);
}

export function findTask(id: string) {
  return state.tasks.find((task) => task.id === id);
}

export function parseBody(body?: string): Record<string, unknown> {
  if (!body) return {};
  try {
    return JSON.parse(body) as Record<string, unknown>;
  } catch {
    return {};
  }
}

export function taskEvent(task: Task) {
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

export function messageEvent(message: Message) {
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

export function respond(socketId: string, id: string, payload: unknown, error = false) {
  socketMessage(socketId, {
    id,
    type: error ? "error" : "response",
    action: "demo.response",
    payload,
  });
}

export function notify(action: string, payload: unknown) {
  conversationRuntime.publish(action, payload);
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

export function socketMessage(socketId: string, value: unknown) {
  post({ kind: "ws-event", socketId, event: "message", data: JSON.stringify(value) });
}

export function persist() {
  state.plansByTask = plansByTask;
  post({ kind: "persist", state: JSON.stringify(state) });
}

export function makeWorkflowRuntime() {
  return createDemoWorkflowRuntime({
    snapshot: state.workflowRuntime,
    getTasks: activeTasks,
    deleteTasks(matches) {
      for (const task of state.tasks.filter(matches)) removeTask(task);
    },
    onChange(snapshot) {
      state.workflowRuntime = snapshot;
      persist();
    },
    notify,
  });
}

export function json(body: unknown, status = 200): DemoHttpResponse {
  return { status, headers: { "Content-Type": "application/json" }, body };
}

export function empty(): DemoHttpResponse {
  return { status: 204 };
}

export function post(message: DemoWorkerResponse) {
  scope.postMessage(message);
}

export function demoWorkspace() {
  return createBootPayload(state).initialState?.workspaces?.items[0];
}

export function withDemoEnvironment(session: TaskSession): TaskSession {
  return { ...session, task_environment_id: session.task_environment_id ?? DEMO_ENVIRONMENT_ID };
}

export function demoTerminal() {
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

export const scope: DedicatedWorkerGlobalScope = self as never;

export let state = createDemoState();

export let plansByTask = createDemoPlans();

export let systemRuntime = createDemoSystemRuntime();

export let workflowRuntime = makeWorkflowRuntime();

export let jiraRuntime = createDemoJiraRuntime();

export const activeRuns = new Set<string>();

export const runTimers = createDemoRunTimers();

export const pendingReplies = new Map<string, Message[]>();

export const queuedMessages = new Map<string, QueuedMessage>();

export const acceptedQueueMessages = new Map<string, QueuedMessage>();

export const fileReviewsBySession = new Map<
  string,
  Map<string, { reviewed: boolean; diffHash: string }>
>();

export const socketUrls = new Map<string, string>();

export const terminalInputBySocket = new Map<string, string>();

export const conversationRuntime = createDemoConversationRuntime(socketMessage);

export const DEMO_ENVIRONMENT_ID = "demo-environment";

export const DEMO_TERMINAL_ID = "demo-terminal-1";

export const TASK_UPDATED_EVENT = "task.updated";

export const DEMO_TIMESTAMP = "2026-07-18T12:00:00.000Z";

export const DEMO_SLACK_UTILITY_AGENT_ID = "demo-utility-triage";

export const DEMO_JIRA_CONFIG: JiraConfig = {
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

export const DEMO_LINEAR_CONFIG: LinearConfig = {
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

// i18n-exempt: browser demo fixture data is intentionally literal demo content
export const DEMO_LINEAR_TEAMS: LinearTeam[] = [
  { id: "demo-linear-team-eng", key: "ENG", name: "Engineering" },
  { id: "demo-linear-team-plat", key: "PLAT", name: "Platform" },
];

// i18n-exempt: browser demo fixture data is intentionally literal demo content
export const DEMO_SENTRY_INSTANCES: SentryConfig[] = [
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

export const DEMO_SLACK_CONFIG = {
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

// i18n-exempt: browser demo fixture data is intentionally literal demo content
export const DEMO_UTILITY_AGENTS: UtilityAgent[] = [
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

// i18n-exempt: browser demo fixture data is intentionally literal demo content
export const DEMO_REPOSITORY_SCRIPTS = [
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

export function initializeDemoRuntime(message: Extract<DemoWorkerRequest, { kind: "init" }>) {
  runTimers.clear();
  state = restoreState(message.persistedState);
  plansByTask = state.plansByTask ?? createDemoPlans();
  systemRuntime = createDemoSystemRuntime();
  workflowRuntime = makeWorkflowRuntime();
  jiraRuntime = createDemoJiraRuntime();
  activeRuns.clear();
  pendingReplies.clear();
  queuedMessages.clear();
  acceptedQueueMessages.clear();
  fileReviewsBySession.clear();
  conversationRuntime.reset();
  post({ kind: "result", id: message.id, value: createBootPayload(state) });
  return;
}
