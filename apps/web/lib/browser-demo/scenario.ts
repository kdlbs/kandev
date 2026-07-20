/* eslint-disable max-lines, max-lines-per-function, max-params, sonarjs/no-duplicate-string */

import type { BootPayload } from "@/src/boot-payload";
import type {
  Message,
  Repository,
  Task,
  TaskPlan,
  TaskSession,
  Workflow,
  WorkflowStep,
} from "@/lib/types/http";
import type { GitHubPR, PRFeedback, TaskPR } from "@/lib/types/github";

export const DEMO_STORAGE_KEY = "kandev-browser-demo:v1";
export const DEMO_SCENARIO_VERSION = 2;

export const DEMO_IDS = {
  workspace: "demo-workspace",
  workflow: "demo-workflow",
  repository: "demo-repository",
  apiRepository: "demo-api-repository",
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
const API_REPOSITORY_ID = DEMO_IDS.apiRepository as never;

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

export const demoPRFeedback: PRFeedback = {
  pr: demoGitHubPR,
  reviews: [
    {
      id: 701,
      author: "mira",
      author_avatar: "https://github.com/identicons/mira.png",
      state: "COMMENTED",
      body: "The event model looks good. I left two small notes on retention and redaction.",
      created_at: NOW,
    },
  ],
  comments: [
    {
      id: 801,
      author: "mira",
      author_avatar: "https://github.com/identicons/mira.png",
      author_is_bot: false,
      body: "Can we keep actor IP addresses out of the payload and store only the region?",
      path: "src/audit/record-event.ts",
      line: 48,
      side: "RIGHT",
      comment_type: "review",
      created_at: NOW,
      updated_at: NOW,
      in_reply_to: null,
    },
    {
      id: 802,
      author: "kandev-demo",
      author_avatar: "https://github.com/identicons/kandev-demo.png",
      author_is_bot: false,
      body: "Done. The event now records the coarse region returned by the privacy filter.",
      path: "src/audit/record-event.ts",
      line: 48,
      side: "RIGHT",
      comment_type: "review",
      created_at: NOW,
      updated_at: NOW,
      in_reply_to: 801,
    },
  ],
  checks: [
    {
      name: "test",
      source: "check_run",
      status: "completed",
      conclusion: "success",
      html_url: "https://github.com/kandev-demo/acme-web/actions/runs/142",
      output: "428 tests passed",
      started_at: NOW,
      completed_at: NOW,
    },
  ],
  has_issues: true,
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

export const demoApiRepository: Repository = {
  ...demoRepository,
  id: API_REPOSITORY_ID,
  name: "acme-api",
  local_path: "/demo/acme-api",
  provider_repo_id: "kandev-demo/acme-api",
  provider_name: "acme-api",
  setup_script: "go mod download",
  dev_script: "go run ./cmd/api",
  copy_files: ".env",
};

export const demoUpgradePlan: TaskPlan = {
  id: "demo-plan-react",
  task_id: "demo-task-react",
  title: "React 19 multi-repository upgrade",
  content: `# React 19 multi-repository upgrade

1. Update React, the test renderer, and type packages in **acme-web**.
2. Regenerate the client fixtures consumed by **acme-api** contract tests.
3. Run both repositories' focused suites and compare serialized response snapshots.
4. Roll out behind the existing compatibility flag, then remove the flag after CI is green.

## Verification

- \`pnpm test --filter web\`
- \`go test ./internal/contracts/...\`
- Browser smoke test for checkout, settings, and task creation`,
  created_by: "agent",
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
    makeTask(
      "demo-task-react",
      "Upgrade React dependencies",
      DEMO_IDS.steps.backlog,
      "IN_PROGRESS",
      0,
      {
        description:
          "Plan a coordinated dependency upgrade across the web application and API contract tests.",
        primarySessionId: "demo-session-react",
        primarySessionState: "IDLE",
        repositories: [
          makeTaskRepository("demo-task-react", DEMO_IDS.repository, 0),
          makeTaskRepository("demo-task-react", DEMO_IDS.apiRepository, 1, "develop"),
        ],
      },
    ),
    makeTask("demo-task-empty", "Improve empty states", DEMO_IDS.steps.progress, "IN_PROGRESS", 1, {
      description: "Design useful empty states for first-time workspace members.",
      primarySessionId: "demo-session-empty",
      primarySessionState: "IDLE",
    }),
    makeTask("demo-task-auth", "Harden session refresh", DEMO_IDS.steps.done, "COMPLETED", 0),
  ];
  const sessions = [
    makeSession("demo-session-checkout", "demo-task-checkout", "RUNNING"),
    makeSession("demo-session-audit", "demo-task-audit", "IDLE"),
    makeSession("demo-session-react", "demo-task-react", "IDLE"),
    makeSession("demo-session-empty", "demo-task-empty", "IDLE"),
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
          { turnId: "checkout-turn" },
        ),
        makeMessage(
          "checkout-thinking",
          "demo-session-checkout",
          "demo-task-checkout",
          "agent",
          "The timeout only appears when inventory reservation and payment completion contend for the same order lock.",
          {
            type: "thinking",
            turnId: "checkout-turn",
            metadata: {
              thinking:
                "The timeout only appears when inventory reservation and payment completion contend for the same order lock.",
            },
          },
        ),
        makeMessage(
          "checkout-search",
          "demo-session-checkout",
          "demo-task-checkout",
          "agent",
          "Searched for order lock acquisition",
          {
            type: "tool_search",
            turnId: "checkout-turn",
            metadata: {
              status: "complete",
              normalized: {
                code_search: {
                  query: "withOrderLock",
                  path: "src/checkout",
                  output: {
                    files: ["src/checkout/complete-order.ts", "src/checkout/reserve-inventory.ts"],
                    file_count: 2,
                  },
                },
              },
            },
          },
        ),
        makeMessage(
          "checkout-read",
          "demo-session-checkout",
          "demo-task-checkout",
          "agent",
          "Read src/checkout/complete-order.ts",
          {
            type: "tool_read",
            turnId: "checkout-turn",
            metadata: {
              status: "complete",
              normalized: {
                read_file: {
                  file_path: "/demo/worktrees/demo-task-checkout/src/checkout/complete-order.ts",
                  offset: 1,
                  limit: 160,
                  output: { line_count: 118, language: "typescript", truncated: false },
                },
              },
            },
          },
        ),
        makeMessage(
          "checkout-command",
          "demo-session-checkout",
          "demo-task-checkout",
          "agent",
          "pnpm test checkout --runInBand",
          {
            type: "tool_execute",
            turnId: "checkout-turn",
            metadata: {
              status: "complete",
              normalized: {
                shell_exec: {
                  command: "pnpm test checkout --runInBand",
                  work_dir: "/demo/worktrees/demo-task-checkout",
                  description: "Run checkout regression tests",
                  output: {
                    exit_code: 0,
                    has_output: true,
                    stdout_bytes: 1842,
                    stderr_bytes: 0,
                  },
                },
              },
            },
          },
        ),
        makeMessage(
          "checkout-edit",
          "demo-session-checkout",
          "demo-task-checkout",
          "agent",
          "Edited src/checkout/complete-order.ts",
          {
            type: "tool_edit",
            turnId: "checkout-turn",
            metadata: {
              status: "complete",
              normalized: {
                modify_file: {
                  file_path: "/demo/worktrees/demo-task-checkout/src/checkout/complete-order.ts",
                  mutations: [
                    {
                      type: "patch",
                      old_content: "await withOrderLock(order.id, completeOrder);",
                      new_content:
                        "await completePayment(order);\nawait withOrderLock(order.id, reserveInventory);",
                      diff: "@@ -42,1 +42,2 @@\n-await withOrderLock(order.id, completeOrder);\n+await completePayment(order);\n+await withOrderLock(order.id, reserveInventory);",
                      start_line: 42,
                      end_line: 43,
                    },
                  ],
                },
              },
            },
          },
        ),
        makeMessage(
          "checkout-summary",
          "demo-session-checkout",
          "demo-task-checkout",
          "agent",
          "The checkout path now holds the order lock only while reserving inventory. The focused regression suite passes:\n\n```text\nPASS checkout/concurrent-inventory.test.ts\nTests: 12 passed, 12 total\n```",
          { turnId: "checkout-turn" },
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
      "demo-session-react": [
        makeMessage(
          "react-user",
          "demo-session-react",
          "demo-task-react",
          "user",
          "Plan the React upgrade across acme-web and the API contract test repository before changing code.",
          { metadata: { plan_mode: true } },
        ),
        makeMessage(
          "react-thinking",
          "demo-session-react",
          "demo-task-react",
          "agent",
          "I need to align the client runtime, test renderer, and generated API fixtures so both repositories move together.",
          {
            type: "thinking",
            metadata: { thinking: "Mapping the cross-repository dependency edges." },
          },
        ),
        makeMessage(
          "react-plan",
          "demo-session-react",
          "demo-task-react",
          "agent",
          demoUpgradePlan.content,
          { type: "agent_plan" },
        ),
        makeMessage(
          "react-agent",
          "demo-session-react",
          "demo-task-react",
          "agent",
          "The implementation plan is ready. It keeps generated API fixtures compatible while the web runtime and tests move to React 19.",
        ),
      ],
      "demo-session-empty": [
        makeMessage(
          "empty-user",
          "demo-session-empty",
          "demo-task-empty",
          "user",
          "Improve the empty states for new workspace members.",
        ),
        makeMessage(
          "empty-thinking",
          "demo-session-empty",
          "demo-task-empty",
          "agent",
          "The correct call to action depends on whether new members can create repositories themselves.",
          { type: "thinking" },
        ),
        makeMessage(
          "empty-question",
          "demo-session-empty",
          "demo-task-empty",
          "agent",
          "Which action should the empty workspace emphasize?",
          {
            type: "clarification_request",
            requestsInput: true,
            metadata: {
              pending_id: "demo-clarification-empty-state",
              session_id: "demo-session-empty",
              task_id: "demo-task-empty",
              question_id: "empty-state-action",
              question_index: 0,
              question_total: 1,
              status: "pending",
              question: {
                id: "empty-state-action",
                title: "Primary action",
                prompt: "Which action should the empty workspace emphasize?",
                options: [
                  {
                    option_id: "connect",
                    label: "Connect repository",
                    description: "Guide members through connecting their first GitHub repository.",
                  },
                  {
                    option_id: "invite",
                    label: "Invite an administrator",
                    description: "Direct members to someone who can configure repository access.",
                  },
                ],
              },
            },
          },
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
        itemsByWorkspaceId: { [DEMO_IDS.workspace]: [demoRepository, demoApiRepository] },
        loadingByWorkspaceId: { [DEMO_IDS.workspace]: false },
        loadedByWorkspaceId: { [DEMO_IDS.workspace]: true },
      },
      repositoryBranches: {
        itemsByRepositoryId: {
          [DEMO_IDS.repository]: [{ name: "main", type: "local" }],
          [DEMO_IDS.apiRepository]: [
            { name: "main", type: "local" },
            { name: "develop", type: "local" },
          ],
        },
        loadingByRepositoryId: {
          [DEMO_IDS.repository]: false,
          [DEMO_IDS.apiRepository]: false,
        },
        loadedByRepositoryId: {
          [DEMO_IDS.repository]: true,
          [DEMO_IDS.apiRepository]: true,
        },
        fetchedAtByRepositoryId: {
          [DEMO_IDS.repository]: NOW,
          [DEMO_IDS.apiRepository]: NOW,
        },
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
      taskPlans: {
        byTaskId: { "demo-task-react": demoUpgradePlan },
        loadingByTaskId: { "demo-task-react": false },
        loadedByTaskId: { "demo-task-react": true },
        savingByTaskId: {},
        revisionsByTaskId: {},
        revisionsLoadingByTaskId: {},
        revisionsLoadedByTaskId: {},
        revisionContentCache: {},
        previewRevisionIdByTaskId: {},
        comparePairByTaskId: {},
        lastSeenUpdatedAtByTaskId: { "demo-task-react": NOW },
      },
      chatInput: { planModeBySessionId: { "demo-session-react": true } },
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
    repositories: options.repositories ?? [makeTaskRepository(id, DEMO_IDS.repository, 0)],
    primary_session_id: options.primarySessionId as never,
    primary_session_state: options.primarySessionState,
    session_count: options.primarySessionId ? 1 : 0,
    review_status: options.reviewStatus,
    created_at: NOW,
    updated_at: NOW,
  };
}

function makeTaskRepository(
  taskId: string,
  repositoryId: string,
  position: number,
  baseBranch = "main",
): NonNullable<Task["repositories"]>[number] {
  return {
    id: `${taskId}-repository-${position + 1}`,
    task_id: taskId as never,
    repository_id: repositoryId as never,
    base_branch: baseBranch,
    position,
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
  options: {
    type?: Message["type"];
    metadata?: Record<string, unknown>;
    requestsInput?: boolean;
    turnId?: string;
  } = {},
): Message {
  return {
    id,
    session_id: sessionId as never,
    task_id: taskId as never,
    author_type: author,
    author_id: author === "agent" ? DEMO_IDS.agent : "demo-user",
    content,
    type: options.type ?? "message",
    metadata: options.metadata,
    requests_input: options.requestsInput,
    turn_id: options.turnId,
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
