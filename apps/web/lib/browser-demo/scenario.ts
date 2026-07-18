/* eslint-disable max-lines-per-function, max-params, sonarjs/no-duplicate-string */

import type { BootPayload } from "@/src/boot-payload";
import type {
  Message,
  Repository,
  Task,
  TaskSession,
  Workflow,
  WorkflowStep,
} from "@/lib/types/http";
import type { GitHubPR, TaskPR } from "@/lib/types/github";

export const DEMO_STORAGE_KEY = "kandev-browser-demo:v1";
export const DEMO_SCENARIO_VERSION = 1;

export const DEMO_IDS = {
  workspace: "demo-workspace",
  workflow: "demo-workflow",
  repository: "demo-repository",
  profile: "demo-mock-profile",
  agent: "demo-mock-agent",
  steps: {
    backlog: "demo-step-backlog",
    progress: "demo-step-progress",
    review: "demo-step-review",
    done: "demo-step-done",
  },
} as const;

export type DemoState = {
  version: number;
  nextTask: number;
  tasks: Task[];
  sessions: TaskSession[];
  messagesBySession: Record<string, Message[]>;
  taskPRs: Record<string, TaskPR[]>;
};

const NOW = "2026-07-18T12:00:00.000Z";
const WORKSPACE_ID = DEMO_IDS.workspace as never;
const WORKFLOW_ID = DEMO_IDS.workflow as never;
const REPOSITORY_ID = DEMO_IDS.repository as never;

export const demoGitHubPR: GitHubPR = {
  number: 142,
  title: "Add privileged action audit trail",
  body: "Adds structured audit events for role and access changes, plus an admin activity view.",
  url: "https://api.github.com/repos/kandev-demo/acme-web/pulls/142",
  html_url: "https://github.com/kandev-demo/acme-web/pull/142",
  state: "open",
  head_branch: "kandev/audit-logging",
  base_branch: "main",
  author_login: "kandev-demo",
  repo_owner: "kandev-demo",
  repo_name: "acme-web",
  draft: false,
  mergeable: true,
  mergeable_state: "clean",
  additions: 184,
  deletions: 27,
  requested_reviewers: [{ login: "mira", type: "user" }],
  created_at: NOW,
  updated_at: NOW,
  merged_at: null,
  closed_at: null,
};

export const demoWorkflow: Workflow = {
  id: WORKFLOW_ID,
  workspace_id: WORKSPACE_ID,
  name: "Product delivery",
  description: "A realistic Kandev workflow running entirely in this browser.",
  agent_profile_id: DEMO_IDS.profile as never,
  sort_order: 0,
  style: "kanban",
  created_at: NOW,
  updated_at: NOW,
};

export const demoSteps: WorkflowStep[] = [
  makeStep(DEMO_IDS.steps.backlog, "Backlog", 0, "bg-neutral-400", "work", true),
  makeStep(DEMO_IDS.steps.progress, "In progress", 1, "bg-blue-500", "work"),
  makeStep(DEMO_IDS.steps.review, "Review", 2, "bg-amber-500", "review"),
  makeStep(DEMO_IDS.steps.done, "Done", 3, "bg-emerald-500", "approval"),
];

export const demoRepository: Repository = {
  id: REPOSITORY_ID,
  workspace_id: WORKSPACE_ID,
  name: "acme-web",
  source_type: "github",
  local_path: "/demo/acme-web",
  provider: "github",
  provider_repo_id: "kandev-demo/acme-web",
  provider_owner: "kandev-demo",
  provider_name: "acme-web",
  default_branch: "main",
  worktree_branch_prefix: "kandev/",
  pull_before_worktree: true,
  setup_script: "pnpm install",
  cleanup_script: "",
  dev_script: "pnpm dev",
  copy_files: ".env.local",
  created_at: NOW,
  updated_at: NOW,
};

