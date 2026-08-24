import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { StateProvider } from "@/components/state-provider";
import type { HydrationState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import { TaskItem } from "./task-item";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { formatRelativeTime } from "@/lib/utils";

const TURN_FINISHED_ICON_TEST_ID = "task-state-turn-finished";
const WORKFLOW_COMPLETE_ICON_TEST_ID = "task-state-workflow-complete";
const RUNNING_ICON_TEST_ID = "task-state-running";
const BACKGROUND_ICON_TEST_ID = "task-state-background-running";
const WAITING_FOR_INPUT_ICON_TEST_ID = "task-state-waiting-for-input";
const PENDING_PERMISSION_ICON_TEST_ID = "task-state-pending-permission";
const INTERRUPTED_ICON_TEST_ID = "task-state-interrupted";
const AGENT_ERROR_ICON_TEST_ID = "task-agent-error-icon";
const PREPARING_PHASE = "preparing";
const DATA_LOADING_PHASE = "data-loading-phase";
const RUNNING_PHASE = "running";
const YELLOW_SPINNER_CLASS = "text-yellow-500";
const VIOLET_SPINNER_CLASS = "text-violet-500";
const PREPARING_SPINNER_CLASS = "text-muted-foreground/40";
const SPIN_CLASS = "animate-spin";
const SLOW_SPIN_CLASS = "[animation-duration:2s]";
const TASK_ACTIONS_LABEL = "Task actions";
const PR_ICON_TEST_ID = "pr-task-icon-t1";
const MR_ICON_TEST_ID = "mr-task-icon-t1";

afterEach(() => cleanup());

function renderTaskItem(
  props: Partial<ComponentProps<typeof TaskItem>> = {},
  initialState: HydrationState = {},
) {
  return render(
    <StateProvider initialState={initialState}>
      <TooltipProvider>
        <TaskItem title="Needs answer" state="REVIEW" {...props} />
      </TooltipProvider>
    </StateProvider>,
  );
}

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    task_id: "t1",
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
    task_id: "t1",
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

function expectPreparingSpinner(): void {
  const icon = screen.getByTestId(RUNNING_ICON_TEST_ID);
  expect(icon.getAttribute(DATA_LOADING_PHASE)).toBe(PREPARING_PHASE);
  expect(icon.classList.contains(PREPARING_SPINNER_CLASS)).toBe(true);
  expect(icon.classList.contains(SPIN_CLASS)).toBe(true);
  expect(icon.classList.contains(SLOW_SPIN_CLASS)).toBe(true);
}

describe("TaskItem status icon states", () => {
  it("shows the autopilot icon with an accessible description", () => {
    renderTaskItem({ autopilot: true });

    expect(screen.getByTestId("task-autopilot-icon")).not.toBeNull();
    expect(screen.getByLabelText(/autopilot task/i)).not.toBeNull();
  });

  it("shows a dashed progress check when the session is idle after a non-final turn", () => {
    renderTaskItem({ sessionState: "WAITING_FOR_INPUT" });

    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(WORKFLOW_COMPLETE_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).toBeNull();
  });

  it("shows the continuous check when the finished turn is on the final workflow step", () => {
    renderTaskItem({ sessionState: "COMPLETED", isOnLastWorkflowStep: true });

    expect(screen.queryByTestId(WORKFLOW_COMPLETE_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).toBeNull();
  });

  it("shows question icon when a clarification is pending", () => {
    renderTaskItem({ sessionState: "WAITING_FOR_INPUT", hasPendingClarification: true });

    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).toBeNull();
  });

  it("shows question icon when task state is waiting for input", () => {
    renderTaskItem({ state: "WAITING_FOR_INPUT", hasPendingClarification: false });

    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).toBeNull();
  });

  it("shows alert icon when a permission request is pending", () => {
    renderTaskItem({ sessionState: "WAITING_FOR_INPUT", hasPendingPermission: true });

    expect(screen.queryByTestId(PENDING_PERMISSION_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).toBeNull();
  });

  it("prefers permission icon over clarification icon when both are pending", () => {
    renderTaskItem({
      sessionState: "WAITING_FOR_INPUT",
      hasPendingClarification: true,
      hasPendingPermission: true,
    });

    expect(screen.queryByTestId(PENDING_PERMISSION_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).toBeNull();
  });

  it("shows a slower muted spinner while the workflow is scheduling", () => {
    renderTaskItem({ state: "SCHEDULING" });

    expectPreparingSpinner();
  });

  it("shows a slower muted spinner while the session is starting before progress", () => {
    renderTaskItem({ state: "TODO", sessionState: "STARTING" });

    expectPreparingSpinner();
  });

  it("shows a slower muted spinner for in-progress tasks while the session is starting", () => {
    renderTaskItem({ state: "IN_PROGRESS", sessionState: "STARTING" });

    expectPreparingSpinner();
  });
});

describe("TaskItem status icon fallbacks", () => {
  it("does not show a spinner for a created task waiting for manual start", () => {
    renderTaskItem({ state: "CREATED" });

    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("task-state-backlog")).not.toBeNull();
  });

  it("does not show a spinner for a created task with a prepared CREATED session", () => {
    renderTaskItem({ state: "CREATED", sessionState: "CREATED" });

    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("task-state-backlog")).not.toBeNull();
  });

  it("shows a running spinner while an in-progress session is created but not started", () => {
    renderTaskItem({ state: "IN_PROGRESS", sessionState: "CREATED" });

    const icon = screen.getByTestId(RUNNING_ICON_TEST_ID);
    expect(icon.getAttribute(DATA_LOADING_PHASE)).toBe(RUNNING_PHASE);
    expect(icon.classList.contains(YELLOW_SPINNER_CLASS)).toBe(true);
    expect(icon.classList.contains(SPIN_CLASS)).toBe(true);
  });

  it("does not show preparing spinner for a review task whose session transiently hits STARTING", () => {
    renderTaskItem({ state: "REVIEW", sessionState: "STARTING" });

    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
  });

  it("keeps running tasks on the normal running spinner", () => {
    renderTaskItem({ state: "IN_PROGRESS", sessionState: "RUNNING" });

    const icon = screen.getByTestId(RUNNING_ICON_TEST_ID);
    expect(icon.getAttribute(DATA_LOADING_PHASE)).toBe(RUNNING_PHASE);
    expect(icon.classList.contains(YELLOW_SPINNER_CLASS)).toBe(true);
    expect(icon.classList.contains(SPIN_CLASS)).toBe(true);
    expect(icon.classList.contains(PREPARING_SPINNER_CLASS)).toBe(false);
  });

  it("shows a progress check for completed review tasks without final-step context", () => {
    renderTaskItem({ sessionState: "COMPLETED" });

    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).toBeNull();
  });

  it("shows a separate agent error icon when the task has retained error details", () => {
    renderTaskItem({ agentErrorMessage: "peer disconnected before response" });

    const errorIcon = screen.getByTestId(AGENT_ERROR_ICON_TEST_ID);
    expect(errorIcon).not.toBeNull();
    expect(errorIcon.classList.contains("cursor-help")).toBe(true);
    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).not.toBeNull();
  });
});

