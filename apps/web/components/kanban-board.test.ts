import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useMultiSelectDerived } from "./kanban-board";

const board = { workflowId: null, tasks: [], steps: [] } as never;
const snapshots = {
  a: {
    workflowId: "a",
    tasks: [{ id: "task-a", workflowId: "a" }],
    steps: [
      { id: "a-start", title: "Start", position: 0, color: "blue" },
      { id: "a-done", title: "Done", position: 1, color: null },
    ],
  },
  b: {
    workflowId: "b",
    tasks: [{ id: "task-b", workflowId: "b" }],
    steps: [{ id: "b-done", title: "Done B", position: 0, color: "blue" }],
  },
} as never;

describe("bulk move destinations", () => {
  it("offers only visible steps of the selected workflow", () => {
    const { result } = renderHook(() =>
      useMultiSelectDerived(new Set(["task-a"]), snapshots, [], { a: ["a-start"] }, board),
    );
    expect(result.current.multiSelectSteps).toEqual([{ id: "a-done", title: "Done", color: "" }]);
    expect(result.current.isMixedWorkflowSelection).toBe(false);
  });

  it.each([
    ["task-a", "task-b"],
    ["task-a", "missing"],
  ])("offers no destinations for mixed or unknown selection %s", (...ids) => {
    const { result } = renderHook(() =>
      useMultiSelectDerived(new Set(ids), snapshots, [], {}, board),
    );
    expect(result.current.multiSelectSteps).toEqual([]);
    expect(result.current.isMixedWorkflowSelection).toBe(true);
  });
});

it("uses matching hydrated board steps for a task-bearing placeholder", () => {
  const placeholder = {
    a: {
      workflowId: "a",
      workflowName: "A",
      isPlaceholder: true,
      tasks: [{ id: "task-a", workflowId: "a" }],
      steps: [],
    },
  } as never;
  const hydrated = {
    workflowId: "a",
    tasks: [{ id: "task-a", workflowId: "a" }],
    steps: [{ id: "a-done", title: "Done", position: 1, color: "" }],
  } as never;
  const { result } = renderHook(() =>
    useMultiSelectDerived(new Set(["task-a"]), placeholder, [], {}, hydrated),
  );
  expect(result.current.multiSelectSteps.map((step) => step.id)).toEqual(["a-done"]);
});

it("does not use another workflow's hydrated steps for a placeholder", () => {
  const placeholder = {
    b: {
      workflowId: "b",
      workflowName: "B",
      isPlaceholder: true,
      tasks: [{ id: "task-b", workflowId: "b" }],
      steps: [],
    },
  } as never;
  const hydrated = {
    workflowId: "a",
    tasks: [],
    steps: [{ id: "a-done", title: "Done", position: 1, color: "" }],
  } as never;
  const { result } = renderHook(() =>
    useMultiSelectDerived(new Set(["task-b"]), placeholder, [], {}, hydrated),
  );
  expect(result.current.multiSelectSteps).toEqual([]);
});

it("uses nonempty snapshot steps and rejects conflicting target ownership", () => {
  const current = {
    a: {
      workflowId: "a",
      tasks: [{ id: "task-a", workflowId: "a" }],
      steps: [
        { id: "shared", title: "Shared", position: 0, color: "" },
        { id: "a-only", title: "A", position: 1, color: "" },
      ],
    },
    b: {
      workflowId: "b",
      tasks: [],
      steps: [{ id: "shared", title: "Shared B", position: 0, color: "" }],
    },
  } as never;
  const hydrated = {
    workflowId: "a",
    tasks: [{ id: "task-a", workflowId: "a" }],
    steps: [{ id: "board-only", title: "Board", position: 0, color: "" }],
  } as never;
  const { result } = renderHook(() =>
    useMultiSelectDerived(new Set(["task-a"]), current, [], {}, hydrated),
  );
  expect(result.current.multiSelectSteps.map((step) => step.id)).toEqual(["a-only"]);
});
