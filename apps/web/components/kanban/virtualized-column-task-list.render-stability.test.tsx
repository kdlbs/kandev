import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const cardRenderCounts = vi.hoisted(() => new Map<string, number>());
const virtualizerState = vi.hoisted(() => ({
  measurementsCache: [] as Array<{ index: number; key: string; size: number }>,
  elementsCache: new Map<string, HTMLDivElement>(),
  measureElement: vi.fn((node: HTMLDivElement | null) => {
    if (!node) return;
    if (node.dataset.taskId === "task-a") {
      virtualizerState.measurementsCache[0]!.size = 180;
    }
  }),
}));

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 100,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        key: `task-${index}`,
        size: 100,
        start: index * 100,
      })),
    measurementsCache: virtualizerState.measurementsCache,
    elementsCache: virtualizerState.elementsCache,
    measureElement: virtualizerState.measureElement,
    resizeItem: vi.fn(),
    measure: vi.fn(),
  }),
}));

vi.mock("../kanban-card", () => ({
  KanbanCard: ({ task }: { task: { id: string } }) => {
    cardRenderCounts.set(task.id, (cardRenderCounts.get(task.id) ?? 0) + 1);
    return <div data-testid={`card-${task.id}`} />;
  },
  resolveTaskRepositoryChips: () => [],
}));

import type { Task } from "../kanban-card";
import type { WorkflowStep } from "../kanban-column";
import type { Repository } from "@/lib/types/http";
import { VirtualizedColumnTaskList } from "./virtualized-column-task-list";

const STEP: WorkflowStep = { id: "step-1", title: "Todo", color: "bg-blue-500" };
const STEPS = [STEP];
const TASK_A: Task = {
  id: "task-a",
  title: "Task A",
  workflowStepId: STEP.id,
  position: 0,
};
const TASK_B: Task = {
  id: "task-b",
  title: "Task B",
  workflowStepId: STEP.id,
  position: 1,
};
const HANDLERS = {
  onPreviewTask: vi.fn(),
  onOpenTask: vi.fn(),
  onEditTask: vi.fn(),
  onDeleteTask: vi.fn(),
};
const EXTERNAL_LINK_AVAILABILITY = { jira: false, linear: false, sentry: false };
const REPOSITORIES: Repository[] = [];

function TaskList({
  tasks,
  deletingTaskId,
  archivingTaskId,
  onContentHeightChange,
}: {
  tasks: Task[];
  deletingTaskId?: string;
  archivingTaskId?: string;
  onContentHeightChange?: (height: number, element: HTMLDivElement) => void;
}) {
  return (
    <VirtualizedColumnTaskList
      onContentHeightChange={onContentHeightChange}
      orderedTasks={tasks}
      queuedStartIndex={tasks.length}
      queuedCount={0}
      step={STEP}
      steps={STEPS}
      presentation="desktop"
      workspaceId="workspace-1"
      repositories={REPOSITORIES}
      externalLinkAvailability={{ ...EXTERNAL_LINK_AVAILABILITY }}
      deletingTaskId={deletingTaskId}
      archivingTaskId={archivingTaskId}
      {...HANDLERS}
    />
  );
}

beforeEach(() => {
  cardRenderCounts.clear();
  virtualizerState.measurementsCache = [];
  virtualizerState.elementsCache.clear();
  virtualizerState.measureElement.mockClear();
});
afterEach(cleanup);

describe("VirtualizedColumnTaskList card render isolation", () => {
  it("reports only the first six logical rows for compact lane sizing", () => {
    const report = vi.fn();
    const tasks = Array.from({ length: 8 }, (_, index) => ({
      ...TASK_A,
      id: `task-${index}`,
      title: `Task ${index}`,
      position: index,
    }));

    render(<TaskList tasks={tasks} onContentHeightChange={report} />);

    expect(report).toHaveBeenLastCalledWith(600, expect.any(HTMLDivElement));
  });

  it("invalidates an offscreen prefix row and remeasures it when it returns", () => {
    virtualizerState.measurementsCache = [
      { index: 0, key: TASK_A.id, size: 300 },
      { index: 1, key: TASK_B.id, size: 100 },
    ];
    const report = vi.fn();
    const view = render(<TaskList tasks={[TASK_A, TASK_B]} onContentHeightChange={report} />);

    expect(report).toHaveBeenLastCalledWith(400, expect.any(HTMLDivElement));

    view.rerender(
      <TaskList
        tasks={[{ ...TASK_A, title: "Updated prefix task" }, TASK_B]}
        onContentHeightChange={report}
      />,
    );

    expect(report).toHaveBeenLastCalledWith(192, expect.any(HTMLDivElement));

    const returnedRow = document.createElement("div");
    returnedRow.dataset.taskId = TASK_A.id;
    virtualizerState.elementsCache.set(TASK_A.id, returnedRow);
    view.rerender(
      <TaskList
        tasks={[{ ...TASK_A, title: "Updated prefix task" }, TASK_B]}
        onContentHeightChange={report}
      />,
    );

    expect(report).toHaveBeenLastCalledWith(276, expect.any(HTMLDivElement));
  });

  it("does not rerender an unchanged card after a sibling task updates", () => {
    const view = render(<TaskList tasks={[TASK_A, TASK_B]} />);

    view.rerender(<TaskList tasks={[{ ...TASK_A, title: "Updated Task A" }, TASK_B]} />);

    expect({
      affectedCard: cardRenderCounts.get(TASK_A.id),
      unaffectedCard: cardRenderCounts.get(TASK_B.id),
    }).toEqual({ affectedCard: 2, unaffectedCard: 1 });
  });

  it.each(["deletingTaskId", "archivingTaskId"] as const)(
    "does not rerender sibling cards when %s targets another card",
    (busyProp) => {
      const view = render(<TaskList tasks={[TASK_A, TASK_B]} />);

      view.rerender(
        <TaskList
          tasks={[TASK_A, TASK_B]}
          deletingTaskId={busyProp === "deletingTaskId" ? TASK_A.id : undefined}
          archivingTaskId={busyProp === "archivingTaskId" ? TASK_A.id : undefined}
        />,
      );

      expect({
        affectedCard: cardRenderCounts.get(TASK_A.id),
        unaffectedCard: cardRenderCounts.get(TASK_B.id),
      }).toEqual({ affectedCard: 2, unaffectedCard: 1 });
    },
  );
});
