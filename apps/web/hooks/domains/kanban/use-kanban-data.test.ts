import { describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";

type MockKanbanTask = {
  id: string;
  title: string;
  workflowStepId: string;
  state?: string;
  primarySessionId?: string | null;
  primarySessionState?: string | null;
  primaryExecutorType?: string | null;
  primaryExecutorName?: string | null;
  isRemoteExecutor?: boolean;
  sessionCount?: number | null;
  position?: number;
};

type MockState = {
  kanban: {
    workflowId: string | null;
    isLoading: boolean;
    steps: Array<{ id: string; title: string; color: string; position: number }>;
    tasks: MockKanbanTask[];
  };
  workspaces: { activeId: string | null };
  workflows: { activeId: string | null };
  repositories: { itemsByWorkspaceId: Record<string, unknown[]> };
  userSettings: { enablePreviewOnClick: boolean };
};

let mockState: MockState = {
  kanban: { workflowId: "wf-1", isLoading: false, steps: [], tasks: [] },
  workspaces: { activeId: "ws-1" },
  workflows: { activeId: "wf-1" },
  repositories: { itemsByWorkspaceId: {} },
  userSettings: { enablePreviewOnClick: true },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/hooks/use-workflows", () => ({
  useWorkflows: vi.fn(),
}));

vi.mock("@/hooks/use-workflow-snapshot", () => ({
  useWorkflowSnapshot: vi.fn(),
}));

vi.mock("@/hooks/use-user-display-settings", () => ({
  useUserDisplaySettings: () => ({
    settings: {},
    commitSettings: vi.fn(),
    selectedRepositoryIds: new Set<string>(),
  }),
}));

import { useKanbanData } from "./use-kanban-data";

describe("useKanbanData", () => {
  it("preserves runtime fields needed by kanban cards", () => {
    mockState = {
      ...mockState,
      kanban: {
        ...mockState.kanban,
        steps: [
          { id: "step-in-progress", title: "In Progress", color: "bg-blue-500", position: 0 },
        ],
        tasks: [
          {
            id: "task-1",
            title: "Running review task",
            workflowStepId: "step-in-progress",
            state: "REVIEW",
            primarySessionId: "session-1",
            primarySessionState: "RUNNING",
            primaryExecutorType: "remote_docker",
            primaryExecutorName: "Remote Docker",
            isRemoteExecutor: true,
            sessionCount: 1,
          },
        ],
      },
    };

    const { result } = renderHook(() =>
      useKanbanData({
        onWorkspaceChange: vi.fn(),
        onWorkflowChange: vi.fn(),
      }),
    );

    expect(result.current.filteredTasks[0]).toMatchObject({
      primarySessionId: "session-1",
      primarySessionState: "RUNNING",
      primaryExecutorType: "remote_docker",
      primaryExecutorName: "Remote Docker",
      isRemoteExecutor: true,
    });
  });
});
