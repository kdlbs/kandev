import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import { TaskTitleHoverCard } from "./task-title-hover-card";

afterEach(() => cleanup());

const LONG_TITLE = "A very long parent task title that should render in full";
const FIRST_SUBTASK_TITLE = "First subtask";
const HOVER_CARD_TEST_ID = "task-title-hover-card";

function makeTask(overrides: Partial<KanbanState["tasks"][number]>): KanbanState["tasks"][number] {
  return {
    id: "id",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    title: "Task",
    position: 0,
    ...overrides,
  };
}

function makePR(overrides: Pick<TaskPR, "id" | "task_id">): TaskPR {
  return {
    workspace_id: "workspace-1",
    owner: "o",
    repo: "r",
    pr_number: 1,
    pr_url: "",
    pr_title: "Fix",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "open" as const,
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

function makeMR(): TaskMR {
  return {
    id: "mr-1",
    task_id: "child-1",
    host: "https://gitlab.com",
    project_path: "group/project",
    mr_iid: 5,
    mr_url: "",
    mr_title: "Linked MR",
    head_branch: "feature",
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
  };
}

function renderCard(tasks: KanbanState["tasks"], title = LONG_TITLE) {
  render(
    <StateProvider initialState={{ kanban: { workflowId: "wf-1", steps: [], tasks } }}>
      <TaskTitleHoverCard taskId="parent-1" title={title}>
        <span data-testid="trigger">{title}</span>
      </TaskTitleHoverCard>
    </StateProvider>,
  );
}

// Keyboard activation opens the interactive preview and moves focus into it.
async function openCard() {
  const trigger = screen.getByTestId("task-title-preview-trigger");
  trigger.focus();
  fireEvent.click(trigger, { detail: 0 });
  await screen.findAllByTestId(HOVER_CARD_TEST_ID);
}

/** First (portal) match for a subtask row; Radix also force-mounts a copy. */
function subtaskRow(childId: string): HTMLElement {
  return screen.getAllByTestId(`task-subtask-row-${childId}`)[0];
}

describe("TaskTitleHoverCard", () => {
  it("AC12: shows the full title in bold (font-semibold) when opened", async () => {
    renderCard([
      makeTask({ id: "parent-1" }),
      makeTask({ id: "child-1", parentTaskId: "parent-1" }),
    ]);
    await openCard();

    const card = screen.getAllByTestId(HOVER_CARD_TEST_ID)[0];
    expect(card.textContent).toContain(LONG_TITLE);
    const titleEl = card.querySelector(".font-semibold");
    expect(titleEl?.textContent).toBe(LONG_TITLE);
  });

  it("AC13: renders one row per direct subtask with its full title", async () => {
    renderCard([
      makeTask({ id: "parent-1" }),
      makeTask({
        id: "child-1",
        title: FIRST_SUBTASK_TITLE,
        parentTaskId: "parent-1",
        position: 1,
      }),
      makeTask({ id: "child-2", title: "Second subtask", parentTaskId: "parent-1", position: 2 }),
    ]);
    await openCard();

    expect(subtaskRow("child-1").textContent).toContain(FIRST_SUBTASK_TITLE);
    expect(subtaskRow("child-2").textContent).toContain("Second subtask");
  });

  it("names the subtask link by its title, with the state label on the icon", async () => {
    renderCard([
      makeTask({ id: "parent-1" }),
      makeTask({
        id: "child-1",
        title: FIRST_SUBTASK_TITLE,
        parentTaskId: "parent-1",
        position: 1,
        state: "IN_PROGRESS",
      }),
    ]);
    await openCard();

    const row = subtaskRow("child-1");
    // An aria-label on the anchor would override its accessible name, so every
    // row would announce as "In progress" with the title never read out.
    expect(row.getAttribute("aria-label")).toBeNull();
    expect(row.textContent).toContain(FIRST_SUBTASK_TITLE);
    expect(row.querySelector('[role="img"]')?.getAttribute("aria-label")).toBe("In progress");
  });

  it("AC13: shows a CI glyph only for a subtask with a linked PR", async () => {
    render(
      <StateProvider
        initialState={{
          kanban: {
            workflowId: "wf-1",
            steps: [],
            tasks: [
              makeTask({ id: "parent-1" }),
              makeTask({
                id: "child-1",
                title: "Has a PR",
                parentTaskId: "parent-1",
                position: 1,
              }),
              makeTask({
                id: "child-2",
                title: "No PR",
                parentTaskId: "parent-1",
                position: 2,
              }),
            ],
          },
          taskPRs: {
            byTaskId: {
              "child-1": [makePR({ id: "pr-1", task_id: "child-1" })],
            },
          },
        }}
      >
        <TaskTitleHoverCard taskId="parent-1" title="Parent">
          <span tabIndex={0} data-testid="trigger">
            Parent
          </span>
        </TaskTitleHoverCard>
      </StateProvider>,
    );
    await openCard();

    const withPR = subtaskRow("child-1");
    const withoutPR = subtaskRow("child-2");
    expect(withPR.querySelector(".tabler-icon-git-pull-request")).not.toBeNull();
    expect(withoutPR.querySelector(".tabler-icon-git-pull-request")).toBeNull();
    expect(withoutPR.querySelector(".tabler-icon-git-merge")).toBeNull();
  });
});

describe("TaskTitleHoverCard — GitLab MR subtasks", () => {
  it("AC13: shows the merge glyph for a subtask whose only link is a GitLab MR", async () => {
    render(
      <StateProvider
        initialState={{
          workspaces: { items: [], activeId: "ws-1" },
          kanban: {
            workflowId: "wf-1",
            steps: [],
            tasks: [
              makeTask({ id: "parent-1" }),
              makeTask({
                id: "child-1",
                title: "Has an MR",
                parentTaskId: "parent-1",
                position: 1,
              }),
              makeTask({ id: "child-2", title: "No link", parentTaskId: "parent-1", position: 2 }),
            ],
          },
          taskMRs: { byWorkspaceId: { "ws-1": { "child-1": [makeMR()] } } },
        }}
      >
        <TaskTitleHoverCard taskId="parent-1" title="Parent">
          <span tabIndex={0} data-testid="trigger">
            Parent
          </span>
        </TaskTitleHoverCard>
      </StateProvider>,
    );
    await openCard();

    const withMR = subtaskRow("child-1");
    const withoutMR = subtaskRow("child-2");
    expect(withMR.querySelector(".tabler-icon-git-merge")).not.toBeNull();
    expect(withMR.querySelector(".tabler-icon-git-pull-request")).toBeNull();
    expect(withoutMR.querySelector(".tabler-icon-git-merge")).toBeNull();
  });
});

describe("TaskTitleHoverCard — subtask cap and dismissal", () => {
  it("AC15: does not mount a trigger when there are no subtasks", () => {
    renderCard([makeTask({ id: "parent-1" })]);

    expect(screen.queryByTestId("task-title-preview-trigger")).toBeNull();
    expect(screen.queryByTestId(HOVER_CARD_TEST_ID)).toBeNull();
  });

  it("AC17: caps subtask rows at 12 and shows a +N more line", async () => {
    const children = Array.from({ length: 15 }, (_, i) =>
      makeTask({
        id: `child-${i}`,
        title: `Subtask ${i}`,
        parentTaskId: "parent-1",
        position: i,
      }),
    );
    renderCard([makeTask({ id: "parent-1" }), ...children]);
    await openCard();

    for (let i = 0; i < 12; i++) {
      expect(screen.getAllByTestId(`task-subtask-row-child-${i}`).length).toBeGreaterThan(0);
    }
    for (let i = 12; i < 15; i++) {
      expect(screen.queryAllByTestId(`task-subtask-row-child-${i}`)).toHaveLength(0);
    }
    const subtasksSection = screen.getAllByTestId("task-title-hover-subtasks")[0];
    expect(subtasksSection.textContent).toContain("3 more");
  });

  it("AC19: never renders a TooltipContent inside the card", async () => {
    renderCard([
      makeTask({ id: "parent-1" }),
      makeTask({ id: "child-1", title: "Child", parentTaskId: "parent-1", position: 1 }),
    ]);
    await openCard();

    expect(document.querySelector('[data-slot="tooltip-content"]')).toBeNull();
  });

  it("does not bubble a click on the title (non-link) area to the parent row's onClick", async () => {
    const onParentClick = vi.fn();
    render(
      <StateProvider
        initialState={{
          kanban: {
            workflowId: "wf-1",
            steps: [],
            tasks: [
              makeTask({ id: "parent-1" }),
              makeTask({ id: "child-1", parentTaskId: "parent-1" }),
            ],
          },
        }}
      >
        <div onClick={onParentClick} data-testid="parent-row">
          <TaskTitleHoverCard taskId="parent-1" title={LONG_TITLE}>
            <span tabIndex={0} data-testid="trigger">
              {LONG_TITLE}
            </span>
          </TaskTitleHoverCard>
        </div>
      </StateProvider>,
    );
    await openCard();

    const card = screen.getAllByTestId(HOVER_CARD_TEST_ID)[0];
    const titleEl = card.querySelector(".font-semibold");
    expect(titleEl).not.toBeNull();
    fireEvent.click(titleEl!);

    expect(onParentClick).not.toHaveBeenCalled();
  });
});

describe("TaskTitleHoverCard — keyboard propagation", () => {
  it("lets Escape bubble from the trigger to a closed surface (e.g. a preview panel) when the preview itself is not open", () => {
    // Regression: a plain click focuses the trigger button without opening
    // the preview (handleTriggerClick early-returns for real clicks). If
    // Escape were unconditionally swallowed here, closing an ancestor
    // surface (like the kanban preview panel) via Escape would silently stop
    // working the moment its title happened to be under focus.
    const onParentKeyDown = vi.fn();
    render(
      <StateProvider
        initialState={{
          kanban: {
            workflowId: "wf-1",
            steps: [],
            tasks: [
              makeTask({ id: "parent-1" }),
              makeTask({ id: "child-1", parentTaskId: "parent-1" }),
            ],
          },
        }}
      >
        <div onKeyDown={onParentKeyDown}>
          <TaskTitleHoverCard taskId="parent-1" title={LONG_TITLE}>
            <span data-testid="trigger">{LONG_TITLE}</span>
          </TaskTitleHoverCard>
        </div>
      </StateProvider>,
    );
    const trigger = screen.getByTestId("task-title-preview-trigger");
    trigger.focus();
    fireEvent.keyDown(trigger, { key: "Escape" });

    expect(onParentKeyDown).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId(HOVER_CARD_TEST_ID)).toBeNull();
  });

  it("does not bubble keyboard activation from a subtask link to the parent row", async () => {
    const onParentKeyDown = vi.fn();
    render(
      <StateProvider
        initialState={{
          kanban: {
            workflowId: "wf-1",
            steps: [],
            tasks: [
              makeTask({ id: "parent-1" }),
              makeTask({ id: "child-1", title: "Child", parentTaskId: "parent-1" }),
            ],
          },
        }}
      >
        <div onKeyDown={onParentKeyDown}>
          <TaskTitleHoverCard taskId="parent-1" title="Parent">
            <span>Parent</span>
          </TaskTitleHoverCard>
        </div>
      </StateProvider>,
    );
    await openCard();

    const row = subtaskRow("child-1");
    expect(document.activeElement).toBe(row);
    row.focus();
    fireEvent.keyDown(row, { key: "Enter" });

    expect(onParentKeyDown).not.toHaveBeenCalled();
  });
});
