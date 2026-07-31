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
    ],
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { items: [], activeId: mocks.activeWorkspaceId },
      workflows: { items: mocks.workflows, activeId: mocks.activeWorkflowId },
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
      repositoryIds: [],
      tasksListShowDetails: false,
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
