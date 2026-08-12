import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { TaskPR } from "@/lib/types/github";
import { TaskTitleHoverCard } from "./task-title-hover-card";

afterEach(() => cleanup());

const LONG_TITLE = "A very long parent task title that should render in full";
const FIRST_SUBTASK_TITLE = "First subtask";

function makeTask(overrides: Partial<KanbanState["tasks"][number]>): KanbanState["tasks"][number] {
  return {
    id: "id",
    workflowStepId: "step-1",
    title: "Task",
    position: 0,
    ...overrides,
  };
}

function makePR(overrides: Pick<TaskPR, "id" | "task_id">): TaskPR {
  return {
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

function renderCard(tasks: KanbanState["tasks"], title = LONG_TITLE) {
  render(
    <StateProvider initialState={{ kanban: { workflowId: "wf-1", steps: [], tasks } }}>
      <TaskTitleHoverCard taskId="parent-1" title={title}>
        <span tabIndex={0} data-testid="trigger">
          {title}
        </span>
      </TaskTitleHoverCard>
    </StateProvider>,
  );
}

// Radix HoverCard schedules the open (even on focus) through the same
// openDelay timer, so the content mounts asynchronously — waiting via
// findBy lets real timers elapse rather than asserting synchronously.
async function openCard() {
  screen.getByTestId("trigger").focus();
  await screen.findAllByTestId("task-title-hover-card");
}

describe("TaskTitleHoverCard", () => {
  it("AC12: shows the full title in bold (font-semibold) when opened", async () => {
    renderCard([makeTask({ id: "parent-1" })]);
    await openCard();

    const card = screen.getAllByTestId("task-title-hover-card")[0];
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

    expect(screen.getAllByTestId("task-subtask-row-child-1")[0].textContent).toContain(
      FIRST_SUBTASK_TITLE,
    );
    expect(screen.getAllByTestId("task-subtask-row-child-2")[0].textContent).toContain(
      "Second subtask",
    );
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

    const row = screen.getAllByTestId("task-subtask-row-child-1")[0];
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

    const withPR = screen.getAllByTestId("task-subtask-row-child-1")[0];
    const withoutPR = screen.getAllByTestId("task-subtask-row-child-2")[0];
    expect(withPR.querySelector(".tabler-icon-git-pull-request")).not.toBeNull();
    expect(withoutPR.querySelector(".tabler-icon-git-pull-request")).toBeNull();
    expect(withoutPR.querySelector(".tabler-icon-git-merge")).toBeNull();
  });
});

describe("TaskTitleHoverCard — subtask cap and dismissal", () => {
  it("AC15: opens with only the bold title when there are no subtasks", async () => {
    renderCard([makeTask({ id: "parent-1" })]);
    await openCard();

    expect(screen.queryAllByTestId("task-title-hover-subtasks")).toHaveLength(0);
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
});
