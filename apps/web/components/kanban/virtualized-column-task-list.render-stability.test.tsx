import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const cardRenderCounts = vi.hoisted(() => new Map<string, number>());

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 100,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({ index, start: index * 100 })),
    measureElement: vi.fn(),
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

function TaskList({ tasks }: { tasks: Task[] }) {
  return (
    <VirtualizedColumnTaskList
      orderedTasks={tasks}
      queuedStartIndex={tasks.length}
      queuedCount={0}
      step={STEP}
      steps={STEPS}
      presentation="desktop"
      workspaceId="workspace-1"
      repositories={REPOSITORIES}
      externalLinkAvailability={{ ...EXTERNAL_LINK_AVAILABILITY }}
      {...HANDLERS}
    />
  );
}

beforeEach(() => cardRenderCounts.clear());
afterEach(cleanup);

describe("VirtualizedColumnTaskList card render isolation", () => {
  it("does not rerender an unchanged card after a sibling task updates", () => {
    const view = render(<TaskList tasks={[TASK_A, TASK_B]} />);

    view.rerender(<TaskList tasks={[{ ...TASK_A, title: "Updated Task A" }, TASK_B]} />);

    expect({
      affectedCard: cardRenderCounts.get(TASK_A.id),
      unaffectedCard: cardRenderCounts.get(TASK_B.id),
    }).toEqual({ affectedCard: 2, unaffectedCard: 1 });
  });
});
