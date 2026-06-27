import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";

vi.mock("@kandev/ui/tooltip", () => ({
  TooltipProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
  Tooltip: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
}));

vi.mock("@/lib/api/domains/kanban-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/kanban-api")>();
  return {
    ...actual,
    fetchTask: vi.fn(() => new Promise(() => {})),
  };
});

import { StateProvider } from "@/components/state-provider";
import { PRRowTaskIndicator } from "./pr-row-task-indicator";
import { qk } from "@/lib/query/keys";
import type { TaskPR } from "@/lib/types/github";
import {
  taskId as toTaskId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type Task,
  type WorkflowSnapshot,
} from "@/lib/types/http";

const CREATED_AT = "2026-06-24T00:00:00Z";
const TASK_ID = toTaskId("task-1");
const WORKSPACE_ID = toWorkspaceId("workspace-1");
const WORKFLOW_ID = toWorkflowId("workflow-1");

function renderWithStore(ui: ReactNode, seed?: (client: QueryClient) => void) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  seed?.(queryClient);
  return render(
    <QueryClientProvider client={queryClient}>
      <StateProvider>
        <TooltipProvider>{ui}</TooltipProvider>
      </StateProvider>
    </QueryClientProvider>,
  );
}

function makeTaskPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-1",
    task_id: "task-1",
    owner: "o",
    repo: "r",
    pr_number: 1,
    pr_url: "https://github.com/o/r/pull/1",
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
    ...overrides,
  };
}

function queryTask(): Task {
  return {
    id: TASK_ID,
    workspace_id: WORKSPACE_ID,
    workflow_id: WORKFLOW_ID,
    workflow_step_id: "step-query",
    position: 0,
    title: "Query task",
    description: "",
    state: "TODO",
    priority: 0,
    repositories: [],
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

function workflowSnapshot(): WorkflowSnapshot {
  return {
    workflow: {
      id: WORKFLOW_ID,
      workspace_id: WORKSPACE_ID,
      name: "Workflow",
      sort_order: 0,
      hidden: false,
      created_at: CREATED_AT,
      updated_at: CREATED_AT,
    },
    steps: [
      {
        id: "step-query",
        workflow_id: WORKFLOW_ID,
        name: "Query Review",
        position: 0,
        color: "bg-blue-500",
        allow_manual_move: true,
      },
    ],
    tasks: [queryTask()],
  };
}

afterEach(() => cleanup());

describe("PRRowTaskIndicator", () => {
  it("shows 'No task created yet' when tasks is undefined", () => {
    renderWithStore(<PRRowTaskIndicator tasks={undefined} />);
    expect(screen.getByText("No task created yet")).toBeTruthy();
  });

  it("shows 'No task created yet' when tasks is empty", () => {
    renderWithStore(<PRRowTaskIndicator tasks={[]} />);
    expect(screen.getByText("No task created yet")).toBeTruthy();
  });

  it("renders a clickable button with task title for a single task", () => {
    renderWithStore(<PRRowTaskIndicator tasks={[makeTaskPR({ pr_title: "My Feature PR" })]} />);
    const btn = screen.getByRole("button");
    expect(btn.textContent).toContain("My Feature PR");
  });

  it("shows 'Tasks' trigger with count badge for multiple tasks", () => {
    const tasks = [
      makeTaskPR({ id: "a", task_id: "t1", pr_title: "First PR" }),
      makeTaskPR({ id: "b", task_id: "t2", pr_title: "Second PR" }),
    ];
    const { container } = renderWithStore(<PRRowTaskIndicator tasks={tasks} />);
    expect(screen.getByRole("button").textContent).toContain("Tasks");
    expect(container.textContent).toContain("2");
  });

  it("truncates long task titles (>40 chars)", () => {
    const longTitle = "This is an extremely long pull request title that should be truncated";
    renderWithStore(<PRRowTaskIndicator tasks={[makeTaskPR({ pr_title: longTitle })]} />);
    const btn = screen.getByRole("button");
    expect(btn.textContent).toContain("…");
    expect(btn.textContent!.length).toBeLessThan(longTitle.length + 5);
  });

  it("uses workflow snapshot Query cache for the single-task step tooltip", () => {
    renderWithStore(
      <PRRowTaskIndicator tasks={[makeTaskPR({ task_id: "task-1" })]} />,
      (client) => {
        client.setQueryData(qk.tasks.detail(TASK_ID), queryTask());
        client.setQueryData(qk.workflows.snapshot(WORKFLOW_ID), workflowSnapshot());
      },
    );

    expect(screen.getByText("Step: Query Review")).toBeTruthy();
  });
});