export function createDemoState(): DemoState {
  const tasks = [
    makeTask(
      "demo-task-checkout",
      "Fix checkout timeout",
      DEMO_IDS.steps.progress,
      "IN_PROGRESS",
      0,
      {
        description: "Trace the intermittent checkout timeout and add a regression test.",
        primarySessionId: "demo-session-checkout",
        primarySessionState: "RUNNING",
      },
    ),
    makeTask("demo-task-audit", "Add audit logging", DEMO_IDS.steps.review, "REVIEW", 0, {
      description: "Record privileged account changes and expose them to workspace admins.",
      primarySessionId: "demo-session-audit",
      primarySessionState: "IDLE",
      reviewStatus: "pending",
    }),
    makeTask("demo-task-react", "Upgrade React dependencies", DEMO_IDS.steps.backlog, "TODO", 0),
    makeTask("demo-task-empty", "Improve empty states", DEMO_IDS.steps.backlog, "TODO", 1),
    makeTask("demo-task-auth", "Harden session refresh", DEMO_IDS.steps.done, "COMPLETED", 0),
  ];
  const sessions = [
    makeSession("demo-session-checkout", "demo-task-checkout", "RUNNING"),
    makeSession("demo-session-audit", "demo-task-audit", "IDLE"),
  ];
  return {
    version: DEMO_SCENARIO_VERSION,
    nextTask: 1,
    tasks,
    sessions,
    messagesBySession: {
      "demo-session-checkout": [
        makeMessage(
          "checkout-user",
          "demo-session-checkout",
          "demo-task-checkout",
          "user",
          "Investigate the checkout timeout and ship a tested fix.",
        ),
        makeMessage(
          "checkout-agent",
          "demo-session-checkout",
          "demo-task-checkout",
          "agent",
          "I reproduced the timeout under concurrent inventory updates. I am narrowing the lock scope and adding a concurrency regression test.",
        ),
      ],
      "demo-session-audit": [
        makeMessage(
          "audit-user",
          "demo-session-audit",
          "demo-task-audit",
          "user",
          "Add audit events for role and access changes.",
        ),
        makeMessage(
          "audit-agent",
          "demo-session-audit",
          "demo-task-audit",
          "agent",
          "Implemented the audit event schema, persistence path, and admin activity view. The pull request is ready for review.",
        ),
      ],
    },
    taskPRs: {
      "demo-task-audit": [makeTaskPR()],
    },
  };
}

