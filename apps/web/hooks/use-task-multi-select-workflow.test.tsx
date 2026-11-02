import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useMultiSelectDerived } from "@/components/kanban-board";
import { resolveBoardWorkflowId } from "@/lib/kanban/resolve-workflow";
import { bulkMoveSelectedTasks } from "@/lib/api";
import { useTaskMultiSelect } from "./use-task-multi-select";

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  bulkMoveSelectedTasks: vi.fn(),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));

const task = (id: string, workflowId: string) => ({
  id,
  title: id,
  workflowId,
  workflowStepId: `${workflowId}-start`,
  position: 0,
});
const snapshot = (workflowId: string) => ({
  workflowId,
  workflowName: workflowId,
  tasks: [task(`task-${workflowId}`, workflowId)],
  steps: [
    { id: `${workflowId}-start`, title: "Start", position: 0 },
    { id: `${workflowId}-done`, title: "Done", position: 1 },
  ],
});

function setup(hydratedWorkflowId: string | null) {
  const snapshots = { a: snapshot("a"), b: snapshot("b") };
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <StateProvider
      initialState={
        {
          kanban: {
            workflowId: hydratedWorkflowId,
            tasks: hydratedWorkflowId ? snapshots[hydratedWorkflowId as "a" | "b"].tasks : [],
            steps: hydratedWorkflowId ? snapshots[hydratedWorkflowId as "a" | "b"].steps : [],
          },
          kanbanMulti: { snapshots, isLoading: false },
        } as never
      }
    >
      {children}
    </StateProvider>
  );
  return renderHook(
    () => {
      const actualSnapshots = useAppStore((state) => state.kanbanMulti.snapshots);
      const board = useAppStore((state) => state.kanban);
      const store = useAppStoreApi();
      const effectiveWorkflowId = resolveBoardWorkflowId({
        isMobile: false,
        selectedWorkflowId: null,
        focusedWorkflowId: null,
        hydratedWorkflowId,
      });
      const selection = useTaskMultiSelect(effectiveWorkflowId);
      const derived = useMultiSelectDerived(selection.selectedIds, actualSnapshots, [], {}, board);
      return { selection, derived, store };
    },
    { wrapper },
  );
}

beforeEach(() => {
  vi.mocked(bulkMoveSelectedTasks)
    .mockReset()
    .mockResolvedValue({ moved_count: 1 } as never);
});

describe("board bulk step move workflow scope (AC-TASKS-BOARD-BULK-MOVE-SCOPE-001.1)", () => {
  it.each(["a", null])(
    "moves selected B using B when hydrated workflow is %s",
    async (hydrated) => {
      const { result } = setup(hydrated);
      act(() => result.current.selection.toggleSelect("task-b"));
      expect(result.current.derived.isMixedWorkflowSelection).toBe(false);
      expect(result.current.derived.multiSelectSteps.map((step) => step.id)).toContain("b-done");
      await act(async () => result.current.selection.bulkMove("b-done"));
      expect(bulkMoveSelectedTasks).toHaveBeenCalledExactlyOnceWith({
        task_ids: ["task-b"],
        target_workflow_id: "b",
        target_step_id: "b-done",
      });
    },
  );

  it("keeps the matching single-workflow move working", async () => {
    const { result } = setup("a");
    act(() => result.current.selection.toggleSelect("task-a"));
    await act(async () => result.current.selection.bulkMove("a-done"));
    expect(bulkMoveSelectedTasks).toHaveBeenCalledExactlyOnceWith({
      task_ids: ["task-a"],
      target_workflow_id: "a",
      target_step_id: "a-done",
    });
  });
});

describe("bulk move rejects invalid scope (AC-TASKS-BOARD-BULK-MOVE-SCOPE-001.2)", () => {
  it.each([
    ["mixed selection", "mixed"],
    ["unknown task", "unknown"],
    ["duplicate task across workflows", "duplicate"],
    ["conflicting task identity", "conflicting"],
    ["missing target", "missing-target"],
    ["target from another workflow", "wrong-target"],
    ["ambiguous target", "ambiguous-target"],
    ["hidden target", "hidden-target"],
  ])("rejects %s without sending or mutating", async (_label, scenario) => {
    const { result } = setup("a");
    act(() => {
      result.current.selection.toggleSelect("task-b");
      if (scenario === "mixed") result.current.selection.toggleSelect("task-a");
      if (scenario === "unknown") result.current.selection.toggleSelect("unknown");
      const state = result.current.store.getState();
      if (scenario === "duplicate") {
        result.current.store.setState({
          kanbanMulti: {
            ...state.kanbanMulti,
            snapshots: {
              ...state.kanbanMulti.snapshots,
              a: { ...state.kanbanMulti.snapshots.a, tasks: [task("task-b", "a")] },
            },
          },
        } as never);
      }
      if (scenario === "conflicting") {
        result.current.store.setState({
          kanbanMulti: {
            ...state.kanbanMulti,
            snapshots: {
              ...state.kanbanMulti.snapshots,
              b: { ...state.kanbanMulti.snapshots.b, tasks: [task("task-b", "a")] },
            },
          },
        } as never);
      }
      if (scenario === "ambiguous-target") {
        state.setWorkflowSnapshot("a", {
          ...state.kanbanMulti.snapshots.a,
          steps: [
            ...state.kanbanMulti.snapshots.a.steps,
            { id: "b-done", title: "Duplicate", position: 3, color: "" },
          ],
        } as never);
      }
      if (scenario === "hidden-target") {
        state.hydrate({
          userSettings: { ...state.userSettings, hiddenWorkflowStepIds: { b: ["b-done"] } },
        });
      }
    });
    let target = "b-done";
    if (scenario === "missing-target") target = "absent";
    if (scenario === "wrong-target") target = "a-done";
    const before = result.current.store
      .getState()
      .kanbanMulti.snapshots.b.tasks.map((t) => t.workflowStepId);
    await act(async () => result.current.selection.bulkMove(target));
    expect(bulkMoveSelectedTasks).not.toHaveBeenCalled();
    expect(
      result.current.store.getState().kanbanMulti.snapshots.b.tasks.map((t) => t.workflowStepId),
    ).toEqual(before);
    expect(result.current.selection.selectedIds.has("task-b")).toBe(true);
  });

  it("keeps selection and local task state on backend rejection", async () => {
    vi.mocked(bulkMoveSelectedTasks).mockRejectedValue(new Error("rejected"));
    const { result } = setup("a");
    act(() => result.current.selection.toggleSelect("task-b"));
    await act(async () => result.current.selection.bulkMove("b-done"));
    expect(bulkMoveSelectedTasks).toHaveBeenCalledOnce();
    expect(result.current.store.getState().kanbanMulti.snapshots.b.tasks[0].workflowStepId).toBe(
      "b-start",
    );
    expect(result.current.selection.selectedIds).toEqual(new Set(["task-b"]));
  });
});

it("moves using matching hydrated steps when the snapshot is a placeholder", async () => {
  const { result } = setup("a");
  act(() => {
    const state = result.current.store.getState();
    state.setWorkflowSnapshot("a", {
      ...state.kanbanMulti.snapshots.a,
      steps: [],
      isPlaceholder: true,
    });
    result.current.selection.toggleSelect("task-a");
  });
  expect(result.current.derived.multiSelectSteps.map((step) => step.id)).toContain("a-done");
  await act(async () => result.current.selection.bulkMove("a-done"));
  expect(bulkMoveSelectedTasks).toHaveBeenCalledExactlyOnceWith({
    task_ids: ["task-a"],
    target_workflow_id: "a",
    target_step_id: "a-done",
  });
});
