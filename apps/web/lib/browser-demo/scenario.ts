/* eslint-disable max-lines, max-lines-per-function, max-params, sonarjs/no-duplicate-string */

import type { BootPayload } from "@/src/boot-payload";
import type {
  Agent,
  Executor,
  Message,
  Repository,
  Task,
  TaskPlan,
  TaskSession,
  Workflow,
  WorkflowStep,
} from "@/lib/types/http";
import type { GitHubPR, PRFeedback, TaskPR } from "@/lib/types/github";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import {
  agentProfileId as toAgentProfileId,
  repositoryId as toRepositoryId,
  sessionId as toSessionId,
  taskId as toTaskId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
} from "@/lib/types/ids";

export const DEMO_STORAGE_KEY = "kandev-browser-demo:v1";
export const DEMO_SCENARIO_VERSION = 4;

export const DEMO_IDS = {
  workspace: "demo-workspace",
  workflow: "demo-workflow",
  supportWorkflow: "demo-support-workflow",
  repository: "demo-repository",
  apiRepository: "demo-api-repository",
  profile: "demo-mock-profile",
  reviewProfile: "demo-review-profile",
  agent: "demo-mock-agent",
  executor: "demo-worktree-executor",
  executorProfile: "demo-worktree-profile",
  safeExecutorProfile: "demo-worktree-safe-profile",
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
const WORKSPACE_ID = toWorkspaceId(DEMO_IDS.workspace);
const WORKFLOW_ID = toWorkflowId(DEMO_IDS.workflow);
const SUPPORT_WORKFLOW_ID = toWorkflowId(DEMO_IDS.supportWorkflow);
const REPOSITORY_ID = toRepositoryId(DEMO_IDS.repository);
const API_REPOSITORY_ID = toRepositoryId(DEMO_IDS.apiRepository);

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
  sort_order: 0,
  style: "kanban",
  created_at: NOW,
  updated_at: NOW,
};

export const demoSupportWorkflow: Workflow = {
  ...demoWorkflow,
  id: SUPPORT_WORKFLOW_ID,
  name: "Incident response",
  description: "A focused workflow for diagnosing and validating production fixes.",
  agent_profile_id: toAgentProfileId(DEMO_IDS.reviewProfile),
  sort_order: 1,
};

export const demoWorkflows = [demoWorkflow, demoSupportWorkflow];

export const demoSteps: WorkflowStep[] = [
  makeStep(DEMO_IDS.steps.backlog, "Backlog", 0, "bg-neutral-400", "work", true),
  makeStep(DEMO_IDS.steps.progress, "In progress", 1, "bg-blue-500", "work"),
  makeStep(DEMO_IDS.steps.review, "Review", 2, "bg-amber-500", "review"),
  makeStep(DEMO_IDS.steps.done, "Done", 3, "bg-emerald-500", "approval"),
];

export const demoSupportSteps: WorkflowStep[] = [
  {
    ...demoSteps[0],
    id: "demo-support-step-triage",
    workflow_id: SUPPORT_WORKFLOW_ID,
    name: "Triage",
    color: "bg-rose-500",
  },
  {
    ...demoSteps[2],
    id: "demo-support-step-verify",
    workflow_id: SUPPORT_WORKFLOW_ID,
    name: "Verify",
    position: 1,
    color: "bg-cyan-500",
  },
  {
    ...demoSteps[3],
    id: "demo-support-step-resolved",
    workflow_id: SUPPORT_WORKFLOW_ID,
    name: "Resolved",
    position: 2,
  },
];

export const demoAgents: Agent[] = [
  {
    id: DEMO_IDS.agent,
    name: "mock",
    supports_mcp: true,
    capability_status: "ok",
    profiles: [
      makeAgentProfile(DEMO_IDS.profile, "Build", "demo-fast"),
      makeAgentProfile(DEMO_IDS.reviewProfile, "Review", "demo-review"),
    ],
    created_at: NOW,
    updated_at: NOW,
  },
];