export function createBootPayload(state: DemoState): BootPayload {
  const snapshot = createSnapshot(state.tasks);
  return {
    version: 1,
    runtime: { apiPrefix: "/api/v1", webSocketPath: "/ws", debug: false },
    initialState: {
      features: { office: false, plugins: false },
      workspaces: {
        items: [
          {
            id: DEMO_IDS.workspace,
            name: "Acme Platform",
            description: "Browser demo workspace",
            owner_id: "demo-user",
            default_agent_profile_id: DEMO_IDS.profile,
            created_at: NOW,
            updated_at: NOW,
          },
        ],
        activeId: DEMO_IDS.workspace,
      },
      workflows: {
        items: [
          {
            id: DEMO_IDS.workflow,
            workspaceId: DEMO_IDS.workspace,
            name: demoWorkflow.name,
            description: demoWorkflow.description,
            sortOrder: 0,
            agent_profile_id: DEMO_IDS.profile,
            style: "kanban",
          },
        ],
        activeId: DEMO_IDS.workflow,
      },
      repositories: {
        itemsByWorkspaceId: { [DEMO_IDS.workspace]: [demoRepository] },
        loadingByWorkspaceId: { [DEMO_IDS.workspace]: false },
        loadedByWorkspaceId: { [DEMO_IDS.workspace]: true },
      },
      repositoryBranches: {
        itemsByRepositoryId: { [DEMO_IDS.repository]: [{ name: "main", type: "local" }] },
        loadingByRepositoryId: { [DEMO_IDS.repository]: false },
        loadedByRepositoryId: { [DEMO_IDS.repository]: true },
        fetchedAtByRepositoryId: { [DEMO_IDS.repository]: NOW },
        fetchErrorByRepositoryId: {},
      },
      kanban: { ...snapshot, isLoading: false },
      kanbanMulti: { snapshots: { [DEMO_IDS.workflow]: snapshot }, isLoading: false },
      taskSessions: {
        items: Object.fromEntries(state.sessions.map((session) => [session.id, session])),
      },
      taskSessionsByTask: {
        itemsByTaskId: Object.fromEntries(
          state.tasks.map((task) => [
            task.id,
            state.sessions.filter((session) => session.task_id === task.id),
          ]),
        ),
        loadingByTaskId: Object.fromEntries(state.tasks.map((task) => [task.id, false])),
        loadedByTaskId: Object.fromEntries(state.tasks.map((task) => [task.id, true])),
      },
      messages: {
        bySession: state.messagesBySession,
        metaBySession: Object.fromEntries(
          Object.keys(state.messagesBySession).map((id) => [
            id,
            { isLoading: false, hasMore: false, oldestCursor: null },
          ]),
        ),
      },
      settingsAgents: {
        items: [
          {
            id: DEMO_IDS.agent,
            name: "mock",
            supports_mcp: true,
            profiles: [],
            capability_status: "ok",
            created_at: NOW,
            updated_at: NOW,
          },
        ],
      },
      agentProfiles: {
        items: [
          {
            id: DEMO_IDS.profile,
            label: "Mock agent - Browser demo",
            agent_id: DEMO_IDS.agent,
            agent_name: "mock",
            cli_passthrough: false,
            capability_status: "ok",
          },
        ],
        version: 0,
      },
      settingsData: { agentsLoaded: true, executorsLoaded: true },
      userSettings: {
        workspaceId: DEMO_IDS.workspace,
        workflowId: DEMO_IDS.workflow,
        repositoryIds: [DEMO_IDS.repository],
        taskCreateLastUsed: {
          repositoryId: DEMO_IDS.repository,
          branch: "main",
          agentProfileId: DEMO_IDS.profile,
          executorProfileId: null,
          synced: true,
        },
        enablePreviewOnClick: true,
        loaded: true,
      } as never,
      githubStatus: {
        status: {
          authenticated: true,
          auth_method: "gh_cli",
          username: "kandev-demo",
          token_configured: false,
          required_scopes: [],
        },
        loaded: true,
        loading: false,
      },
      taskPRs: { byTaskId: state.taskPRs },
    },
  };
}

export function createSnapshot(tasks: Task[]) {
  return {
    workflowId: DEMO_IDS.workflow,
    workflowName: demoWorkflow.name,
    steps: demoSteps.map((step) => ({
      id: step.id,
      title: step.name,
      color: step.color,
      position: step.position,
      events: step.events,
      allow_manual_move: true,
      is_start_step: step.is_start_step,
      agent_profile_id: DEMO_IDS.profile,
      stage_type: step.stage_type,
    })),
    tasks: tasks
      .filter((task) => !task.archived_at)
      .map((task) => ({
        id: task.id,
        workflowStepId: task.workflow_step_id,
        title: task.title,
        description: task.description,
        position: task.position,
        state: task.state,
        repositoryId: DEMO_IDS.repository,
        repositories: task.repositories,
        primarySessionId: task.primary_session_id,
        primarySessionState: task.primary_session_state,
        sessionCount: task.session_count,
        reviewStatus: task.review_status,
        updatedAt: task.updated_at,
        createdAt: task.created_at,
      })),
  };
}

