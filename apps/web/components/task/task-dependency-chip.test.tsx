import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const getTaskDependenciesMock = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/task-dependencies-api", () => ({
  getTaskDependencies: getTaskDependenciesMock,
}));
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { TaskDependencyChip } from "./task-dependency-chip";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

beforeEach(() => {
  getTaskDependenciesMock.mockReset();
  // Default: the server also reports no edges, so the fetch fallback cannot be
  // what makes a "renders nothing" assertion pass.
  getTaskDependenciesMock.mockResolvedValue({ id: "unknown", depends_on: [], blocks: [] });
});

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

  it("drops fetched data as soon as the store can answer 'no edges'", async () => {
    // Regression: `fromStore ?? fetched` fell through on the store's `null`,
    // which is an explicit "this task has no edges" answer. Switching from a
    // task resolved by fetch to one the store knows is edge-free then kept
    // rendering the previous task's dependencies.
    getTaskDependenciesMock.mockResolvedValue({
      id: "fetched-task",
      blocked: true,
      depends_on: [{ id: "p", title: "P", status: "pending" }],
      blocks: [],
    });
    // Explicit empty lists = the store's real "no edges" answer. A task with no
    // projection at all is "unknown", which correctly falls through to a fetch.
    const store = [task({ id: "a", blocked: false, dependsOn: [], blocks: [] })];
    const { rerender } = renderChip(store, "fetched-task");
    await screen.findByTestId(CHIP_TESTID);

    rerender(
      <StateProvider
        initialState={{ kanban: { workflowId: "wf-1", steps: [], tasks: store } } as never}
      >
        <TaskDependencyChip taskId="a" />
      </StateProvider>,
    );
    await waitFor(() => expect(screen.queryByTestId(CHIP_TESTID)).toBeNull());
  });
});
