import { useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { TaskMR } from "@/lib/types/gitlab";
import type { TaskPR } from "@/lib/types/github";
import { TaskCreateDependencies } from "./task-create-dialog-dependencies";

const workspaceHydrationMocks = vi.hoisted(() => ({
  useWorkspacePRs: vi.fn(),
  useWorkspaceMRs: vi.fn(),
}));

vi.mock("@/hooks/domains/github/use-task-pr", () => workspaceHydrationMocks);
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => workspaceHydrationMocks);

const WORKSPACE_ID = "workspace-1";

type MockStore = {
  kanban: { tasks: KanbanState["tasks"] };
  kanbanMulti: { snapshots: Record<string, { tasks?: KanbanState["tasks"] }> };
  workspaces: { activeId: string | null };
  workspaceContextGeneration: number;
  taskMRs: { byWorkspaceId: Record<string, Record<string, TaskMR[]>> };
  taskPRs: {
    byTaskId: Record<string, TaskPR[]>;
    workspaceId?: string | null;
    workspaceContextGeneration?: number;
  };
};

const ALPHA_ID = "task-alpha";
const BETA_ID = "task-beta";
const ALPHA_TITLE = "Alpha task";
const BETA_TITLE = "Beta task";
const NO_DEPENDENCY_LABEL = "No dependency";
const SEARCH_TASKS_LABEL = "Search tasks or #PR/MR number...";
const INFO_LABEL = "About task dependencies";
const TWO_DEPENDENCIES_LABEL = "2 dependencies";
const TRIGGER_TEST_ID = "task-create-dependencies-trigger";
const POPOVER_TEST_ID = "task-create-dependencies-popover";

function emptyStore(): MockStore {
  return {
    kanban: { tasks: [] },
    kanbanMulti: { snapshots: {} },
    workspaces: { activeId: WORKSPACE_ID },
    workspaceContextGeneration: 0,
    taskMRs: { byWorkspaceId: {} },
    taskPRs: {
      byTaskId: {},
      workspaceId: WORKSPACE_ID,
      workspaceContextGeneration: 0,
    },
  };
}

function taskPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-assoc-1",
    workspace_id: WORKSPACE_ID,
    task_id: ALPHA_ID,
    owner: "kdlbs",
    repo: "kandev",
    pr_number: 3295,
    pr_url: "https://github.com/kdlbs/kandev/pull/3295",
    pr_title: "fix flaky test",
    head_branch: "fix",
    base_branch: "main",
    author_login: "nova28",
    state: "merged",
    review_state: "",
    checks_state: "",
    mergeable_state: "mergeable",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    ...overrides,
  } as TaskPR;
}

let mockStore: MockStore = emptyStore();

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockStore) => unknown) => selector(mockStore),
}));

function task(
  id: string,
  title: string,
  overrides: Partial<KanbanState["tasks"][number]> = {},
): KanbanState["tasks"][number] {
  return {
    id,
    workflowId: "workflow-1",
    workflowStepId: "step-1",
    title,
    position: 0,
    isArchived: false,
    ...overrides,
  };
}

function renderDependencies(value: string[] = [], onChange = vi.fn()) {
  return render(
    <TooltipProvider>
      <TaskCreateDependencies value={value} onChange={onChange} />
    </TooltipProvider>,
  );
}

function ControlledDependencies({ initialValue = [] }: { initialValue?: string[] }) {
  const [value, setValue] = useState(initialValue);
  return <TaskCreateDependencies value={value} onChange={setValue} />;
}

function optionTestId(taskId: string): string {
  return `task-create-dependency-option-${taskId}`;
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  mockStore = emptyStore();
});

