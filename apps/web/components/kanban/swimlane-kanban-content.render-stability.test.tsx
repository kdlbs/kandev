import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const columnRenderCounts = vi.hoisted(() => new Map<string, number>());
const responsive = vi.hoisted(() => ({ isMobile: false, isTablet: false }));
const externalLinkAvailability = vi.hoisted(() => ({
  gitlab: false,
  jira: false,
  linear: false,
  sentry: false,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

vi.mock("embla-carousel-react", () => ({
  default: () => [vi.fn(), null],
}));

vi.mock("@/components/kanban-external-link-availability", () => ({
  useKanbanExternalLinkAvailability: () => externalLinkAvailability,
}));

vi.mock("@dnd-kit/core", () => ({
  PointerSensor: function PointerSensor() {},
  TouchSensor: function TouchSensor() {},
  useSensor: () => ({}),
  useSensors: (...sensors: unknown[]) => sensors,
  useDroppable: () => ({ setNodeRef: vi.fn(), isOver: false }),
}));

vi.mock("./kanban-drag-surface", () => ({
  KanbanDragSurface: ({ layoutContent }: { layoutContent: React.ReactNode }) => layoutContent,
}));

vi.mock("./virtualized-column-task-list", () => ({
  VirtualizedColumnTaskList: ({ step }: { step: { id: string } }) => {
    columnRenderCounts.set(step.id, (columnRenderCounts.get(step.id) ?? 0) + 1);
    return <div data-testid={`task-list-${step.id}`} />;
  },
}));

import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import { defaultState } from "@/lib/state/default-state";
import { SwimlaneKanbanContent } from "./swimlane-kanban-content";

const WORKFLOW_ID = "workflow-1";
const STEPS: WorkflowStep[] = [
  { id: "step-a", title: "Step A", color: "bg-blue-500" },
  { id: "step-b", title: "Step B", color: "bg-green-500" },
];
const TASK_A: Task = {
  id: "task-a",
  title: "Task A",
  workflowStepId: "step-a",
  position: 0,
};
const TASK_B: Task = {
  id: "task-b",
  title: "Task B",
  workflowStepId: "step-b",
  position: 0,
};
const HANDLERS = {
  onPreviewTask: vi.fn(),
  onOpenTask: vi.fn(),
  onEditTask: vi.fn(),
  onDeleteTask: vi.fn(),
};

function Content({ tasks }: { tasks: Task[] }) {
  return (
    <ToastProvider>
      <StateProvider
        initialState={{
          workspaces: { ...defaultState.workspaces, activeId: "workspace-1" },
          repositories: { ...defaultState.repositories, itemsByWorkspaceId: {} },
          mobileKanban: {
            ...defaultState.mobileKanban,
            activeStepIdByWorkflowId: { [WORKFLOW_ID]: STEPS[0].id },
          },
        }}
      >
        <SwimlaneKanbanContent
          workflowId={WORKFLOW_ID}
          steps={STEPS}
          moveTargetSteps={STEPS}
          tasks={tasks}
          {...HANDLERS}
        />
      </StateProvider>
    </ToastProvider>
  );
}

beforeEach(() => {
  columnRenderCounts.clear();
  responsive.isMobile = false;
  responsive.isTablet = false;
});
afterEach(cleanup);

describe("SwimlaneKanbanContent column render isolation", () => {
  it("does not rerender an unaffected column after a task in another column updates", () => {
    const view = render(<Content tasks={[TASK_A, TASK_B]} />);

    view.rerender(<Content tasks={[{ ...TASK_A, title: "Updated Task A" }, TASK_B]} />);

    expect({
      affectedColumn: columnRenderCounts.get("step-a"),
      unaffectedColumn: columnRenderCounts.get("step-b"),
    }).toEqual({ affectedColumn: 2, unaffectedColumn: 1 });
  });

  it("keeps the same isolation in the mobile swipeable-column composition", () => {
    responsive.isMobile = true;
    const view = render(<Content tasks={[TASK_A, TASK_B]} />);

    view.rerender(<Content tasks={[{ ...TASK_A, title: "Updated Task A" }, TASK_B]} />);

    expect({
      affectedColumn: columnRenderCounts.get("step-a"),
      unaffectedColumn: columnRenderCounts.get("step-b"),
    }).toEqual({ affectedColumn: 2, unaffectedColumn: 1 });
  });
});
