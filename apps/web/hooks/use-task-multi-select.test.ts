import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskPriority } from "@/lib/types/http";
import type { DisplayOrderTask } from "@/lib/kanban/task-order";
import {
  multiSelectReducer,
  INITIAL_STATE,
  buildPipelineStepIndexOf,
  filterIdsByPriorityFilter,
} from "./use-task-multi-select";

describe("multiSelectReducer", () => {
  it("reset returns initial state", () => {
    const dirty = {
      selectedIds: new Set(["a", "b"]),
      isMultiSelectEnabled: true,
      isDeleting: true,
      isArchiving: false,
      anchorId: "a",
    };
    expect(multiSelectReducer(dirty, { type: "reset" })).toBe(INITIAL_STATE);
  });

  it("toggle_select adds a task", () => {
    const next = multiSelectReducer(INITIAL_STATE, { type: "toggle_select", taskId: "t1" });
    expect(next.selectedIds).toEqual(new Set(["t1"]));
  });

  it("toggle_select removes an already-selected task", () => {
    const state = { ...INITIAL_STATE, selectedIds: new Set(["t1", "t2"]) };
    const next = multiSelectReducer(state, { type: "toggle_select", taskId: "t1" });
    expect(next.selectedIds).toEqual(new Set(["t2"]));
  });

  it("set_selected replaces the selection set", () => {
    const state = { ...INITIAL_STATE, selectedIds: new Set(["old"]) };
    const next = multiSelectReducer(state, { type: "set_selected", ids: new Set(["a", "b"]) });
    expect(next.selectedIds).toEqual(new Set(["a", "b"]));
  });

  it("set_enabled controls isMultiSelectEnabled", () => {
    const on = multiSelectReducer(INITIAL_STATE, { type: "set_enabled", value: true });
    expect(on.isMultiSelectEnabled).toBe(true);
    const off = multiSelectReducer(on, { type: "set_enabled", value: false });
    expect(off.isMultiSelectEnabled).toBe(false);
  });

  describe("bulk operation state flags", () => {
    it("set_deleting toggles isDeleting", () => {
      const on = multiSelectReducer(INITIAL_STATE, { type: "set_deleting", value: true });
      expect(on.isDeleting).toBe(true);
      const off = multiSelectReducer(on, { type: "set_deleting", value: false });
      expect(off.isDeleting).toBe(false);
    });

    it("set_archiving toggles isArchiving", () => {
      const on = multiSelectReducer(INITIAL_STATE, { type: "set_archiving", value: true });
      expect(on.isArchiving).toBe(true);
      const off = multiSelectReducer(on, { type: "set_archiving", value: false });
      expect(off.isArchiving).toBe(false);
    });
  });

  describe("bulk action scenarios (reducer-level)", () => {
    it("all succeed: selectedIds empty + enabled false", () => {
      const state = {
        ...INITIAL_STATE,
        selectedIds: new Set(["t1", "t2"]),
        isMultiSelectEnabled: true,
      };
      // Simulate: set_selected with empty failed set, then set_enabled false
      const afterSelect = multiSelectReducer(state, {
        type: "set_selected",
        ids: new Set<string>(),
      });
      const afterDisable = multiSelectReducer(afterSelect, { type: "set_enabled", value: false });
      expect(afterDisable.selectedIds.size).toBe(0);
      expect(afterDisable.isMultiSelectEnabled).toBe(false);
    });

    it("some fail: selectedIds contains failed IDs, enabled stays true", () => {
      const state = {
        ...INITIAL_STATE,
        selectedIds: new Set(["t1", "t2", "t3"]),
        isMultiSelectEnabled: true,
      };
      // Simulate: set_selected with failed IDs only, no set_enabled call
      const afterSelect = multiSelectReducer(state, {
        type: "set_selected",
        ids: new Set(["t2"]),
      });
      expect(afterSelect.selectedIds).toEqual(new Set(["t2"]));
      expect(afterSelect.isMultiSelectEnabled).toBe(true);
    });

    it("all fail: selectedIds unchanged, enabled stays true", () => {
      const state = {
        ...INITIAL_STATE,
        selectedIds: new Set(["t1", "t2"]),
        isMultiSelectEnabled: true,
      };
      const afterSelect = multiSelectReducer(state, {
        type: "set_selected",
        ids: new Set(["t1", "t2"]),
      });
      expect(afterSelect.selectedIds).toEqual(new Set(["t1", "t2"]));
      expect(afterSelect.isMultiSelectEnabled).toBe(true);
    });
  });
});