describe("TaskCreateDependencies", () => {
  it("hydrates change-request associations for the active workspace", () => {
    renderDependencies();

    expect(workspaceHydrationMocks.useWorkspacePRs).toHaveBeenCalledWith(WORKSPACE_ID);
    expect(workspaceHydrationMocks.useWorkspaceMRs).toHaveBeenCalledWith(WORKSPACE_ID);
  });

  it("starts with a no-dependency selector and its dependency icon", () => {
    renderDependencies();

    const trigger = screen.getByTestId(TRIGGER_TEST_ID);
    expect(trigger.textContent).toContain(NO_DEPENDENCY_LABEL);
    expect(trigger.querySelector("svg")).not.toBeNull();
  });

  it("shows searchable non-archived tasks with task icons and teaching help", async () => {
    mockStore = {
      ...emptyStore(),
      kanban: {
        tasks: [
          task(ALPHA_ID, ALPHA_TITLE),
          task("task-archived", "Archived task", { isArchived: true }),
        ],
      },
      kanbanMulti: {
        snapshots: {
          other: { tasks: [task(BETA_ID, BETA_TITLE)] },
        },
      },
    };

    renderDependencies();
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));

    const picker = screen.getByTestId(POPOVER_TEST_ID);
    expect(within(picker).getByPlaceholderText(SEARCH_TASKS_LABEL)).toBeTruthy();
    expect(within(picker).getByTestId("task-create-dependency-info")).toBeTruthy();
    expect(within(picker).getByTestId(optionTestId(ALPHA_ID))).toBeTruthy();
    expect(within(picker).getByTestId(optionTestId(BETA_ID))).toBeTruthy();
    expect(within(picker).queryByText("Archived task")).toBeNull();
    expect(
      within(within(picker).getByTestId(optionTestId(ALPHA_ID))).getByTestId(
        "task-create-dependency-task-icon",
      ),
    ).toBeTruthy();

    const search = within(picker).getByPlaceholderText(SEARCH_TASKS_LABEL);
    fireEvent.change(search, { target: { value: "Beta" } });
    expect(within(picker).getByText(BETA_TITLE)).toBeTruthy();
    expect(within(picker).queryByText(ALPHA_TITLE)).toBeNull();
    expect(within(picker).getByTestId("task-create-no-dependency")).toBeTruthy();

    const info = within(picker).getByTestId("task-create-dependency-info");
    expect(info.getAttribute("aria-label")).toBe(INFO_LABEL);
    fireEvent.focus(info);
    await waitFor(() => {
      expect(screen.getByRole("tooltip").textContent).toMatch(/waits until every selected task/i);
    });
  });

  it("toggles multiple predecessors and exposes a localized count", () => {
    mockStore = {
      ...emptyStore(),
      kanban: {
        tasks: [task(ALPHA_ID, ALPHA_TITLE), task(BETA_ID, BETA_TITLE)],
      },
    };

    render(
      <TooltipProvider>
        <ControlledDependencies />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByTestId("task-create-dependencies-trigger"));

    fireEvent.click(screen.getByTestId(optionTestId(ALPHA_ID)));
    expect(screen.getByTestId(TRIGGER_TEST_ID).textContent).toContain(ALPHA_TITLE);

    fireEvent.click(screen.getByTestId(optionTestId(BETA_ID)));
    expect(screen.getByTestId(TRIGGER_TEST_ID).textContent).toContain(TWO_DEPENDENCIES_LABEL);

    fireEvent.click(screen.getByTestId(optionTestId(ALPHA_ID)));
    expect(screen.getByTestId(TRIGGER_TEST_ID).textContent).toContain(BETA_TITLE);
  });

  it("clears all selected predecessors from the no-dependency entry", () => {
    mockStore = {
      ...emptyStore(),
      kanban: {
        tasks: [task(ALPHA_ID, ALPHA_TITLE), task(BETA_ID, BETA_TITLE)],
      },
    };

    render(
      <TooltipProvider>
        <ControlledDependencies initialValue={[ALPHA_ID, BETA_ID]} />
      </TooltipProvider>,
    );
    expect(screen.getByTestId(TRIGGER_TEST_ID).textContent).toContain(TWO_DEPENDENCIES_LABEL);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    fireEvent.click(screen.getByTestId("task-create-no-dependency"));

    expect(screen.getByTestId(TRIGGER_TEST_ID).textContent).toContain(NO_DEPENDENCY_LABEL);
  });
});

