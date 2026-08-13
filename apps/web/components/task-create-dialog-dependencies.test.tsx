import { useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { TaskCreateDependencies } from "./task-create-dialog-dependencies";

type MockStore = {
  kanban: { tasks: KanbanState["tasks"] };
  kanbanMulti: { snapshots: Record<string, { tasks?: KanbanState["tasks"] }> };
};

const ALPHA_ID = "task-alpha";
const BETA_ID = "task-beta";
const ALPHA_TITLE = "Alpha task";
const BETA_TITLE = "Beta task";
const NO_DEPENDENCY_LABEL = "No dependency";
const SEARCH_TASKS_LABEL = "Search tasks...";
const INFO_LABEL = "About task dependencies";
const TWO_DEPENDENCIES_LABEL = "2 dependencies";
const TRIGGER_TEST_ID = "task-create-dependencies-trigger";

let mockStore: MockStore = {
  kanban: { tasks: [] },
  kanbanMulti: { snapshots: {} },
};

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

afterEach(() => {
  cleanup();
  mockStore = {
    kanban: { tasks: [] },
    kanbanMulti: { snapshots: {} },
  };
});

describe("TaskCreateDependencies", () => {
  it("starts with a no-dependency selector and its dependency icon", () => {
    renderDependencies();

    const trigger = screen.getByTestId(TRIGGER_TEST_ID);
    expect(trigger.textContent).toContain(NO_DEPENDENCY_LABEL);
    expect(trigger.querySelector("svg")).not.toBeNull();
  });

  it("shows searchable non-archived tasks with task icons and teaching help", async () => {
    mockStore = {
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

    const picker = screen.getByTestId("task-create-dependencies-popover");
    expect(within(picker).getByPlaceholderText(SEARCH_TASKS_LABEL)).toBeTruthy();
    expect(within(picker).getByTestId("task-create-dependency-info")).toBeTruthy();
    expect(within(picker).getByTestId(`task-create-dependency-option-${ALPHA_ID}`)).toBeTruthy();
    expect(within(picker).getByTestId(`task-create-dependency-option-${BETA_ID}`)).toBeTruthy();
    expect(within(picker).queryByText("Archived task")).toBeNull();
    expect(
      within(within(picker).getByTestId(`task-create-dependency-option-${ALPHA_ID}`)).getByTestId(
        "task-create-dependency-task-icon",
      ),
    ).toBeTruthy();

    const search = within(picker).getByPlaceholderText(SEARCH_TASKS_LABEL);
    fireEvent.change(search, { target: { value: "Beta" } });
    expect(within(picker).getByText(BETA_TITLE)).toBeTruthy();
    expect(within(picker).queryByText(ALPHA_TITLE)).toBeNull();

    const info = within(picker).getByTestId("task-create-dependency-info");
    expect(info.getAttribute("aria-label")).toBe(INFO_LABEL);
    fireEvent.focus(info);
    await waitFor(() => {
      expect(screen.getByRole("tooltip").textContent).toMatch(/waits until every selected task/i);
    });
  });

  it("toggles multiple predecessors and exposes a localized count", () => {
    mockStore = {
      kanban: {
        tasks: [task(ALPHA_ID, ALPHA_TITLE), task(BETA_ID, BETA_TITLE)],
      },
      kanbanMulti: { snapshots: {} },
    };

    render(
      <TooltipProvider>
        <ControlledDependencies />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByTestId("task-create-dependencies-trigger"));

    fireEvent.click(screen.getByTestId(`task-create-dependency-option-${ALPHA_ID}`));
    expect(screen.getByTestId(TRIGGER_TEST_ID).textContent).toContain(ALPHA_TITLE);

    fireEvent.click(screen.getByTestId(`task-create-dependency-option-${BETA_ID}`));
    expect(screen.getByTestId(TRIGGER_TEST_ID).textContent).toContain(TWO_DEPENDENCIES_LABEL);

    fireEvent.click(screen.getByTestId(`task-create-dependency-option-${ALPHA_ID}`));
    expect(screen.getByTestId(TRIGGER_TEST_ID).textContent).toContain(BETA_TITLE);
  });

  it("clears all selected predecessors from the no-dependency entry", () => {
    mockStore = {
      kanban: {
        tasks: [task(ALPHA_ID, ALPHA_TITLE), task(BETA_ID, BETA_TITLE)],
      },
      kanbanMulti: { snapshots: {} },
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
