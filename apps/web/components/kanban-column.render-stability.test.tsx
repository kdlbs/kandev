import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const taskListRenderCounts = vi.hoisted(() => ({ count: 0 }));

vi.mock("@dnd-kit/core", () => ({
  useDroppable: () => ({ setNodeRef: vi.fn(), isOver: false }),
}));

vi.mock("./kanban/virtualized-column-task-list", () => ({
  VirtualizedColumnTaskList: () => {
    taskListRenderCounts.count += 1;
    return <div data-testid="task-list" />;
  },
}));

import { StateProvider } from "@/components/state-provider";
import type { Task } from "@/components/kanban-card";
import { defaultState } from "@/lib/state/default-state";
import { KanbanColumn, type WorkflowStep } from "@/components/kanban-column";

const STEP: WorkflowStep = { id: "step-1", title: "Todo", color: "bg-blue-500" };
const TASK_A: Task = { id: "task-a", title: "Task A", workflowStepId: STEP.id, position: 0 };
const TASK_B: Task = { id: "task-b", title: "Task B", workflowStepId: STEP.id, position: 1 };
const EXTERNAL_LINK_AVAILABILITY = { gitlab: false, jira: false, linear: false, sentry: false };
const HANDLERS = {
  onPreviewTask: vi.fn(),
  onOpenTask: vi.fn(),
  onEditTask: vi.fn(),
  onDeleteTask: vi.fn(),
};

function Column(props: {
  activeTaskId?: string | null;
  keyboardDraft?: {
    taskId: string;
    stepId: string;
    band: "admitted" | "queued";
    order: string[];
  } | null;
  onCardKeyDown?: (event: React.KeyboardEvent, task: Task) => void;
}) {
  return (
    <StateProvider
      initialState={{
        workspaces: { ...defaultState.workspaces, activeId: "workspace-1" },
        repositories: { ...defaultState.repositories, itemsByWorkspaceId: {} },
      }}
    >
      <KanbanColumn
        step={STEP}
        tasks={[TASK_A, TASK_B]}
        externalLinkAvailability={EXTERNAL_LINK_AVAILABILITY}
        {...HANDLERS}
        {...props}
      />
    </StateProvider>
  );
}

afterEach(() => {
  taskListRenderCounts.count = 0;
  cleanup();
});

describe("KanbanColumn memo comparator", () => {
  it("rerenders when keyboardDraft changes even though every other prop is unchanged", () => {
    const view = render(<Column keyboardDraft={null} />);
    expect(taskListRenderCounts.count).toBe(1);

    view.rerender(
      <Column
        keyboardDraft={{
          taskId: TASK_A.id,
          stepId: STEP.id,
          band: "admitted",
          order: [TASK_A.id, TASK_B.id],
        }}
      />,
    );

    expect(taskListRenderCounts.count).toBe(2);
  });

  it("rerenders when activeTaskId changes even though every other prop is unchanged", () => {
    const view = render(<Column activeTaskId={null} />);
    expect(taskListRenderCounts.count).toBe(1);

    view.rerender(<Column activeTaskId={TASK_A.id} />);

    expect(taskListRenderCounts.count).toBe(2);
  });

  it("rerenders when onCardKeyDown changes even though every other prop is unchanged", () => {
    const view = render(<Column onCardKeyDown={vi.fn()} />);
    expect(taskListRenderCounts.count).toBe(1);

    view.rerender(<Column onCardKeyDown={vi.fn()} />);

    expect(taskListRenderCounts.count).toBe(2);
  });
});
