import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { Workflow } from "@/lib/types/http";

type StoreWorkflow = {
  id: string;
  workspaceId: string;
  name: string;
  description?: string | null;
};

type MockState = { workflows: { items: StoreWorkflow[] } };

let mockState: MockState = { workflows: { items: [] } };

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
}));

import { useWorkflowSettings } from "./use-workflow-settings";

function setStore(items: StoreWorkflow[]) {
  mockState = { workflows: { items } };
}

const wf = (id: string, workspaceId: string, name: string): Workflow => ({
  id,
  workspace_id: workspaceId,
  name,
  description: "",
  created_at: "",
  updated_at: "",
});

const NAME_A1 = "Workflow A1";
const NAME_B1 = "Workflow B1";
const STORE_A1: StoreWorkflow = { id: "wf-a1", workspaceId: "ws-a", name: NAME_A1 };
const STORE_B1: StoreWorkflow = { id: "wf-b1", workspaceId: "ws-b", name: NAME_B1 };

beforeEach(() => {
  setStore([]);
});

describe("useWorkflowSettings", () => {
  it("does not include workflows from other workspaces present in the global store", () => {
    // Store has a workflow from workspace A (e.g. user previously visited it)
    setStore([STORE_A1]);

    // We render the settings hook for workspace B with no initial workflows
    const { result } = renderHook(() => useWorkflowSettings([], "ws-b"));

    // The leaked workflow from workspace A must not appear in B's list
    expect(result.current.workflowItems).toHaveLength(0);
    expect(result.current.savedWorkflowItems).toHaveLength(0);
  });

  it("adds workflows from the store that belong to the current workspace", () => {
    setStore([STORE_A1, STORE_B1]);

    const { result } = renderHook(() => useWorkflowSettings([], "ws-b"));

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });

  it("does not remove a workspace's workflows when an unrelated workspace's entries are added/removed in the store", () => {
    // Initial: workspace B has one saved workflow from SSR
    const initial = [wf("wf-b1", "ws-b", NAME_B1)];
    setStore([STORE_B1]);

    const { result, rerender } = renderHook(
      ({ store }: { store: StoreWorkflow[] }) => {
        setStore(store);
        return useWorkflowSettings(initial, "ws-b");
      },
      { initialProps: { store: [STORE_B1] } },
    );

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);

    // Workspace A workflow is added to the store (e.g. WS event from another tab)
    act(() => {
      rerender({ store: [STORE_B1, STORE_A1] });
    });
    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);

    // Workspace A workflow is removed from the store — must not affect B's list
    act(() => {
      rerender({ store: [STORE_B1] });
    });
    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });

  it("falls back to the unscoped store when no workspaceId is provided", () => {
    setStore([STORE_A1, STORE_B1]);

    const { result } = renderHook(() => useWorkflowSettings([]));

    expect(result.current.workflowItems.map((w) => w.id).sort()).toEqual(["wf-a1", "wf-b1"]);
  });

  it("syncs name updates from the store within the current workspace", () => {
    const initial = [wf("wf-b1", "ws-b", NAME_B1)];
    setStore([STORE_B1]);

    const { result, rerender } = renderHook(
      ({ store }: { store: StoreWorkflow[] }) => {
        setStore(store);
        return useWorkflowSettings(initial, "ws-b");
      },
      { initialProps: { store: [STORE_B1] } },
    );

    expect(result.current.workflowItems[0].name).toEqual(NAME_B1);

    act(() => {
      rerender({ store: [{ id: "wf-b1", workspaceId: "ws-b", name: "Renamed B1" }] });
    });

    expect(result.current.workflowItems[0].name).toEqual("Renamed B1");
  });

  it("starts scoping store entries once a workspaceId becomes defined", () => {
    setStore([STORE_A1, STORE_B1]);

    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId?: string }) => useWorkflowSettings([], workspaceId),
      { initialProps: { workspaceId: undefined as string | undefined } },
    );

    // No workspaceId → unscoped fallback shows both
    expect(result.current.workflowItems.map((w) => w.id).sort()).toEqual(["wf-a1", "wf-b1"]);

    act(() => {
      rerender({ workspaceId: "ws-b" });
    });

    // Once scoped to B, A's workflow is dropped
    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });
});