describe("buildPipelineStepIndexOf", () => {
  const snapshots = {
    "wf-1": {
      steps: [
        { id: "todo", position: 0 },
        { id: "review", position: 1 },
        { id: "done", position: 2 },
      ],
    },
  };

  it("indexes steps by their displayed (position) order", () => {
    const indexOf = buildPipelineStepIndexOf(snapshots, {});
    expect(indexOf("todo")).toBe(0);
    expect(indexOf("review")).toBe(1);
    expect(indexOf("done")).toBe(2);
  });

  it("skips a hidden step when indexing the displayed order", () => {
    const indexOf = buildPipelineStepIndexOf(snapshots, { "wf-1": ["review"] });
    expect(indexOf("todo")).toBe(0);
    expect(indexOf("done")).toBe(1);
    expect(indexOf("review")).toBe(Infinity);
  });

  it("sorts an unknown or undefined step id last", () => {
    const indexOf = buildPipelineStepIndexOf(snapshots, {});
    expect(indexOf("unknown-step")).toBe(Infinity);
    expect(indexOf(undefined)).toBe(Infinity);
  });
});

describe("filterIdsByPriorityFilter", () => {
  // STORED_INVALID_ID is AC-001.10's persistent unranked origin (a stored value
  // outside the four tokens); "unranked" is the transient origin (absent
  // field). Both must be excluded identically under a non-empty selection.
  const STORED_INVALID_ID = "stored-invalid";
  const taskById = new Map<string, DisplayOrderTask>([
    ["high", { id: "high", priority: "high" }],
    ["low", { id: "low", priority: "low" }],
    ["unranked", { id: "unranked" }],
    [STORED_INVALID_ID, { id: STORED_INVALID_ID, priority: "urgent" as TaskPriority }],
  ]);

  it("keeps every id when the priority filter is empty", () => {
    expect(
      filterIdsByPriorityFilter(["high", "low", "unranked", STORED_INVALID_ID], taskById, []),
    ).toEqual(["high", "low", "unranked", STORED_INVALID_ID]);
  });

  it("drops an id whose task's priority is outside the active selection", () => {
    expect(filterIdsByPriorityFilter(["high", "low"], taskById, ["high"])).toEqual(["high"]);
  });

  it("drops an unranked task under a non-empty selection", () => {
    expect(filterIdsByPriorityFilter(["high", "unranked"], taskById, ["high"])).toEqual(["high"]);
  });

  it("drops a persistent-origin unranked task (stored value outside the vocabulary) under a non-empty selection", () => {
    expect(filterIdsByPriorityFilter(["high", STORED_INVALID_ID], taskById, ["high"])).toEqual([
      "high",
    ]);
  });

  it("keeps an id whose task isn't in the map — visibility can't be determined, so it isn't excluded", () => {
    expect(filterIdsByPriorityFilter(["ghost"], taskById, ["high"])).toEqual(["ghost"]);
  });
});

const moveTaskById = vi.fn();
const deleteTaskById = vi.fn();
const archiveTaskById = vi.fn();

vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({
    moveTaskById,
    deleteTaskById,
    archiveTaskById,
    renameTaskById: vi.fn(),
  }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

function makeTask(id: string, priority?: TaskPriority) {
  return {
    id,
    workflowId: "wf1",
    workflowStepId: "step1",
    title: id,
    position: 0,
    priority,
  };
}

function makeStoreApi(
  tasks: (ReturnType<typeof makeTask> & { createdAt?: string })[],
  priorityFilterTokens: TaskPriority[] = [],
  kanbanSort: "created_desc" | "priority_desc" = "created_desc",
) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let state: any = {
    kanban: { workflowId: "wf1", steps: [], tasks: [] },
    kanbanMulti: {
      snapshots: {
        wf1: {
          workflowId: "wf1",
          workflowName: "wf1",
          steps: [{ id: "step1", position: 0 }],
          tasks,
        },
      },
      isLoading: false,
    },
    userSettings: {
      kanbanSort,
      kanbanPriorityFilterTokens: priorityFilterTokens,
      kanbanViewMode: "kanban",
      hiddenWorkflowStepIds: {},
    },
  };
  state.hydrate = (patch: Partial<typeof state>) => {
    state = { ...state, ...patch };
  };
  state.setWorkflowSnapshot = (wfId: string, snapshot: unknown) => {
    state = {
      ...state,
      kanbanMulti: {
        ...state.kanbanMulti,
        snapshots: { ...state.kanbanMulti.snapshots, [wfId]: snapshot },
      },
    };
  };
  return { getState: () => state };
}

