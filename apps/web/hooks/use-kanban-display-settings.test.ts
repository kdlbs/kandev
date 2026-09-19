import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  const workspaceId = "workspace-1";
  const workflowId = "workflow-1";
  return {
    workspaceId,
    workflowId,
    activeWorkspaceId: workspaceId as string | null,
    activeWorkflowId: workflowId as string | null,
    setActiveWorkspace: vi.fn(),
    setActiveWorkflow: vi.fn(),
    commitSettings: vi.fn(),
    setView: vi.fn(),
    workflows: [
      { id: workflowId, workspaceId },
      { id: "workflow-2", workspaceId: "workspace-2" },
    ] as { id: string; workspaceId: string; hidden?: boolean }[],
    snapshots: {} as Record<string, unknown>,
    repositoryIds: [] as string[],
    hiddenWorkflowStepIds: {} as Record<string, string[]>,
    kanbanSort: "created_desc" as "created_desc" | "priority_desc",
    kanbanPriorityFilterTokens: [] as string[],
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { items: [], activeId: mocks.activeWorkspaceId },
      workflows: { items: mocks.workflows, activeId: mocks.activeWorkflowId },
      kanbanMulti: { snapshots: mocks.snapshots },
      setActiveWorkspace: mocks.setActiveWorkspace,
      setActiveWorkflow: mocks.setActiveWorkflow,
      userSettings: { enablePreviewOnClick: false },
    }),
}));
vi.mock("@/hooks/use-task-listing-view", () => ({
  useTaskListingView: () => ({ effectiveView: "kanban", setView: mocks.setView }),
}));
vi.mock("@/hooks/use-user-display-settings", () => ({
  useUserDisplaySettings: () => ({
    settings: {
      workspaceId: mocks.workspaceId,
      workflowId: mocks.workflowId,
      repositoryIds: mocks.repositoryIds,
      tasksListShowDetails: false,
      hiddenWorkflowStepIds: mocks.hiddenWorkflowStepIds,
      kanbanSort: mocks.kanbanSort,
      kanbanPriorityFilterTokens: mocks.kanbanPriorityFilterTokens,
    },
    commitSettings: mocks.commitSettings,
    repositories: [],
    repositoriesLoading: false,
    allRepositoriesSelected: true,
  }),
}));

import { useKanbanDisplaySettings } from "./use-kanban-display-settings";

function resetMocks() {
  mocks.activeWorkspaceId = mocks.workspaceId;
  mocks.activeWorkflowId = mocks.workflowId;
  mocks.setActiveWorkspace.mockReset();
  mocks.setActiveWorkflow.mockReset();
  mocks.setActiveWorkspace.mockImplementation((workspaceId: string | null) => {
    mocks.activeWorkspaceId = workspaceId;
  });
  mocks.setActiveWorkflow.mockImplementation((workflowId: string | null) => {
    mocks.activeWorkflowId = workflowId;
  });
  mocks.commitSettings.mockReset();
  mocks.setView.mockReset();
  mocks.snapshots = {};
  mocks.repositoryIds = [];
  mocks.hiddenWorkflowStepIds = {};
  mocks.kanbanSort = "created_desc";
  mocks.kanbanPriorityFilterTokens = [];
  window.history.replaceState({}, "", "/");
}

beforeEach(resetMocks);
afterEach(resetMocks);

describe("useKanbanDisplaySettings", () => {
  it("keeps workspace and workflow scope in task overview history", () => {
    const { result, rerender } = renderHook(() => useKanbanDisplaySettings());

    act(() => result.current.onWorkspaceChange("workspace-2"));
    expect(window.location.search).toBe("?home=overview&workspaceId=workspace-2");
    rerender();

    act(() => result.current.onWorkflowChange("workflow-2"));
    expect(window.location.search).toBe(
      "?home=overview&workspaceId=workspace-2&workflowId=workflow-2",
    );
    rerender();

    act(() => result.current.onWorkflowChange(null));
    expect(window.location.search).toBe("?home=overview&workspaceId=workspace-2");
  });
});

describe("useKanbanDisplaySettings — view mode", () => {
  it("stores each listing view under its own name", () => {
    const { result } = renderHook(() => useKanbanDisplaySettings());

    act(() => result.current.onViewModeChange("threads"));
    expect(mocks.setView).toHaveBeenCalledWith("threads");

    act(() => result.current.onViewModeChange("graph2"));
    expect(mocks.setView).toHaveBeenCalledWith("pipeline");

    act(() => result.current.onViewModeChange("list"));
    expect(mocks.setView).toHaveBeenCalledWith("list");
  });
});