export const demoExecutors: Executor[] = [
  {
    id: DEMO_IDS.executor,
    name: "Browser workspace",
    type: "worktree",
    status: "active",
    is_system: true,
    profiles: [
      makeExecutorProfile(DEMO_IDS.executorProfile, "Standard workspace"),
      makeExecutorProfile(DEMO_IDS.safeExecutorProfile, "Isolated workspace"),
    ],
    created_at: NOW,
    updated_at: NOW,
  },
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

export const demoAccessibleRepositories = [
  {
    full_name: "kandev-demo/acme-web",
    owner: "kandev-demo",
    name: "acme-web",
    private: false,
    default_branch: "main",
    description: "Customer checkout and account experience",
    pushed_at: "2026-07-18T11:48:00Z",
  },
  {
    full_name: "kandev-demo/acme-api",
    owner: "kandev-demo",
    name: "acme-api",
    private: true,
    default_branch: "main",
    description: "Payments, sessions, and audit APIs",
    pushed_at: "2026-07-18T10:22:00Z",
  },
];

export const demoRepositoryBranches: Record<string, { name: string }[]> = {
  "kandev-demo/acme-web": [
    { name: "main" },
    { name: "kandev/audit-logging" },
    { name: "fix/checkout-timeout" },
  ],
  "kandev-demo/acme-api": [
    { name: "main" },
    { name: "develop" },
    { name: "feature/session-refresh" },
  ],
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
      primarySessionState: "WAITING_FOR_INPUT",
      primarySessionPendingAction: "permission",
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
    makeTask("demo-task-auth", "Harden session refresh", DEMO_IDS.steps.done, "COMPLETED", 0, {
      description:
        "Rotate refresh tokens safely during concurrent requests and preserve device sessions.",
      primarySessionId: "demo-session-auth",
      primarySessionState: "IDLE",
    }),
  ];
  const sessions = [
    makeSession("demo-session-checkout", "demo-task-checkout", "RUNNING"),
    makeSession("demo-session-audit", "demo-task-audit", "WAITING_FOR_INPUT"),
    makeSession("demo-session-react", "demo-task-react", "IDLE"),
    makeSession("demo-session-empty", "demo-task-empty", "IDLE"),
    makeSession("demo-session-auth", "demo-task-auth", "IDLE"),
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
        makeMessage(
          "checkout-followup-user",
          "demo-session-checkout",
          "demo-task-checkout",
          "user",
          "Good find. Keep payment capture outside the lock, but make sure a gateway retry cannot reserve inventory twice.",
          { turnId: "checkout-followup" },
        ),
        makeMessage(
          "checkout-followup-thinking",
          "demo-session-checkout",
          "demo-task-checkout",
          "agent",
          "The existing inventory client accepts an idempotency key. I can reuse the order ID and assert the retry behavior without widening the critical section.",
          {
            type: "thinking",
            turnId: "checkout-followup",
            metadata: {
              thinking:
                "The inventory boundary already supports idempotency, so the regression belongs at the checkout orchestration layer.",
            },
          },
        ),
        makeMessage(
          "checkout-followup-todos",
          "demo-session-checkout",
          "demo-task-checkout",
          "agent",
          "Updated implementation checklist",
          {
            type: "todo",
            turnId: "checkout-followup",
            metadata: {
              todos: [
                { text: "Reproduce lock contention", status: "completed" },
                { text: "Narrow the order lock", status: "completed" },
                { text: "Cover idempotent gateway retries", status: "completed" },
                { text: "Run the full checkout suite", status: "in_progress" },
              ],
              previous_todo_snapshots: [
                {
                  todos: [
                    { text: "Reproduce lock contention", status: "completed" },
                    { text: "Narrow the order lock", status: "in_progress" },
                    { text: "Cover idempotent gateway retries", status: "pending" },
                  ],
                  created_at: NOW,
                },
              ],
            },
          },
        ),
        makeMessage(
          "checkout-followup-edit",
          "demo-session-checkout",
          "demo-task-checkout",
          "agent",
          "Edited tests/checkout/concurrent-inventory.test.ts",
          {
            type: "tool_edit",
            turnId: "checkout-followup",
            metadata: {
              status: "complete",
              normalized: {
                modify_file: {
                  file_path:
                    "/demo/worktrees/demo-task-checkout/tests/checkout/concurrent-inventory.test.ts",
                  mutations: [
                    {
                      type: "patch",
                      old_content: 'expect(result.status).toBe("confirmed");',
                      new_content:
                        'expect(result.status).toBe("confirmed");\nexpect(inventory.reserve).toHaveBeenCalledTimes(1);',
                      diff: '@@ -31,1 +31,2 @@\n expect(result.status).toBe("confirmed");\n+expect(inventory.reserve).toHaveBeenCalledTimes(1);',
                      start_line: 31,
                      end_line: 32,
                    },
                  ],
                },
              },
            },
          },
        ),
        makeMessage(
          "checkout-followup-command",
          "demo-session-checkout",
          "demo-task-checkout",
          "agent",
          "pnpm test tests/checkout --runInBand",
          {
            type: "tool_execute",
            turnId: "checkout-followup",
            metadata: {
              status: "running",
              normalized: {
                shell_exec: {
                  command: "pnpm test tests/checkout --runInBand",
                  work_dir: "/demo/worktrees/demo-task-checkout",
                  description: "Run the full checkout test suite",
                  output: { has_output: true, stdout_bytes: 936, stderr_bytes: 0 },
                },
              },
            },
          },
        ),
      ],
      "demo-session-audit": [
        makeMessage(
          "audit-user",
          "demo-session-audit",
          "demo-task-audit",
          "user",
          "Add audit events for role and access changes.",
          { turnId: "audit-implementation" },
        ),
        makeMessage(
          "audit-thinking",
          "demo-session-audit",
          "demo-task-audit",
          "agent",
          "Role changes currently write directly through three separate handlers. A shared append-only recorder will keep the event contract consistent.",
          {
            type: "thinking",
            turnId: "audit-implementation",
            metadata: {
              thinking:
                "Trace every privileged mutation first, then introduce one privacy-filtered event boundary.",
            },
          },
        ),
        makeMessage(
          "audit-search",
          "demo-session-audit",
          "demo-task-audit",
          "agent",
          "Searched for privileged role mutations",
          {
            type: "tool_search",
            turnId: "audit-implementation",
            metadata: {
              status: "complete",
              normalized: {
                code_search: {
                  query: "updateRole|revokeAccess|inviteMember",
                  path: "/demo/worktrees/demo-task-audit/src",
                  output: {
                    files: [
                      "src/admin/members/update-role.ts",
                      "src/admin/members/revoke-access.ts",
                      "src/admin/members/invite-member.ts",
                    ],
                    file_count: 3,
                  },
                },
              },
            },
          },
        ),
        makeMessage(
          "audit-schema-edit",
          "demo-session-audit",
          "demo-task-audit",
          "agent",
          "Created migrations/20260718_create_audit_events.sql",
          {
            type: "tool_edit",
            turnId: "audit-implementation",
            metadata: {
              status: "complete",
              normalized: {
                modify_file: {
                  file_path:
                    "/demo/worktrees/demo-task-audit/migrations/20260718_create_audit_events.sql",
                  mutations: [
                    {
                      type: "create",
                      content:
                        "CREATE TABLE audit_events (\n  id UUID PRIMARY KEY,\n  action TEXT NOT NULL,\n  actor_id UUID NOT NULL,\n  target_type TEXT NOT NULL,\n  target_id TEXT NOT NULL,\n  region TEXT NOT NULL,\n  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()\n);",
                      start_line: 1,
                      end_line: 9,
                    },
                  ],
                },
              },
            },
          },
        ),
        makeMessage(
          "audit-tests",
          "demo-session-audit",
          "demo-task-audit",
          "agent",
          "pnpm test tests/audit tests/components/audit-log.test.tsx",
          {
            type: "tool_execute",
            turnId: "audit-implementation",
            metadata: {
              status: "complete",
              normalized: {
                shell_exec: {
                  command: "pnpm test tests/audit tests/components/audit-log.test.tsx",
                  work_dir: "/demo/worktrees/demo-task-audit",
                  description: "Run audit service and activity view tests",
                  output: { exit_code: 0, has_output: true, stdout_bytes: 1260, stderr_bytes: 0 },
                },
              },
            },
          },
        ),
        makeMessage(
          "audit-agent",
          "demo-session-audit",
          "demo-task-audit",
          "agent",
          "Implemented the audit event schema, persistence path, and admin activity view. The pull request is ready for review.\n\n**Coverage:** role updates, access revocation, and member invitations. All 18 focused tests pass.",
          { turnId: "audit-implementation" },
        ),
        makeMessage(
          "audit-review-user",
          "demo-session-audit",
          "demo-task-audit",
          "user",
          "Mira left a review note: do not persist actor IP addresses. Keep enough location context for incident response and update the PR.",
          { turnId: "audit-review-fix" },
        ),
        makeMessage(
          "audit-review-read",
          "demo-session-audit",
          "demo-task-audit",
          "agent",
          "Read docs/architecture/audit-events.md",
          {
            type: "tool_read",
            turnId: "audit-review-fix",
            metadata: {
              status: "complete",
              normalized: {
                read_file: {
                  file_path: "/demo/worktrees/demo-task-audit/docs/architecture/audit-events.md",
                  offset: 1,
                  limit: 32,
                  output: {
                    content:
                      "Privileged audit payloads may include a coarse request region, but never raw IP addresses, credentials, or request bodies.",
                    line_count: 6,
                    language: "markdown",
                    truncated: false,
                  },
                },
              },
            },
          },
        ),
        makeMessage(
          "audit-review-edit",
          "demo-session-audit",
          "demo-task-audit",
          "agent",
          "Edited src/audit/record-event.ts",
          {
            type: "tool_edit",
            turnId: "audit-review-fix",
            metadata: {
              status: "complete",
              normalized: {
                modify_file: {
                  file_path: "/demo/worktrees/demo-task-audit/src/audit/record-event.ts",
                  mutations: [
                    {
                      type: "patch",
                      old_content: "requestIp: input.requestIp,",
                      new_content: "region: coarseRegion(input.requestIp),",
                      diff: "@@ -45,1 +45,1 @@\n-requestIp: input.requestIp,\n+region: coarseRegion(input.requestIp),",
                      start_line: 45,
                      end_line: 45,
                    },
                  ],
                },
              },
            },
          },
        ),
        makeMessage(
          "audit-review-tests",
          "demo-session-audit",
          "demo-task-audit",
          "agent",
          "pnpm test tests/audit/record-event.test.ts",
          {
            type: "tool_execute",
            turnId: "audit-review-fix",
            metadata: {
              status: "complete",
              normalized: {
                shell_exec: {
                  command: "pnpm test tests/audit/record-event.test.ts",
                  work_dir: "/demo/worktrees/demo-task-audit",
                  description: "Verify privacy-safe audit payloads",
                  output: { exit_code: 0, has_output: true, stdout_bytes: 684, stderr_bytes: 0 },
                },
              },
            },
          },
        ),
        makeMessage(
          "audit-review-summary",
          "demo-session-audit",
          "demo-task-audit",
          "agent",
          "Addressed Mira's review: raw IP addresses never cross the audit boundary now. The recorder stores only `internal`, `global`, or `global-ipv6`, and the new regression test asserts the source address is absent. PR #142 is updated and checks are green.",
          { turnId: "audit-review-fix" },
        ),
        makeMessage(
          "audit-migration-check",
          "demo-session-audit",
          "demo-task-audit",
          "agent",
          "Run the privacy-safe audit migration check against staging",
          {
            type: "tool_call",
            turnId: "audit-review-fix",
            metadata: {
              tool_call_id: "audit-migration-check",
              tool_name: "Bash",
              title: "Verify the audit migration on staging",
              status: "pending",
              args: {
                command: "pnpm audit:migrate:check --env staging",
                cwd: "/demo/worktrees/demo-task-audit",
              },
            },
          },
        ),
        makeMessage(
          "audit-migration-permission",
          "demo-session-audit",
          "demo-task-audit",
          "agent",
          "Approve staging migration verification",
          {
            type: "permission_request",
            turnId: "audit-review-fix",
            requestsInput: true,
            metadata: {
              pending_id: "audit-migration-permission",
              tool_call_id: "audit-migration-check",
              action_type: "command",
              action_details: {
                command: "pnpm audit:migrate:check --env staging",
                cwd: "/demo/worktrees/demo-task-audit",
                description:
                  "Runs a read-only schema compatibility check against the shared staging database.",
                raw_input: { command: "pnpm audit:migrate:check --env staging" },
              },
              options: [
                { option_id: "audit-allow-once", name: "Allow once", kind: "allow_once" },
                { option_id: "audit-reject-once", name: "Reject", kind: "reject_once" },
              ],
              status: "pending",
            },
          },
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
          "react-web-search",
          "demo-session-react",
          "demo-task-react",
          "agent",
          "Searched acme-web for React runtime dependencies",
          {
            type: "tool_search",
            turnId: "react-plan-turn",
            metadata: {
              status: "complete",
              normalized: {
                code_search: {
                  query: "react-dom|react-test-renderer|hydrateRoot",
                  path: "/demo/worktrees/demo-task-react/acme-web",
                  output: {
                    files: [
                      "acme-web/package.json",
                      "acme-web/src/main.tsx",
                      "acme-web/tests/setup.ts",
                    ],
                    file_count: 3,
                  },
                },
              },
            },
          },
        ),
        makeMessage(
          "react-web-read",
          "demo-session-react",
          "demo-task-react",
          "agent",
          "Read acme-web/package.json",
          {
            type: "tool_read",
            turnId: "react-plan-turn",
            metadata: {
              status: "complete",
              normalized: {
                read_file: {
                  file_path: "/demo/worktrees/demo-task-react/acme-web/package.json",
                  offset: 1,
                  limit: 80,
                  output: {
                    content:
                      '"react": "^18.3.1",\n"react-dom": "^18.3.1",\n"@types/react": "^18.3.18",\n"vitest": "^3.0.5"',
                    line_count: 42,
                    language: "json",
                    truncated: false,
                  },
                },
              },
            },
          },
        ),
        makeMessage(
          "react-api-read",
          "demo-session-react",
          "demo-task-react",
          "agent",
          "Read acme-api/internal/contracts/dashboard_fixture_test.go",
          {
            type: "tool_read",
            turnId: "react-plan-turn",
            metadata: {
              status: "complete",
              normalized: {
                read_file: {
                  file_path:
                    "/demo/worktrees/demo-task-react/acme-api/internal/contracts/dashboard_fixture_test.go",
                  offset: 1,
                  limit: 120,
                  output: {
                    content:
                      'func TestDashboardFixtureMatchesWebClient(t *testing.T) {\n  fixtures.AssertMatches(t, "../../../acme-web/src/api/generated")\n}',
                    line_count: 67,
                    language: "go",
                    truncated: false,
                  },
                },
              },
            },
          },
        ),
        makeMessage(
          "react-versions-command",
          "demo-session-react",
          "demo-task-react",
          "agent",
          "pnpm why react react-dom && go list -m all | rg acme-contracts",
          {
            type: "tool_execute",
            turnId: "react-plan-turn",
            metadata: {
              status: "complete",
              normalized: {
                shell_exec: {
                  command: "pnpm why react react-dom && go list -m all | rg acme-contracts",
                  work_dir: "/demo/worktrees/demo-task-react",
                  description: "Map dependency versions across both repositories",
                  output: { exit_code: 0, has_output: true, stdout_bytes: 1104, stderr_bytes: 0 },
                },
              },
            },
          },
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
          "react-agent",
          "demo-session-react",
          "demo-task-react",
          "agent",
          "The implementation plan is ready in the **Plan** tab. The key dependency is the generated dashboard contract: `acme-web` should land first, then `acme-api` can validate the regenerated fixture before either compatibility flag is removed.",
          { turnId: "react-plan-turn" },
        ),
      ],
      "demo-session-empty": [
        makeMessage(
          "empty-user",
          "demo-session-empty",
          "demo-task-empty",
          "user",
          "Improve the empty states for new workspace members.",
          { turnId: "empty-discovery" },
        ),
        makeMessage(
          "empty-read",
          "demo-session-empty",
          "demo-task-empty",
          "agent",
          "Read src/components/empty-workspace.tsx",
          {
            type: "tool_read",
            turnId: "empty-discovery",
            metadata: {
              status: "complete",
              normalized: {
                read_file: {
                  file_path: "/demo/worktrees/demo-task-empty/src/components/empty-workspace.tsx",
                  offset: 1,
                  limit: 90,
                  output: {
                    content:
                      "export function EmptyWorkspace() {\n  return <section><h2>No repositories</h2></section>;\n}",
                    line_count: 18,
                    language: "tsx",
                    truncated: false,
                  },
                },
              },
            },
          },
        ),
        makeMessage(
          "empty-agent-discovery",
          "demo-session-empty",
          "demo-task-empty",
          "agent",
          "The current state only says “No repositories” and offers no recovery path. I can tailor the action to repository permissions and keep the view useful for both admins and members. Should regular members be sent to setup or to an administrator?",
          { turnId: "empty-discovery" },
        ),
        makeMessage(
          "empty-user-constraints",
          "demo-session-empty",
          "demo-task-empty",
          "user",
          "Use permission-aware actions. Administrators can connect a repository; regular members should get a clear handoff. Before implementing, ask me which handoff we want to feature in the demo.",
          { turnId: "empty-clarification" },
        ),
        makeMessage(
          "empty-thinking",
          "demo-session-empty",
          "demo-task-empty",
          "agent",
          "The correct call to action depends on whether new members can create repositories themselves.",
          { type: "thinking", turnId: "empty-clarification" },
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
            turnId: "empty-clarification",
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
      "demo-session-auth": [
        makeMessage(
          "auth-user",
          "demo-session-auth",
          "demo-task-auth",
          "user",
          "Users are occasionally signed out when two tabs refresh at the same time. Fix the rotation race without weakening replay protection.",
          { turnId: "auth-diagnosis" },
        ),
        makeMessage(
          "auth-search",
          "demo-session-auth",
          "demo-task-auth",
          "agent",
          "Searched for refresh token rotation",
          {
            type: "tool_search",
            turnId: "auth-diagnosis",
            metadata: {
              status: "complete",
              normalized: {
                code_search: {
                  query: "rotateRefreshToken",
                  path: "/demo/worktrees/demo-task-auth/src/auth",
                  output: {
                    files: ["src/auth/refresh-session.ts", "src/auth/token-store.ts"],
                    file_count: 2,
                  },
                },
              },
            },
          },
        ),
        makeMessage(
          "auth-thinking",
          "demo-session-auth",
          "demo-task-auth",
          "agent",
          "Both tabs validate the same token before either transaction commits. The second request then treats a legitimate concurrent rotation as replay.",
          {
            type: "thinking",
            turnId: "auth-diagnosis",
            metadata: {
              thinking:
                "Serialize rotation by token family and allow one short grace-window handoff to the already-issued successor.",
            },
          },
        ),
        makeMessage(
          "auth-edit",
          "demo-session-auth",
          "demo-task-auth",
          "agent",
          "Edited src/auth/refresh-session.ts",
          {
            type: "tool_edit",
            turnId: "auth-diagnosis",
            metadata: {
              status: "complete",
              normalized: {
                modify_file: {
                  file_path: "/demo/worktrees/demo-task-auth/src/auth/refresh-session.ts",
                  mutations: [
                    {
                      type: "patch",
                      old_content: "return tokenStore.rotate(token);",
                      new_content:
                        "return tokenStore.withFamilyLock(token.familyId, () =>\n  tokenStore.rotateOrReuseSuccessor(token, ROTATION_GRACE_MS),\n);",
                      diff: "@@ -28,1 +28,3 @@\n-return tokenStore.rotate(token);\n+return tokenStore.withFamilyLock(token.familyId, () =>\n+  tokenStore.rotateOrReuseSuccessor(token, ROTATION_GRACE_MS),\n+);",
                      start_line: 28,
                      end_line: 30,
                    },
                  ],
                },
              },
            },
          },
        ),
        makeMessage(
          "auth-test",
          "demo-session-auth",
          "demo-task-auth",
          "agent",
          "pnpm test tests/auth/refresh-session.test.ts --runInBand",
          {
            type: "tool_execute",
            turnId: "auth-diagnosis",
            metadata: {
              status: "complete",
              normalized: {
                shell_exec: {
                  command: "pnpm test tests/auth/refresh-session.test.ts --runInBand",
                  work_dir: "/demo/worktrees/demo-task-auth",
                  description: "Run session rotation race tests",
                  output: { exit_code: 0, has_output: true, stdout_bytes: 1518, stderr_bytes: 0 },
                },
              },
            },
          },
        ),
        makeMessage(
          "auth-agent",
          "demo-session-auth",
          "demo-task-auth",
          "agent",
          "The concurrent refresh race is fixed with a per-family lock and a five-second successor handoff. Replay attempts outside that window still revoke the family.\n\n```text\nPASS refresh-session.test.ts (14 tests)\nPASS token-replay.test.ts (9 tests)\n```",
          { turnId: "auth-diagnosis" },
        ),
        makeMessage(
          "auth-followup-user",
          "demo-session-auth",
          "demo-task-auth",
          "user",
          "Please also verify that signing out from one device does not revoke a different device's token family.",
          { turnId: "auth-device-check" },
        ),
        makeMessage(
          "auth-device-test",
          "demo-session-auth",
          "demo-task-auth",
          "agent",
          "pnpm test tests/auth/device-sessions.test.ts",
          {
            type: "tool_execute",
            turnId: "auth-device-check",
            metadata: {
              status: "complete",
              normalized: {
                shell_exec: {
                  command: "pnpm test tests/auth/device-sessions.test.ts",
                  work_dir: "/demo/worktrees/demo-task-auth",
                  description: "Verify device-scoped revocation",
                  output: { exit_code: 0, has_output: true, stdout_bytes: 812, stderr_bytes: 0 },
                },
              },
            },
          },
        ),
        makeMessage(
          "auth-final",
          "demo-session-auth",
          "demo-task-auth",
          "agent",
          "Verified. Revocation is scoped to the current `deviceId`; the other device keeps its independent token family. The full auth suite passes, and the task is complete.",
          { turnId: "auth-device-check" },
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
  const supportSnapshot = createWorkflowSnapshot(
    state.tasks,
    demoSupportWorkflow,
    demoSupportSteps,
  );
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
            default_executor_id: DEMO_IDS.executor,
            default_agent_profile_id: DEMO_IDS.profile,
            created_at: NOW,
            updated_at: NOW,
          },
        ],
        activeId: DEMO_IDS.workspace,
      },
      workflows: {
        items: demoWorkflows.map((workflow) => ({
          id: workflow.id,
          workspaceId: workflow.workspace_id,
          name: workflow.name,
          description: workflow.description,
          sortOrder: workflow.sort_order,
          agent_profile_id: workflow.agent_profile_id,
          style: workflow.style,
        })),
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
      kanbanMulti: {
        snapshots: {
          [DEMO_IDS.workflow]: snapshot,
          [DEMO_IDS.supportWorkflow]: supportSnapshot,
        },
        isLoading: false,
      },
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
      environmentIdBySessionId: Object.fromEntries(
        state.sessions.flatMap((session) =>
          session.task_environment_id ? ([[session.id, session.task_environment_id]] as const) : [],
        ),
      ),
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
        items: demoAgents,
      },
      agentProfiles: {
        items: demoAgents[0].profiles.map((profile) => ({
          id: profile.id,
          label: `${profile.agentDisplayName} • ${profile.name}`,
          agent_id: DEMO_IDS.agent,
          agent_name: "mock",
          cli_passthrough: profile.cliPassthrough,
          capability_status: "ok" as const,
        })),
        version: 0,
      },
      availableAgents: { items: [], tools: [], loading: false, loaded: true },
      executors: { items: demoExecutors },
      settingsData: { agentsLoaded: true, executorsLoaded: true },
      userSettings: {
        ...defaultSettingsState.userSettings,
        workspaceId: DEMO_IDS.workspace,
        workflowId: DEMO_IDS.workflow,
        repositoryIds: [DEMO_IDS.repository],
        taskCreateLastUsed: {
          repositoryId: DEMO_IDS.repository,
          branch: "main",
          agentProfileId: DEMO_IDS.profile,
          executorProfileId: DEMO_IDS.executorProfile,
          synced: true,
        },
        enablePreviewOnClick: true,
        loaded: true,
      },
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
  return createWorkflowSnapshot(tasks, demoWorkflow, demoSteps);
}

export function createWorkflowSnapshot(tasks: Task[], workflow: Workflow, steps: WorkflowStep[]) {
  return {
    workflowId: workflow.id,
    workflowName: workflow.name,
    steps: steps.map((step) => ({
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
      .filter((task) => !task.archived_at && task.workflow_id === workflow.id)
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
        primarySessionPendingAction: task.primary_session_pending_action,
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
      workflowId: String(input.workflow_id || DEMO_IDS.workflow),
      repositories: repositories.map((repository, index) => ({
        id: `${id}-repository-${index + 1}`,
        task_id: toTaskId(id),
        repository_id: toRepositoryId(String(repository.repository_id || DEMO_IDS.repository)),
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
    primarySessionPendingAction?: Task["primary_session_pending_action"];
    reviewStatus?: Task["review_status"];
    workflowId?: string;
    repositories?: Task["repositories"];
  } = {},
): Task {
  return {
    id: toTaskId(id),
    workspace_id: WORKSPACE_ID,
    workflow_id: toWorkflowId(options.workflowId ?? WORKFLOW_ID),
    workflow_step_id: stepId,
    position,
    title,
    description: options.description ?? "",
    state,
    priority: 2,
    repositories: options.repositories ?? [makeTaskRepository(id, DEMO_IDS.repository, 0)],
    primary_session_id: options.primarySessionId
      ? toSessionId(options.primarySessionId)
      : undefined,
    primary_session_state: options.primarySessionState,
    primary_session_pending_action: options.primarySessionPendingAction,
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
    task_id: toTaskId(taskId),
    repository_id: toRepositoryId(repositoryId),
    base_branch: baseBranch,
    position,
    created_at: NOW,
    updated_at: NOW,
  };
}

export function makeSession(id: string, taskId: string, state: TaskSession["state"]): TaskSession {
  return {
    id: toSessionId(id),
    task_id: toTaskId(taskId),
    name: "Mock agent",
    agent_profile_id: toAgentProfileId(DEMO_IDS.profile),
    repository_id: REPOSITORY_ID,
    worktree_path: `/demo/worktrees/${taskId}`,
    worktree_branch: `kandev/${taskId}`,
    task_environment_id: `demo-environment-${taskId}`,
    state,
    is_primary: true,
    started_at: NOW,
    updated_at: NOW,
  };
}

function makeAgentProfile(id: string, name: string, model: string): Agent["profiles"][number] {
  return {
    id: toAgentProfileId(id),
    name,
    agentId: DEMO_IDS.agent,
    agentDisplayName: "Mock agent",
    model,
    allowIndexing: false,
    autoApprove: true,
    cliFlags: [],
    cliPassthrough: false,
    createdAt: NOW,
    updatedAt: NOW,
  };
}

function makeExecutorProfile(id: string, name: string): NonNullable<Executor["profiles"]>[number] {
  return {
    id,
    executor_id: DEMO_IDS.executor,
    executor_type: "worktree",
    executor_name: "Browser workspace",
    name,
    prepare_script: "",
    cleanup_script: "",
    created_at: NOW,
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
    session_id: toSessionId(sessionId),
    task_id: toTaskId(taskId),
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