let storeApi: ReturnType<typeof makeStoreApi>;

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => storeApi,
}));

describe("useTaskMultiSelect — bulk actions exclude priority-filtered-out tasks (AC-002.11)", () => {
  beforeEach(() => {
    moveTaskById.mockReset().mockResolvedValue(undefined);
    deleteTaskById.mockReset().mockResolvedValue(undefined);
    archiveTaskById.mockReset().mockResolvedValue(undefined);
  });
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("bulkDelete skips a selected task the active priority filter hides, without clearing it from the selection", async () => {
    storeApi = makeStoreApi([makeTask("visible", "high"), makeTask("hidden", "low")], ["high"]);
    const { useTaskMultiSelect } = await import("./use-task-multi-select");
    const { result } = renderHook(() => useTaskMultiSelect("wf1"));

    act(() => result.current.toggleSelect("visible"));
    act(() => result.current.toggleSelect("hidden"));

    await act(async () => {
      await result.current.bulkDelete();
    });

    expect(deleteTaskById).toHaveBeenCalledTimes(1);
    expect(deleteTaskById).toHaveBeenCalledWith("visible", undefined);
    expect(result.current.selectedIds.has("hidden")).toBe(true);
  });

  it("bulkMove skips a filtered-out selected task and doesn't consume a position index for it", async () => {
    storeApi = makeStoreApi([makeTask("visible", "high"), makeTask("hidden", "low")], ["high"]);
    const { useTaskMultiSelect } = await import("./use-task-multi-select");
    const { result } = renderHook(() => useTaskMultiSelect("wf1"));

    act(() => result.current.toggleSelect("visible"));
    act(() => result.current.toggleSelect("hidden"));

    await act(async () => {
      await result.current.bulkMove("step2");
    });

    expect(moveTaskById).toHaveBeenCalledTimes(1);
    expect(moveTaskById).toHaveBeenCalledWith("visible", {
      workflow_id: "wf1",
      workflow_step_id: "step2",
      position: 0,
    });
  });

  it("bulkMove assigns positions in the live board sort order, not a fixed created-desc order (AC-002.10)", async () => {
    // Under created_desc these would land oldest-last; under priority_desc a
    // backward range selection must still land in priority-rank order.
    storeApi = makeStoreApi(
      [
        { ...makeTask("low", "low"), createdAt: "2026-01-03T00:00:00Z" },
        { ...makeTask("critical", "critical"), createdAt: "2026-01-01T00:00:00Z" },
        { ...makeTask("high", "high"), createdAt: "2026-01-02T00:00:00Z" },
      ],
      [],
      "priority_desc",
    );
    const { useTaskMultiSelect } = await import("./use-task-multi-select");
    const { result } = renderHook(() => useTaskMultiSelect("wf1"));

    // Select in reverse-of-priority order to prove the assigned index comes
    // from the sort token, not selection order.
    act(() => result.current.toggleSelect("low"));
    act(() => result.current.toggleSelect("critical"));
    act(() => result.current.toggleSelect("high"));

    await act(async () => {
      await result.current.bulkMove("step2");
    });

    expect(moveTaskById).toHaveBeenCalledTimes(3);
    expect(moveTaskById).toHaveBeenNthCalledWith(1, "critical", {
      workflow_id: "wf1",
      workflow_step_id: "step2",
      position: 0,
    });
    expect(moveTaskById).toHaveBeenNthCalledWith(2, "high", {
      workflow_id: "wf1",
      workflow_step_id: "step2",
      position: 1,
    });
    expect(moveTaskById).toHaveBeenNthCalledWith(3, "low", {
      workflow_id: "wf1",
      workflow_step_id: "step2",
      position: 2,
    });
  });

  it("acts on every selected task when the priority filter is empty", async () => {
    storeApi = makeStoreApi([makeTask("a", "high"), makeTask("b", "low")], []);
    const { useTaskMultiSelect } = await import("./use-task-multi-select");
    const { result } = renderHook(() => useTaskMultiSelect("wf1"));

    act(() => result.current.toggleSelect("a"));
    act(() => result.current.toggleSelect("b"));

    await act(async () => {
      await result.current.bulkArchive();
    });

    expect(archiveTaskById).toHaveBeenCalledTimes(2);
  });
});

