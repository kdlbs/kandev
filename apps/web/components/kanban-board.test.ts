import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useMultiSelectDerived } from "./kanban-board";

type SnapEntry = {
  tasks: { id: string }[];
  steps: { id: string; title: string; color?: string | null }[];
};

const ACTIVE_STEPS = [{ id: "active-1", title: "Active step", color: "blue" }];

describe("useMultiSelectDerived — bulk-move step list hidden-step filter", () => {
  it("falls back to the board's active steps when nothing is selected", () => {
    const { result } = renderHook(() => useMultiSelectDerived(new Set(), {}, ACTIVE_STEPS, {}));

    expect(result.current.multiSelectSteps).toEqual([
      { id: "active-1", title: "Active step", color: "blue" },
    ]);
    expect(result.current.isMixedWorkflowSelection).toBe(false);
  });

  it("excludes a step hidden in the selection's own workflow from the bulk-move list", () => {
    const snapshots: Record<string, SnapEntry> = {
      "wf-1": {
        tasks: [{ id: "task-1" }],
        steps: [
          { id: "step-visible", title: "Visible" },
          { id: "step-hidden", title: "Hidden" },
        ],
      },
    };

    const { result } = renderHook(() =>
      useMultiSelectDerived(new Set(["task-1"]), snapshots, ACTIVE_STEPS, {
        "wf-1": ["step-hidden"],
      }),
    );

    expect(result.current.multiSelectSteps.map((s) => s.id)).toEqual(["step-visible"]);
  });

  it("does not filter by a different workflow's hidden set", () => {
    const snapshots: Record<string, SnapEntry> = {
      "wf-1": {
        tasks: [{ id: "task-1" }],
        steps: [
          { id: "step-a", title: "A" },
          { id: "step-b", title: "B" },
        ],
      },
      "wf-2": {
        tasks: [{ id: "task-2" }],
        steps: [{ id: "step-a", title: "A (wf-2)" }],
      },
    };

    // Hiding step-a in wf-2 must not affect the bulk-move list for a selection
    // whose task lives in wf-1.
    const { result } = renderHook(() =>
      useMultiSelectDerived(new Set(["task-1"]), snapshots, ACTIVE_STEPS, {
        "wf-2": ["step-a"],
      }),
    );

    expect(result.current.multiSelectSteps.map((s) => s.id)).toEqual(["step-a", "step-b"]);
  });

  it("normalizes a null step color to an empty string", () => {
    const snapshots: Record<string, SnapEntry> = {
      "wf-1": {
        tasks: [{ id: "task-1" }],
        steps: [{ id: "step-1", title: "Step 1", color: null }],
      },
    };

    const { result } = renderHook(() =>
      useMultiSelectDerived(new Set(["task-1"]), snapshots, ACTIVE_STEPS, {}),
    );

    expect(result.current.multiSelectSteps).toEqual([{ id: "step-1", title: "Step 1", color: "" }]);
  });

  it("flags a mixed-workflow selection across two workflows", () => {
    const snapshots: Record<string, SnapEntry> = {
      "wf-1": { tasks: [{ id: "task-1" }], steps: [] },
      "wf-2": { tasks: [{ id: "task-2" }], steps: [] },
    };

    const { result } = renderHook(() =>
      useMultiSelectDerived(new Set(["task-1", "task-2"]), snapshots, ACTIVE_STEPS, {}),
    );

    expect(result.current.isMixedWorkflowSelection).toBe(true);
  });
});