describe("TaskCreateDependencies change-request search", () => {
  it("narrows to the PR-linked task when searching by PR number and shows its badge", () => {
    const prTask = task(ALPHA_ID, ALPHA_TITLE, {
      statusSummary: { pull_request: { number: 3295, state: "merged" } } as never,
    });
    mockStore = {
      ...emptyStore(),
      kanban: {
        tasks: [prTask, task(BETA_ID, BETA_TITLE)],
      },
    };

    renderDependencies();
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));

    const picker = screen.getByTestId(POPOVER_TEST_ID);
    const alphaOption = within(picker).getByTestId(optionTestId(ALPHA_ID));
    expect(within(alphaOption).getByText("#3295")).toBeTruthy();

    const search = within(picker).getByPlaceholderText(SEARCH_TASKS_LABEL);
    fireEvent.change(search, { target: { value: "3295" } });
    expect(within(picker).getByTestId(optionTestId(ALPHA_ID))).toBeTruthy();
    expect(within(picker).queryByTestId(optionTestId(BETA_ID))).toBeNull();
  });

  it("narrows to the MR-linked task when searching with a leading #", () => {
    const mrTask = task(ALPHA_ID, ALPHA_TITLE);
    mockStore = {
      ...emptyStore(),
      kanban: { tasks: [mrTask, task(BETA_ID, BETA_TITLE)] },
      taskMRs: {
        byWorkspaceId: {
          [WORKSPACE_ID]: {
            [ALPHA_ID]: [
              {
                id: "mr-1",
                task_id: ALPHA_ID,
                host: "gitlab.com",
                project_path: "kdlbs/kandev",
                mr_iid: 42,
                mr_url: "https://gitlab.com/kdlbs/kandev/-/merge_requests/42",
                mr_title: "Fix",
                head_branch: "fix",
                base_branch: "main",
                author_username: "nova28",
                state: "open",
              } as TaskMR,
            ],
          },
        },
      },
    };

    renderDependencies();
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));

    const picker = screen.getByTestId(POPOVER_TEST_ID);
    const search = within(picker).getByPlaceholderText(SEARCH_TASKS_LABEL);
    fireEvent.change(search, { target: { value: "#42" } });
    expect(within(picker).getByTestId(optionTestId(ALPHA_ID))).toBeTruthy();
    expect(within(picker).queryByTestId(optionTestId(BETA_ID))).toBeNull();
  });

  it("lists unfiltered candidates most-recently-updated first", () => {
    mockStore = {
      ...emptyStore(),
      kanban: {
        tasks: [
          task(ALPHA_ID, ALPHA_TITLE, { updatedAt: "2026-01-01T00:00:00Z" }),
          task(BETA_ID, BETA_TITLE, { updatedAt: "2026-06-01T00:00:00Z" }),
        ],
      },
    };

    renderDependencies();
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));

    const picker = screen.getByTestId(POPOVER_TEST_ID);
    const options = within(picker)
      .getAllByTestId(/task-create-dependency-option-/)
      .map((option) => option.getAttribute("data-testid"));
    expect(options).toEqual([optionTestId(BETA_ID), optionTestId(ALPHA_ID)]);
  });
});

describe("TaskCreateDependencies multi-PR change requests", () => {
  it("shows a badge for every GitHub PR linked to a multi-repo task and finds it by any number", () => {
    const prTask = task(ALPHA_ID, ALPHA_TITLE, {
      statusSummary: { pull_request: { number: 1001, state: "open" } } as never,
    });
    mockStore = {
      ...emptyStore(),
      kanban: {
        tasks: [prTask, task(BETA_ID, BETA_TITLE)],
      },
      taskPRs: {
        byTaskId: {
          [ALPHA_ID]: [
            taskPR({ id: "pr-assoc-1", pr_number: 1001, repo: "kandev" }),
            taskPR({ id: "pr-assoc-2", pr_number: 1002, repo: "kandev-docs" }),
          ],
        },
        workspaceId: WORKSPACE_ID,
        workspaceContextGeneration: 0,
      },
    };

    renderDependencies();
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));

    const picker = screen.getByTestId(POPOVER_TEST_ID);
    const alphaOption = within(picker).getByTestId(optionTestId(ALPHA_ID));
    expect(within(alphaOption).getByText("#1001")).toBeTruthy();
    expect(within(alphaOption).getByText("#1002")).toBeTruthy();

    const search = within(picker).getByPlaceholderText(SEARCH_TASKS_LABEL);
    fireEvent.change(search, { target: { value: "1002" } });
    expect(within(picker).getByTestId(optionTestId(ALPHA_ID))).toBeTruthy();
    expect(within(picker).queryByTestId(optionTestId(BETA_ID))).toBeNull();
  });
});
