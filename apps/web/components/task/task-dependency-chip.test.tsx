import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { TaskDependencyChip } from "./task-dependency-chip";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

afterEach(cleanup);

type KanbanTask = KanbanState["tasks"][number];

const task = (over: Partial<KanbanTask> & { id: string }): KanbanTask =>
  ({
    workflowStepId: "step-1",
    title: over.id.toUpperCase(),
    position: 0,
    ...over,
  }) as KanbanTask;

const CHIP_TESTID = "task-dependency-chip";

function renderChip(tasks: KanbanTask[], taskId: string) {
  return render(
    <StateProvider initialState={{ kanban: { workflowId: "wf-1", steps: [], tasks } } as never}>
      <TaskDependencyChip taskId={taskId} />
    </StateProvider>,
  );
}

describe("TaskDependencyChip", () => {
  it("renders nothing when the task has no edges in either direction", () => {
    renderChip([task({ id: "a" })], "a");
    expect(screen.queryByTestId(CHIP_TESTID)).toBeNull();
  });

  it("renders nothing for an unknown task", () => {
    renderChip([task({ id: "a" })], "missing");
    expect(screen.queryByTestId(CHIP_TESTID)).toBeNull();
  });

  it("renders when the task only has dependents (it blocks others but is not blocked)", () => {
    // The reverse direction alone must surface the chip: "two tasks are waiting
    // on this one" is exactly the context the chip exists to show.
    renderChip([task({ id: "a", blocks: [{ id: "b" }, { id: "c" }] })], "a");
    expect(screen.getByTestId(CHIP_TESTID)).toBeTruthy();
  });

  it("renders both directions with counts", () => {
    renderChip(
      [
        task({
          id: "b",
          blocked: true,
          blockedReason: "pending",
          dependsOn: [{ id: "a", title: "A", status: "pending" }],
          blocks: [{ id: "c", title: "C" }],
        }),
      ],
      "b",
    );
    const chip = screen.getByTestId(CHIP_TESTID);
    expect(chip.textContent).toMatch(/1/);
  });

  it("renders for a resolved dependency so the chain is still visible after it unblocks", () => {
    renderChip(
      [
        task({
          id: "b",
          blocked: false,
          dependsOn: [{ id: "a", title: "A", status: "resolved" }],
        }),
      ],
      "b",
    );
    expect(screen.getByTestId(CHIP_TESTID)).toBeTruthy();
  });
});