// eligibleSelectedIds (lib.ts:176-187) reads the filter once, synchronously,
// before runBulkAction's only await point (Promise.allSettled) — so every
// `per(id)` dispatch below already happened by the time a test can mutate
// the store. These three tests pin that "snapshot at invocation, not at
// completion" contract for the in-flight window between dispatch and settle.
describe("useTaskMultiSelect — eligibility snapshot is frozen at invocation (AC-002.11)", () => {
  beforeEach(() => {
    moveTaskById.mockReset().mockResolvedValue(undefined);
    deleteTaskById.mockReset().mockResolvedValue(undefined);
    archiveTaskById.mockReset().mockResolvedValue(undefined);
  });
  afterEach(() => {
    vi.clearAllMocks();
  });

  function deferred<T>() {
    let resolve!: (value: T) => void;
    const promise = new Promise<T>((r) => {
      resolve = r;
    });
    return { promise, resolve };
  }

  it("a task hidden by a filter change after invocation still gets acted on", async () => {
    storeApi = makeStoreApi([makeTask("a", "high"), makeTask("b", "low")], []);
    const { useTaskMultiSelect } = await import("./use-task-multi-select");
    const { result } = renderHook(() => useTaskMultiSelect("wf1"));

    act(() => result.current.toggleSelect("a"));
    act(() => result.current.toggleSelect("b"));

    const gates = { a: deferred<void>(), b: deferred<void>() };
    deleteTaskById.mockImplementation((id: string) => gates[id as "a" | "b"].promise);

    let bulkDeletePromise: Promise<void> | undefined;
    act(() => {
      bulkDeletePromise = result.current.bulkDelete();
    });

    // Both dispatches already fired synchronously above; hiding "b" now must
    // not undo the dispatch already made against the pre-mutation snapshot.
    storeApi.getState().userSettings.kanbanPriorityFilterTokens = ["high"];

    await act(async () => {
      gates.a.resolve();
      gates.b.resolve();
      await bulkDeletePromise;
    });

    expect(deleteTaskById).toHaveBeenCalledTimes(2);
    expect(deleteTaskById).toHaveBeenCalledWith("a", undefined);
    expect(deleteTaskById).toHaveBeenCalledWith("b", undefined);
  });

  it("a task revealed after invocation isn't added to the acted-on set", async () => {
    storeApi = makeStoreApi([makeTask("a", "high"), makeTask("c", "low")], ["high"]);
    const { useTaskMultiSelect } = await import("./use-task-multi-select");
    const { result } = renderHook(() => useTaskMultiSelect("wf1"));

    act(() => result.current.toggleSelect("a"));
    act(() => result.current.toggleSelect("c"));

    const gate = deferred<void>();
    deleteTaskById.mockImplementation(() => gate.promise);

    let bulkDeletePromise: Promise<void> | undefined;
    act(() => {
      bulkDeletePromise = result.current.bulkDelete();
    });

    // "c" was excluded from the invocation-time snapshot; revealing it now
    // must not retroactively fold it into the dispatch already in flight.
    storeApi.getState().userSettings.kanbanPriorityFilterTokens = [];

    await act(async () => {
      gate.resolve();
      await bulkDeletePromise;
    });

    expect(deleteTaskById).toHaveBeenCalledTimes(1);
    expect(deleteTaskById).toHaveBeenCalledWith("a", undefined);
    expect(result.current.selectedIds.has("c")).toBe(true);
  });

  it("an invocation whose eligible snapshot is empty is a no-op", async () => {
    storeApi = makeStoreApi([makeTask("c", "low")], ["high"]);
    const { useTaskMultiSelect } = await import("./use-task-multi-select");
    const { result } = renderHook(() => useTaskMultiSelect("wf1"));

    act(() => result.current.toggleSelect("c"));

    await act(async () => {
      await result.current.bulkDelete();
    });

    expect(deleteTaskById).not.toHaveBeenCalled();
    expect(result.current.selectedIds.has("c")).toBe(true);
    expect(result.current.isProcessing).toBe(false);
  });
});
