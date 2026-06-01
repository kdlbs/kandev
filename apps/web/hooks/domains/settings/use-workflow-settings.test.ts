import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import {
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type Workflow,
} from "@/lib/types/http";

type StoreWorkflow = {
  id: string;
  workspaceId: string;
  name: string;
  description?: string | null;
  hidden?: boolean;
  style?: "kanban" | "office" | "custom";
};

let mockWorkflowItems: StoreWorkflow[] = [];

// The workflows list now lives in the TanStack Query cache, read via
// useWorkflowItems. Mock it to return the simulated workspace-scoped list (the
// hook still applies its own workspace + hidden filtering on top).
vi.mock("@/hooks/domains/kanban/use-kanban-snapshots", () => ({
  useWorkflowItems: () => mockWorkflowItems,
}));

import { useWorkflowSettings } from "./use-workflow-settings";

function setStore(items: StoreWorkflow[]) {
  mockWorkflowItems = items;
}

const wf = (id: string, wsId: string, name: string): Workflow => ({
  id: toWorkflowId(id),
  workspace_id: toWorkspaceId(wsId),
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

  it("excludes hidden system workflows from the settings list", () => {
    // System workflows like "Improve Kandev" live in the global store with
    // hidden=true so the kanban can resolve task references, but they must
    // never appear in the management UI.
    const HIDDEN_SYSTEM: StoreWorkflow = {
      id: "wf-improve-kandev",
      workspaceId: "ws-b",
      name: "Improve Kandev",
      hidden: true,
    };
    setStore([STORE_B1, HIDDEN_SYSTEM]);

    const { result } = renderHook(() => useWorkflowSettings([], "ws-b"));

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
    expect(result.current.savedWorkflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });

  it("drops a workflow from the settings list once it becomes hidden", () => {
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

    // Backend flips hidden=true (e.g. healing the improve-kandev record).
    act(() => {
      rerender({ store: [{ ...STORE_B1, hidden: true }] });
    });

    expect(result.current.workflowItems.map((w) => w.id)).toEqual([]);
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

describe("useWorkflowSettings office-style exclusion", () => {
  beforeEach(() => {
    setStore([]);
  });

  it("excludes office-style workflows from the settings list", () => {
    // Office workflows are managed from the Office surface and are not
    // importable/exportable here (ADR-0004). The WS bridge populates the
    // workflows-list cache with office entries too, so the settings hook must
    // filter them out — otherwise Export All would enumerate them (issue #1109).
    const OFFICE: StoreWorkflow = {
      id: "wf-office",
      workspaceId: "ws-b",
      name: "Office Only Workflow",
      style: "office",
    };
    setStore([STORE_B1, OFFICE]);

    const { result } = renderHook(() => useWorkflowSettings([], "ws-b"));

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
    expect(result.current.savedWorkflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });

  it("drops a workflow from the settings list once it becomes office-style", () => {
    const initial = [wf("wf-b1", "ws-b", NAME_B1)];
    const kanbanB1: StoreWorkflow = { ...STORE_B1, style: "kanban" };
    const officeB1: StoreWorkflow = { ...STORE_B1, style: "office" };
    setStore([kanbanB1]);

    const { result, rerender } = renderHook(
      ({ store }: { store: StoreWorkflow[] }) => {
        setStore(store);
        return useWorkflowSettings(initial, "ws-b");
      },
      { initialProps: { store: [kanbanB1] } },
    );

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);

    // Backend reclassifies the workflow as office-style via a workflow.updated WS event.
    act(() => {
      rerender({ store: [officeB1] });
    });

    expect(result.current.workflowItems.map((w) => w.id)).toEqual([]);
  });
});
