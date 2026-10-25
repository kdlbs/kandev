import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import { SwimlaneGraph2Content } from "./swimlane-graph2-content";

afterEach(() => {
  cleanup();
});

const moveTaskMock = vi.fn();

vi.mock("@/hooks/domains/kanban/use-swimlane-move", () => ({
  useSwimlaneMove: () => ({ moveTask: moveTaskMock }),
}));

const IN_PROGRESS_TITLE = "In Progress";

const STEPS: WorkflowStep[] = [
  { id: "step-1", title: "Triage", color: "#888" },
  { id: "step-2", title: IN_PROGRESS_TITLE, color: "#888" },
  { id: "step-3", title: "Done", color: "#888" },
];

function makeTask(id: string, title: string): Task {
  return { id, title, workflowStepId: "step-2" } as Task;
}

function renderList(tasks: Task[]) {
  return render(
    <StateProvider>
      <ToastProvider>
        <TooltipProvider delayDuration={0}>
          <SwimlaneGraph2Content
            workflowId="wf-1"
            steps={STEPS}
            moveTargetSteps={STEPS}
            tasks={tasks}
            onPreviewTask={() => undefined}
            onOpenTask={() => undefined}
            onEditTask={() => undefined}
            onDeleteTask={() => undefined}
            onArchiveTask={() => undefined}
          />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

function clickNextMoveButton(row: HTMLElement) {
  fireEvent.mouseEnter(within(row).getByRole("button", { name: IN_PROGRESS_TITLE }).parentElement!);
  fireEvent.click(within(row).getByRole("button", { name: "Move to Done" }));
}

describe("SwimlaneGraph2Content — the in-flight move guard is per task, not list-wide (AC-UI-PIPELINE-ROW-005.2)", () => {
  afterEach(() => {
    moveTaskMock.mockReset();
  });

  it("keeps task A's move controls disabled while A's own move is in flight, even after task B starts a separate move", async () => {
    const resolvers: Record<string, () => void> = {};
    moveTaskMock.mockImplementation(
      (task: Task) =>
        new Promise<void>((resolve) => {
          resolvers[task.id] = resolve;
        }),
    );

    const taskA = makeTask("task-A", "Task A");
    const taskB = makeTask("task-B", "Task B");
    renderList([taskA, taskB]);

    const rowA = screen.getByTestId("pipeline-task-task-A");
    const rowB = screen.getByTestId("pipeline-task-task-B");

    clickNextMoveButton(rowA);
    await waitFor(() => expect(moveTaskMock).toHaveBeenCalledWith(taskA, "step-3"));
    fireEvent.mouseEnter(
      within(rowA).getByRole("button", { name: IN_PROGRESS_TITLE }).parentElement!,
    );
    await waitFor(() =>
      expect(
        within(rowA).getByRole("button", { name: "Move to Done" }).hasAttribute("disabled"),
      ).toBe(true),
    );

    clickNextMoveButton(rowB);
    await waitFor(() => expect(moveTaskMock).toHaveBeenCalledWith(taskB, "step-3"));

    // Task A's own move has not settled: its controls must stay disabled
    // regardless of task B's independent, still-in-flight move starting.
    fireEvent.mouseEnter(
      within(rowA).getByRole("button", { name: IN_PROGRESS_TITLE }).parentElement!,
    );
    expect(
      within(rowA).getByRole("button", { name: "Move to Done" }).hasAttribute("disabled"),
    ).toBe(true);

    resolvers["task-A"]?.();
    resolvers["task-B"]?.();
  });
});
