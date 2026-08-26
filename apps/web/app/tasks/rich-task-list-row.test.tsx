import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import type { HydrationState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import { sessionId as toSessionId, taskId as toTaskId, type Task } from "@/lib/types/http";
import { TaskListRowPrimaryContent } from "./rich-task-list-row";

afterEach(cleanup);

const PR_ICON_TEST_ID = "pr-task-icon-task-1";
const MR_ICON_TEST_ID = "mr-task-icon-task-1";

function makeTask(overrides: Partial<Task> = {}): Task {
  return {
    id: toTaskId("task-1"),
    title: "A task",
    state: "WAITING_FOR_INPUT",
    workflow_step_id: "step-1",
    primary_session_id: toSessionId("session-1"),
    ...overrides,
  } as Task;
}

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    workspace_id: "ws1",
    task_id: "task-1",
    owner: "o",
    repo: "r",
    pr_number: 1,
    pr_url: "",
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

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "mr-1",
    task_id: "task-1",
    host: "https://gitlab.com",
    project_path: "acme/api",
    mr_iid: 1,
    mr_url: "",
    mr_title: "Test MR",
    head_branch: "feat",
    base_branch: "main",
    author_username: "alice",
    state: "open",
    approval_state: "",
    pipeline_state: "",
    merge_status: "",
    draft: false,
    approval_count: 0,
    required_approvals: 0,
    pipeline_jobs_total: 0,
    pipeline_jobs_pass: 0,
    reviewer_count: 0,
    unapproved_reviewers: 0,
    unresolved_discussions: 0,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function renderRow(showTaskDetails: boolean, initialState: HydrationState = {}) {
  return render(
    <StateProvider initialState={initialState}>
      <TooltipProvider>
        <TaskListRowPrimaryContent
          task={makeTask()}
          level={0}
          repositories={[]}
          parentTasks={[]}
          showTaskDetails={showTaskDetails}
        />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("TaskListRowPrimaryContent, contributions shown (AC8, AC9, AC23)", () => {
  it("renders the PR badge before the MR badge inside a single wrapper with the gap classes", () => {
    renderRow(true, {
      taskPRs: { byTaskId: { "task-1": [makePR()] } },
      taskMRs: { byWorkspaceId: { ws1: { "task-1": [makeMR()] } } },
      workspaces: { items: [], activeId: "ws1" },
    });

    const prIcon = screen.getByTestId(PR_ICON_TEST_ID);
    const mrIcon = screen.getByTestId(MR_ICON_TEST_ID);
    expect(prIcon.compareDocumentPosition(mrIcon) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    const wrapper = prIcon.parentElement;
    expect(wrapper).not.toBeNull();
    expect(wrapper?.contains(mrIcon)).toBe(true);
    expect(wrapper?.classList.contains("inline-flex")).toBe(true);
    expect(wrapper?.classList.contains("items-center")).toBe(true);
    expect(wrapper?.classList.contains("gap-1")).toBe(true);
  });

  it("carries the wrapper classes unconditionally even without an MR badge", () => {
    renderRow(true, {
      taskPRs: { byTaskId: { "task-1": [makePR()] } },
      workspaces: { items: [], activeId: "ws1" },
    });

    const prIcon = screen.getByTestId(PR_ICON_TEST_ID);
    const wrapper = prIcon.parentElement;
    expect(wrapper?.classList.contains("inline-flex")).toBe(true);
    expect(wrapper?.classList.contains("items-center")).toBe(true);
    expect(wrapper?.classList.contains("gap-1")).toBe(true);
    expect(screen.queryByTestId(MR_ICON_TEST_ID)).toBeNull();
  });
});

describe("TaskListRowPrimaryContent, contributions hidden (AC10)", () => {
  it("renders neither badge on the compact path", () => {
    renderRow(false, {
      taskPRs: { byTaskId: { "task-1": [makePR()] } },
      taskMRs: { byWorkspaceId: { ws1: { "task-1": [makeMR()] } } },
      workspaces: { items: [], activeId: "ws1" },
    });

    expect(screen.queryByTestId(PR_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(MR_ICON_TEST_ID)).toBeNull();
  });
});