describe("useKanbanDisplaySettings — step visibility", () => {
  it("derives eligibleWorkflows from selectWorkflowSwimlanes' eligible set, not the post-filter board", () => {
    mocks.snapshots = { [mocks.workflowId]: { steps: [], tasks: [] } };
    mocks.activeWorkflowId = null; // "All Workflows"

    const { result } = renderHook(() => useKanbanDisplaySettings());

    // workflow-2 has no loaded snapshot, so it is correctly excluded; only the
    // workflow with a snapshot is eligible — same source the board renders from.
    expect(result.current.eligibleWorkflows.map((wf) => wf.id)).toEqual([mocks.workflowId]);
  });

  it("unticking a shown step adds it to that workflow's hidden set and preserves other settings", () => {
    mocks.repositoryIds = ["repo-1"];
    const { result } = renderHook(() => useKanbanDisplaySettings());

    act(() => result.current.onToggleStepVisibility(mocks.workflowId, "step-a"));

    expect(mocks.commitSettings).toHaveBeenCalledWith({
      workspaceId: mocks.workspaceId,
      workflowId: mocks.workflowId,
      repositoryIds: ["repo-1"],
      hiddenWorkflowStepIds: { [mocks.workflowId]: ["step-a"] },
      workflowIdsWithAutoHideEmptySteps: undefined,
      kanbanSort: mocks.kanbanSort,
      kanbanPriorityFilterTokens: mocks.kanbanPriorityFilterTokens,
    });
  });

  it("re-ticking a hidden step removes only that id, idempotently", () => {
    mocks.hiddenWorkflowStepIds = { [mocks.workflowId]: ["step-a", "step-b"] };
    const { result } = renderHook(() => useKanbanDisplaySettings());

    act(() => result.current.onToggleStepVisibility(mocks.workflowId, "step-a"));

    expect(mocks.commitSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        hiddenWorkflowStepIds: { [mocks.workflowId]: ["step-b"] },
      }),
    );
  });

  it("scopes the toggle to the given workflow, leaving other workflows' hidden sets untouched", () => {
    mocks.hiddenWorkflowStepIds = { "workflow-2": ["step-z"] };
    const { result } = renderHook(() => useKanbanDisplaySettings());

    act(() => result.current.onToggleStepVisibility(mocks.workflowId, "step-a"));

    expect(mocks.commitSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        hiddenWorkflowStepIds: { "workflow-2": ["step-z"], [mocks.workflowId]: ["step-a"] },
      }),
    );
  });
});

describe("useKanbanDisplaySettings — board sort and priority filter", () => {
  it("commits the selected board sort token, preserving other settings", () => {
    mocks.repositoryIds = ["repo-1"];
    const { result } = renderHook(() => useKanbanDisplaySettings());

    act(() => result.current.onBoardSortChange("priority_desc"));

    expect(mocks.commitSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        repositoryIds: ["repo-1"],
        kanbanSort: "priority_desc",
      }),
    );
  });

  it("toggling a priority token adds it to an empty selection", () => {
    const { result } = renderHook(() => useKanbanDisplaySettings());

    act(() => result.current.onPriorityFilterChange("critical"));

    expect(mocks.commitSettings).toHaveBeenCalledWith(
      expect.objectContaining({ kanbanPriorityFilterTokens: ["critical"] }),
    );
  });

  it("toggling an already-selected priority token removes it", () => {
    mocks.kanbanPriorityFilterTokens = ["critical", "high"];
    const { result } = renderHook(() => useKanbanDisplaySettings());

    act(() => result.current.onPriorityFilterChange("critical"));

    expect(mocks.commitSettings).toHaveBeenCalledWith(
      expect.objectContaining({ kanbanPriorityFilterTokens: ["high"] }),
    );
  });

  it("exposes the current board sort and priority filter selection", () => {
    mocks.kanbanSort = "priority_desc";
    mocks.kanbanPriorityFilterTokens = ["low"];
    const { result } = renderHook(() => useKanbanDisplaySettings());

    expect(result.current.boardSort).toBe("priority_desc");
    expect(result.current.priorityFilterTokens).toEqual(["low"]);
  });
});