describe("TaskItem queue status", () => {
  it("shows WIP queue status without replacing the queued prompt badge", () => {
    renderTaskItem({
      queuedCount: 2,
      wipQueue: { position: 2, total: 4, destinationTitle: "Review" },
    });

    expect(screen.getByTestId("sidebar-task-queued-count")).not.toBeNull();
    const queueStatus = screen.getByTestId("sidebar-task-wip-queue");
    expect(queueStatus.querySelector("svg")).not.toBeNull();
    expect(queueStatus.textContent).toBe("");
    expect(queueStatus.getAttribute("aria-label")).toBe("Position 2 of 4 in Review queue");
  });
});

describe("TaskItem interrupted icon", () => {
  it("shows the red interrupted icon for a task marked interrupted after a restart", () => {
    renderTaskItem({
      state: "REVIEW",
      sessionState: "WAITING_FOR_INPUT",
      interrupted: true,
    });

    expect(screen.queryByTestId(INTERRUPTED_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(WORKFLOW_COMPLETE_ICON_TEST_ID)).toBeNull();
  });

  it("does not show the interrupted icon while the task is running again", () => {
    renderTaskItem({
      state: "REVIEW",
      sessionState: "RUNNING",
      interrupted: true,
    });

    expect(screen.queryByTestId(INTERRUPTED_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).not.toBeNull();
  });

  it("does not show the interrupted icon for a terminal task", () => {
    renderTaskItem({
      state: "COMPLETED",
      sessionState: "COMPLETED",
      interrupted: true,
    });

    expect(screen.queryByTestId(INTERRUPTED_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).not.toBeNull();
  });

  it("keeps pending-input affordances over the interrupted icon", () => {
    renderTaskItem({
      state: "REVIEW",
      sessionState: "WAITING_FOR_INPUT",
      hasPendingClarification: true,
      interrupted: true,
    });

    expect(screen.queryByTestId(INTERRUPTED_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).not.toBeNull();
  });
});

describe("TaskItem autopilot identity", () => {
  it("does not show the autopilot icon when the marker is false", () => {
    renderTaskItem({ autopilot: false });

    expect(screen.queryByTestId("task-autopilot-icon")).toBeNull();
  });
});

describe("TaskItem actions", () => {
  it("keeps row-focus actions visible when no diff stats are available", () => {
    renderTaskItem({ isSelected: true });

    const actionButton = screen.getByRole("button", { name: TASK_ACTIONS_LABEL });
    const actionContainer = actionButton.parentElement;

    expect(actionContainer?.className).toContain("group-focus-within:opacity-100");
    expect(actionContainer?.className.split(/\s+/)).not.toContain("focus-within:opacity-100");
  });

  it("keeps diff stats as the idle affordance when the selected row owns focus", () => {
    renderTaskItem({
      isSelected: true,
      diffStats: { additions: 3488, deletions: 199 },
    });

    const diffStats = screen.getByTestId("sidebar-task-diff-stats");
    const actionButton = screen.getByRole("button", { name: TASK_ACTIONS_LABEL });
    const actionContainer = actionButton.parentElement;

    expect(diffStats.className).not.toContain("group-focus-within:opacity-0");
    expect(actionContainer?.className).not.toContain("group-focus-within:opacity-100");
    expect(actionContainer?.className).toContain("focus-within:opacity-100");
  });

  it("hides diff stats and keeps the action trigger visible when the context menu is open", () => {
    renderTaskItem({
      menuOpen: true,
      diffStats: { additions: 42, deletions: 7 },
    });

    const diffStats = screen.getByTestId("sidebar-task-diff-stats");
    const actionButton = screen.getByRole("button", { name: TASK_ACTIONS_LABEL });
    const actionContainer = actionButton.parentElement;

    expect(diffStats.className).toContain("opacity-0");
    expect(actionContainer?.className).toContain("opacity-100");
  });

  it("announces the task menu state", () => {
    renderTaskItem({ menuOpen: true });

    const actions = screen.getByRole("button", { name: TASK_ACTIONS_LABEL });
    expect(actions.getAttribute("aria-haspopup")).toBe("menu");
    expect(actions.getAttribute("aria-expanded")).toBe("true");
  });

  it("does not announce a closed menu as expanded while deleting", () => {
    renderTaskItem({ isDeleting: true });

    expect(
      screen.getByRole("button", { name: TASK_ACTIONS_LABEL }).getAttribute("aria-expanded"),
    ).toBe("false");
  });
});

describe("TaskItem background-running indicator", () => {
  // The sidebar row reads the task record's most-active-wins aggregate
  // (`foregroundActivity`), not the single most-active client session's substate,
  // so it agrees with the board/kanban card and open-task header.
  it("shows the background-running affordance (spinner, not a check) when the aggregate is background", () => {
    // The task's foreground turns are idle but spawned background work is live.
    // It must read distinctly from generating and never as the review/done check.
    renderTaskItem({
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
      foregroundActivity: "background",
    });

    const icon = screen.getByTestId(BACKGROUND_ICON_TEST_ID);
    expect(icon.classList.contains(VIOLET_SPINNER_CLASS)).toBe(true);
    expect(icon.classList.contains(SPIN_CLASS)).toBe(true);
    expect(icon.parentElement?.getAttribute("tabindex")).toBe("0");
    expect(icon.parentElement?.classList.contains("focus-visible:ring-2")).toBe(true);
    expect(screen.getByLabelText("Background work is running")).not.toBeNull();
    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).toBeNull();
  });

  it("reads background from the task aggregate even when the most-active client session is done", () => {
    // Multi-session task: the client store holds only the finished primary
    // (sessionState COMPLETED), but the backend aggregate says a secondary session
    // still runs background work. The aggregate wins — never a false done check.
    // This is the exact case the old session-substate derivation missed.
    renderTaskItem({
      state: "REVIEW",
      sessionState: "COMPLETED",
      foregroundActivity: "background",
    });

    expect(screen.queryByTestId(BACKGROUND_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).toBeNull();
  });

  it("shows pending clarification instead of background-running", () => {
    renderTaskItem({
      state: "WAITING_FOR_INPUT",
      sessionState: "WAITING_FOR_INPUT",
      foregroundActivity: "background",
      hasPendingClarification: true,
    });

    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(BACKGROUND_ICON_TEST_ID)).toBeNull();
  });

  it("shows pending permission instead of foreground-running", () => {
    renderTaskItem({
      state: "WAITING_FOR_INPUT",
      sessionState: "RUNNING",
      foregroundActivity: "generating",
      hasPendingClarification: true,
      hasPendingPermission: true,
    });

    expect(screen.queryByTestId(PENDING_PERMISSION_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).toBeNull();
  });

  it("shows task-wide pending permission when the selected session is still starting", () => {
    renderTaskItem({
      state: "IN_PROGRESS",
      sessionState: "STARTING",
      foregroundActivity: null,
      hasPendingPermission: true,
    });

    expect(screen.queryByTestId(PENDING_PERMISSION_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
  });

  it("shows task-wide pending input when the selected session is terminal", () => {
    renderTaskItem({
      state: "REVIEW",
      sessionState: "COMPLETED",
      foregroundActivity: null,
      hasPendingClarification: true,
      hasPendingPermission: true,
    });

    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(PENDING_PERMISSION_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).toBeNull();
  });

  it("drives the generating spinner from a generating aggregate even when the coarse session reads done", () => {
    // Multi-session most-active-wins: a generating secondary session keeps the row
    // on the gold generating spinner though the primary session finished.
    renderTaskItem({
      state: "REVIEW",
      sessionState: "COMPLETED",
      foregroundActivity: "generating",
    });

    const icon = screen.getByTestId(RUNNING_ICON_TEST_ID);
    expect(icon.getAttribute(DATA_LOADING_PHASE)).toBe(RUNNING_PHASE);
    expect(icon.classList.contains(YELLOW_SPINNER_CLASS)).toBe(true);
    expect(screen.queryByTestId(BACKGROUND_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).toBeNull();
  });

  it("falls back to the generating spinner when the aggregate substate is unknown", () => {
    // Safe default: an unknown/null aggregate on a live turn must render generating,
    // never done and never the background spinner.
    renderTaskItem({
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
      foregroundActivity: null,
    });

    expect(screen.queryByTestId(BACKGROUND_ICON_TEST_ID)).toBeNull();
    const icon = screen.getByTestId(RUNNING_ICON_TEST_ID);
    expect(icon.getAttribute(DATA_LOADING_PHASE)).toBe(RUNNING_PHASE);
    expect(icon.classList.contains(YELLOW_SPINNER_CLASS)).toBe(true);
  });
});

describe("TaskItem background-running lifecycle", () => {
  it("reflects the full generating → background → done sequence from the aggregate", () => {
    // A single-session task driven through all three states reads each on the sidebar
    // without opening the task.
    const { rerender } = renderTaskItem({
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
      foregroundActivity: "generating",
    });
    expect(screen.getByTestId(RUNNING_ICON_TEST_ID).classList.contains(YELLOW_SPINNER_CLASS)).toBe(
      true,
    );
    expect(screen.queryByTestId(BACKGROUND_ICON_TEST_ID)).toBeNull();

    rerender(
      <StateProvider>
        <TooltipProvider>
          <TaskItem
            title="Needs answer"
            state="IN_PROGRESS"
            sessionState="RUNNING"
            foregroundActivity="background"
          />
        </TooltipProvider>
      </StateProvider>,
    );
    expect(screen.queryByTestId(BACKGROUND_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();

    rerender(
      <StateProvider>
        <TooltipProvider>
          <TaskItem
            title="Needs answer"
            state="REVIEW"
            sessionState="COMPLETED"
            foregroundActivity={null}
          />
        </TooltipProvider>
      </StateProvider>,
    );
    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(BACKGROUND_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
  });
});

describe("TaskItem queued prompt count badge", () => {
  const QUEUED_BADGE_TEST_ID = "sidebar-task-queued-count";

  it("shows a mail badge with the count when prompts are queued", () => {
    renderTaskItem({ queuedCount: 3 });

    const badge = screen.getByTestId(QUEUED_BADGE_TEST_ID);
    expect(badge.textContent).toBe("3");
    expect(badge.querySelector("svg")).not.toBeNull();
  });

  it("hides the badge when nothing is queued", () => {
    renderTaskItem({ queuedCount: 0 });
    expect(screen.queryByTestId(QUEUED_BADGE_TEST_ID)).toBeNull();

    renderTaskItem({ queuedCount: undefined });
    expect(screen.queryByTestId(QUEUED_BADGE_TEST_ID)).toBeNull();
  });

  it("renders the metadata line for a row whose only metadata is the queued count", () => {
    renderTaskItem({ queuedCount: 2, updatedAt: undefined });

    const badge = screen.getByTestId(QUEUED_BADGE_TEST_ID);
    expect(badge.textContent).toBe("2");
  });

  it("localizes the accessible label with the count", () => {
    renderTaskItem({ queuedCount: 1 });

    expect(screen.getByTestId(QUEUED_BADGE_TEST_ID).getAttribute("aria-label")).toContain(
      "queued prompt",
    );
  });
});

describe("TaskItem activity timestamp", () => {
  it("shows activity time only when the active view sorts by activity", () => {
    const activityAt = "2026-07-20T00:00:00Z";
    const updatedAt = "2026-07-24T00:00:00Z";
    const { rerender } = renderTaskItem({
      updatedAt,
      lastActivityAt: activityAt,
      showActivityTime: true,
    });

    expect(screen.getByTestId("sidebar-task-time").textContent).toBe(
      formatRelativeTime(activityAt),
    );

    rerender(
      <StateProvider>
        <TooltipProvider>
          <TaskItem
            title="Needs answer"
            state="REVIEW"
            updatedAt={updatedAt}
            lastActivityAt={activityAt}
            showActivityTime={false}
          />
        </TooltipProvider>
      </StateProvider>,
    );
    expect(screen.getByTestId("sidebar-task-time").textContent).toBe(formatRelativeTime(updatedAt));
  });
});

describe("TaskItem task-row presentation", () => {
  it("moves relative time to the trailing slot and hides the details row", () => {
    renderTaskItem({
      taskId: "t1",
      updatedAt: "2026-07-24T00:00:00Z",
      repositoryPath: "acme/api",
      showRepository: true,
      diffStats: { additions: 2, deletions: 1 },
      taskRowPresentation: {
        detailsEnabled: false,
        detailOrder: ["relative_time", "repository", "pull_request_number"],
        visibleDetails: [],
        trailing: "relative_time",
      },
    });

    expect(screen.queryByTestId("sidebar-task-time")).toBeNull();
    expect(screen.queryByTestId("sidebar-task-repository")).toBeNull();
    expect(screen.queryByTestId("sidebar-task-diff-stats")).toBeNull();
    expect(screen.getByTestId("sidebar-task-trailing-time")).toBeTruthy();
  });
});

describe("TaskItem contribution badges", () => {
  it("renders the PR badge before the MR badge when the task has both (AC2)", () => {
    renderTaskItem(
      { taskId: "t1" },
      {
        taskPRs: { byTaskId: { t1: [makePR()] } },
        taskMRs: { byWorkspaceId: { ws1: { t1: [makeMR()] } } },
        workspaces: { items: [], activeId: "ws1" },
      },
    );

    const prIcon = screen.getByTestId(PR_ICON_TEST_ID);
    const mrIcon = screen.getByTestId(MR_ICON_TEST_ID);
    expect(prIcon.compareDocumentPosition(mrIcon) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("renders the MR badge alone when the task has an MR and no PR (AC3)", () => {
    renderTaskItem(
      { taskId: "t1" },
      {
        taskMRs: { byWorkspaceId: { ws1: { t1: [makeMR()] } } },
        workspaces: { items: [], activeId: "ws1" },
      },
    );

    expect(screen.getByTestId(MR_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(PR_ICON_TEST_ID)).toBeNull();
  });

  it("renders the prInfo fallback PR badge followed by the MR badge (AC4)", () => {
    renderTaskItem(
      { taskId: "t1", prInfo: { number: 7, state: "Open" } },
      {
        taskMRs: { byWorkspaceId: { ws1: { t1: [makeMR()] } } },
        workspaces: { items: [], activeId: "ws1" },
      },
    );

    const prIcon = screen.getByTestId(PR_ICON_TEST_ID);
    const mrIcon = screen.getByTestId(MR_ICON_TEST_ID);
    expect(prIcon.compareDocumentPosition(mrIcon) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("renders no MR badge when the task has no MRs (AC6)", () => {
    renderTaskItem(
      { taskId: "t1" },
      {
        taskPRs: { byTaskId: { t1: [makePR()] } },
        workspaces: { items: [], activeId: "ws1" },
      },
    );

    expect(screen.getByTestId(PR_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(MR_ICON_TEST_ID)).toBeNull();
  });

  it("orders PR, then MR, then the issue badge (AC22)", () => {
    renderTaskItem(
      { taskId: "t1", issueInfo: { url: "https://example.com/issues/42", number: 42 } },
      {
        taskPRs: { byTaskId: { t1: [makePR()] } },
        taskMRs: { byWorkspaceId: { ws1: { t1: [makeMR()] } } },
        workspaces: { items: [], activeId: "ws1" },
      },
    );

    const prIcon = screen.getByTestId(PR_ICON_TEST_ID);
    const mrIcon = screen.getByTestId(MR_ICON_TEST_ID);
    const issueIcon = screen.getByTestId("issue-task-icon");
    expect(prIcon.compareDocumentPosition(mrIcon) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(
      mrIcon.compareDocumentPosition(issueIcon) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("renders no MR badge with no active workspace, even when the task has MRs elsewhere (AC21)", () => {
    renderTaskItem(
      { taskId: "t1" },
      {
        taskMRs: { byWorkspaceId: { ws1: { t1: [makeMR()] } } },
        workspaces: { items: [], activeId: null },
      },
    );

    expect(screen.queryByTestId(MR_ICON_TEST_ID)).toBeNull();
  });
});
