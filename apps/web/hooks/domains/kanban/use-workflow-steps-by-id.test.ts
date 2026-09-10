import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";

type StoreStep = {
  id: string;
  title: string;
  color: string;
  position: number;
  allow_manual_move?: boolean;
};
type MockState = {
  kanban: { workflowId: string | null; steps: StoreStep[] };
  kanbanMulti: { snapshots: Record<string, { steps: StoreStep[] }> };
};

const ACTIVE_WORKFLOW_ID = "workflow-active";

let mockState: MockState = {
  kanban: { workflowId: ACTIVE_WORKFLOW_ID, steps: [] },
  kanbanMulti: { snapshots: {} },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
}));

import { useWorkflowStepsById } from "./use-workflow-steps-by-id";

describe("useWorkflowStepsById", () => {
  it("returns an empty list when no workflow id is given", () => {
    mockState = {
      kanban: { workflowId: ACTIVE_WORKFLOW_ID, steps: [] },
      kanbanMulti: { snapshots: {} },
    };
    const { result } = renderHook(() => useWorkflowStepsById(null));
    expect(result.current).toEqual([]);
  });

  it("resolves from kanban.steps when the workflow id matches the active workflow", () => {
    mockState = {
      kanban: {
        workflowId: ACTIVE_WORKFLOW_ID,
        steps: [
          { id: "b", title: "Work", color: "#222", position: 1 },
          { id: "a", title: "Spec", color: "#111", position: 0 },
        ],
      },
      kanbanMulti: { snapshots: {} },
    };
    const { result } = renderHook(() => useWorkflowStepsById(ACTIVE_WORKFLOW_ID));
    expect(result.current.map((s) => s.id)).toEqual(["a", "b"]);
    expect(result.current[0]).toMatchObject({ id: "a", name: "Spec", position: 0 });
  });

  it("falls back to the workflow snapshot when the id is not the active workflow", () => {
    mockState = {
      kanban: { workflowId: ACTIVE_WORKFLOW_ID, steps: [] },
      kanbanMulti: {
        snapshots: {
          "workflow-other": {
            steps: [{ id: "x", title: "Triage", color: "#333", position: 0 }],
          },
        },
      },
    };
    const { result } = renderHook(() => useWorkflowStepsById("workflow-other"));
    expect(result.current).toHaveLength(1);
    expect(result.current[0]).toMatchObject({ id: "x", name: "Triage" });
  });

  it("returns an empty list when neither the active workflow nor a snapshot resolves it", () => {
    mockState = {
      kanban: { workflowId: ACTIVE_WORKFLOW_ID, steps: [] },
      kanbanMulti: { snapshots: {} },
    };
    const { result } = renderHook(() => useWorkflowStepsById("workflow-unknown"));
    expect(result.current).toEqual([]);
  });

  it("breaks a position tie by ascending id", () => {
    mockState = {
      kanban: {
        workflowId: ACTIVE_WORKFLOW_ID,
        steps: [
          { id: "b", title: "B", color: "#111", position: 0 },
          { id: "a", title: "A", color: "#222", position: 0 },
        ],
      },
      kanbanMulti: { snapshots: {} },
    };
    const { result } = renderHook(() => useWorkflowStepsById(ACTIVE_WORKFLOW_ID));
    expect(result.current.map((s) => s.id)).toEqual(["a", "b"]);
  });
});