export function createTaskFromInput(state: DemoState, input: Record<string, unknown>): Task {
  const id = `demo-task-created-${state.nextTask++}`;
  const repositories =
    Array.isArray(input.repositories) && input.repositories.length > 0
      ? (input.repositories as Array<Record<string, unknown>>)
      : [{ repository_id: DEMO_IDS.repository, base_branch: "main" }];
  return makeTask(
    id,
    String(input.title || "Untitled task"),
    String(input.workflow_step_id || DEMO_IDS.steps.backlog),
    input.start_agent ? "IN_PROGRESS" : "TODO",
    state.tasks.length,
    {
      description: String(input.description || ""),
      repositories: repositories.map((repository, index) => ({
        id: `${id}-repository-${index + 1}`,
        task_id: id as never,
        repository_id: String(repository.repository_id || DEMO_IDS.repository) as never,
        base_branch: String(repository.base_branch || "main"),
        position: index,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      })),
    },
  );
}

function makeStep(
  id: string,
  name: string,
  position: number,
  color: string,
  stageType: WorkflowStep["stage_type"],
  start = false,
): WorkflowStep {
  return {
    id,
    workflow_id: WORKFLOW_ID,
    name,
    position,
    color,
    allow_manual_move: true,
    is_start_step: start,
    show_in_command_panel: true,
    agent_profile_id: DEMO_IDS.profile,
    stage_type: stageType,
    created_at: NOW,
    updated_at: NOW,
  };
}

function makeTask(
  id: string,
  title: string,
  stepId: string,
  state: Task["state"],
  position: number,
  options: {
    description?: string;
    primarySessionId?: string;
    primarySessionState?: Task["primary_session_state"];
    reviewStatus?: Task["review_status"];
    repositories?: Task["repositories"];
  } = {},
): Task {
  return {
    id: id as never,
    workspace_id: WORKSPACE_ID,
    workflow_id: WORKFLOW_ID,
    workflow_step_id: stepId,
    position,
    title,
    description: options.description ?? "",
    state,
    priority: 2,
    repositories: options.repositories ?? [
      {
        id: `${id}-repository`,
        task_id: id as never,
        repository_id: REPOSITORY_ID,
        base_branch: "main",
        position: 0,
        created_at: NOW,
        updated_at: NOW,
      },
    ],
    primary_session_id: options.primarySessionId as never,
    primary_session_state: options.primarySessionState,
    session_count: options.primarySessionId ? 1 : 0,
    review_status: options.reviewStatus,
    created_at: NOW,
    updated_at: NOW,
  };
}

export function makeSession(id: string, taskId: string, state: TaskSession["state"]): TaskSession {
  return {
    id: id as never,
    task_id: taskId as never,
    name: "Mock agent",
    agent_profile_id: DEMO_IDS.profile as never,
    repository_id: REPOSITORY_ID,
    worktree_path: `/demo/worktrees/${taskId}`,
    worktree_branch: `kandev/${taskId}`,
    state,
    is_primary: true,
    started_at: NOW,
    updated_at: NOW,
  };
}

export function makeMessage(
  id: string,
  sessionId: string,
  taskId: string,
  author: "user" | "agent",
  content: string,
): Message {
  return {
    id,
    session_id: sessionId as never,
    task_id: taskId as never,
    author_type: author,
    author_id: author === "agent" ? DEMO_IDS.agent : "demo-user",
    content,
    type: "message",
    created_at: NOW,
    updated_at: NOW,
  };
}

function makeTaskPR(): TaskPR {
  return {
    id: "demo-pr-audit",
    task_id: "demo-task-audit",
    repository_id: DEMO_IDS.repository,
    owner: "kandev-demo",
    repo: "acme-web",
    pr_number: 142,
    pr_url: "https://github.com/kandev-demo/acme-web/pull/142",
    pr_title: "Add privileged action audit trail",
    head_branch: "kandev/audit-logging",
    base_branch: "main",
    author_login: "kandev-demo",
    state: "open",
    review_state: "pending",
    checks_state: "success",
    mergeable_state: "clean",
    review_count: 1,
    pending_review_count: 1,
    required_reviews: 1,
    comment_count: 2,
    unresolved_review_threads: 0,
    checks_total: 5,
    checks_passing: 5,
    additions: 184,
    deletions: 27,
    created_at: NOW,
    merged_at: null,
    closed_at: null,
    last_synced_at: NOW,
    updated_at: NOW,
  };
}
