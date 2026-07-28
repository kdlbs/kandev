import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices";
import type { ForegroundActivity } from "@/lib/types/http";
import { useTaskInFlight } from "./use-task-in-flight";

type Task = KanbanState["tasks"][number];

function task(id: string, foregroundActivity?: ForegroundActivity | null): Task {
  return {
    id,
    workflowStepId: "step-1",
    title: id,
    position: 0,
    foregroundActivity: foregroundActivity ?? undefined,
  };
}

// Seed the ACTIVE workflow's `kanban.tasks` — the primary lookup path, mirroring
// how the board card reads the same aggregate.
function wrapper(tasks: Task[] = []) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider initialState={{ kanban: { workflowId: "wf-1", steps: [], tasks } }}>
        {children}
      </StateProvider>
    );
  };
}

// Seed a cross-workflow snapshot (swimlane / PR-review board), the fallback the
// dialog must also resolve so the guard works in every board context.
function snapshotWrapper(tasks: Task[]) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider
        initialState={{
          kanbanMulti: {
            snapshots: {
              "wf-1": { workflowId: "wf-1", workflowName: "wf", steps: [], tasks },
            },
            isLoading: false,
          },
        }}
      >
        {children}
      </StateProvider>
    );
  };
}

describe("useTaskInFlight", () => {
  it("returns false when no task id is supplied", () => {
    const { result } = renderHook(() => useTaskInFlight(), { wrapper: wrapper() });
    expect(result.current).toBe(false);
  });

  it("returns false when the task is idle (no foreground activity)", () => {
    const { result } = renderHook(() => useTaskInFlight("t-1"), {
      wrapper: wrapper([task("t-1", null)]),
    });
    expect(result.current).toBe(false);
  });

  it("returns true when the task is generating", () => {
    const { result } = renderHook(() => useTaskInFlight("t-1"), {
      wrapper: wrapper([task("t-1", "generating")]),
    });
    expect(result.current).toBe(true);
  });

  it("returns true when spawned background work is running", () => {
    const { result } = renderHook(() => useTaskInFlight("t-1"), {
      wrapper: wrapper([task("t-1", "background")]),
    });
    expect(result.current).toBe(true);
  });

  it("skips in-flight lookup while the consuming surface is hidden", () => {
    const { result } = renderHook(() => useTaskInFlight("t-1", undefined, false), {
      wrapper: wrapper([task("t-1", "background")]),
    });
    expect(result.current).toBe(false);
  });

  it("resolves tasks from cross-workflow snapshots too", () => {
    const { result } = renderHook(() => useTaskInFlight("t-9"), {
      wrapper: snapshotWrapper([task("t-9", "background")]),
    });
    expect(result.current).toBe(true);
  });

  it("returns false when the task id is unknown to the store", () => {
    const { result } = renderHook(() => useTaskInFlight("missing"), {
      wrapper: wrapper([task("t-1", "generating")]),
    });
    expect(result.current).toBe(false);
  });

  it("warns for a bulk selection when ANY task is in-flight", () => {
    const { result } = renderHook(() => useTaskInFlight(undefined, ["t-1", "t-2"]), {
      wrapper: wrapper([task("t-1", null), task("t-2", "generating")]),
    });
    expect(result.current).toBe(true);
  });

  it("returns false for a bulk selection when every task is idle", () => {
    const { result } = renderHook(() => useTaskInFlight(undefined, ["t-1", "t-2"]), {
      wrapper: wrapper([task("t-1", null), task("t-2", null)]),
    });
    expect(result.current).toBe(false);
  });
});
