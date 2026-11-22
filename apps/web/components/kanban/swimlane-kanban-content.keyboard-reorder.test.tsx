import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";

const viewport = vi.hoisted(() => ({ hiddenTaskId: null as string | null }));
const reorderBand = vi.hoisted(() => vi.fn());
const externalLinks = vi.hoisted(() => ({
  gitlab: false,
  jira: false,
  linear: false,
  sentry: false,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isTablet: false }),
}));
vi.mock("@/components/kanban-external-link-availability", () => ({
  useKanbanExternalLinkAvailability: () => externalLinks,
}));
vi.mock("@/hooks/domains/kanban/use-step-reorder", () => ({
  useStepReorder: () => ({ reorderBand, isBandPending: () => false }),
}));
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({
    count,
    getItemKey,
  }: {
    count: number;
    getItemKey: (index: number) => string;
  }) => ({
    getTotalSize: () => count * 96,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        key: getItemKey(index),
        start: index * 96,
        size: 96,
        end: (index + 1) * 96,
      })).filter((row) => row.key !== viewport.hiddenTaskId),
    measureElement: () => undefined,
    resizeItem: () => undefined,
    measurementsCache: [],
    elementsCache: new Map(),
  }),
}));

import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { Task } from "@/components/kanban-card";
import { SwimlaneKanbanContent } from "./swimlane-kanban-content";

const PICKUP_ATTRIBUTE = "aria-grabbed";
const CARD_A_TEST_ID = "task-card-a";
const STEPS = [{ id: "step-1", title: "Backlog", color: "#888" }];
const TASKS: Task[] = ["a", "b", "c"].map((id, position) => ({
  id,
  title: `Task ${id}`,
  workflowStepId: "step-1",
  position,
  wipAdmitted: true,
}));
const noop = () => undefined;

function Board({ tasks }: { tasks: Task[] }) {
  return (
    <ToastProvider>
      <StateProvider>
        <TooltipProvider>
          <SwimlaneKanbanContent
            workflowId="workflow-1"
            steps={STEPS}
            moveTargetSteps={STEPS}
            tasks={tasks}
            onPreviewTask={noop}
            onOpenTask={noop}
            onEditTask={noop}
            onDeleteTask={noop}
          />
        </TooltipProvider>
      </StateProvider>
    </ToastProvider>
  );
}

function press(id: string, key: string) {
  fireEvent.keyDown(screen.getByTestId(`task-card-${id}`), { key });
}

function expectCommitted(id: string, order: string[]) {
  expect(reorderBand).toHaveBeenCalledExactlyOnceWith({
    workflowId: "workflow-1",
    stepId: "step-1",
    band: "admitted",
    draggedId: id,
    visibleOrderAfterMove: order,
  });
  expect(screen.getByTestId(`task-card-${id}`).getAttribute(PICKUP_ATTRIBUTE)).toBeNull();
}

beforeEach(() => {
  viewport.hiddenTaskId = null;
  reorderBand.mockReset();
});
afterEach(cleanup);

describe("Kanban keyboard gesture availability", () => {
  // @covers AC-TASKS-KANBAN-TASK-REORDERING-001.9, AC-TASKS-KANBAN-TASK-REORDERING-001.12
  it("releases a removed real card and reorders a remaining real card", () => {
    const view = render(<Board tasks={TASKS} />);
    press("a", " ");
    press("a", "ArrowDown");
    expect(screen.getByTestId(CARD_A_TEST_ID).getAttribute(PICKUP_ATTRIBUTE)).toBe("true");
    const cardB = screen.getByTestId("task-card-b");
    view.rerender(<Board tasks={TASKS.slice(1)} />);
    expect(screen.getByTestId("task-card-b")).toBe(cardB);
    expect(screen.queryByTestId(CARD_A_TEST_ID)).toBeNull();
    const staleIndicators = screen.queryAllByTestId(/kanban-insertion-indicator/).length;
    expect(reorderBand).not.toHaveBeenCalled();
    press("b", "Escape");
    press("b", " ");
    expect(cardB.getAttribute(PICKUP_ATTRIBUTE)).toBe("true");
    press("b", "ArrowDown");
    press("b", "Enter");
    expectCommitted("b", ["c", "b"]);
    expect(staleIndicators).toBe(0);
  });

  it("commits a normal real-card keyboard reorder", () => {
    render(<Board tasks={TASKS} />);
    press("a", " ");
    press("a", "ArrowDown");
    press("a", "Enter");
    expectCommitted("a", ["b", "a", "c"]);
  });

  it("allows another real card to reorder after explicit Escape", () => {
    render(<Board tasks={TASKS} />);
    press("a", " ");
    press("a", "Escape");
    expect(reorderBand).not.toHaveBeenCalled();
    press("b", " ");
    press("b", "ArrowDown");
    press("b", "Enter");
    expectCommitted("b", ["a", "c", "b"]);
  });

  it("retains a pickup when only its virtual row unmounts", () => {
    const view = render(<Board tasks={TASKS} />);
    press("a", " ");
    press("a", "ArrowDown");
    viewport.hiddenTaskId = "a";
    view.rerender(<Board tasks={[...TASKS]} />);
    expect(screen.queryByTestId(CARD_A_TEST_ID)).toBeNull();
    expect(reorderBand).not.toHaveBeenCalled();
    viewport.hiddenTaskId = null;
    view.rerender(<Board tasks={[...TASKS]} />);
    expect(screen.getByTestId(CARD_A_TEST_ID).getAttribute(PICKUP_ATTRIBUTE)).toBe("true");
    press("a", "Enter");
    expectCommitted("a", ["b", "a", "c"]);
  });
});
