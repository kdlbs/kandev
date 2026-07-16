import { beforeEach, describe, expect, it, vi } from "vitest";

import type { AppState } from "@/lib/state/store";
import type { Task } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  fetchTask: vi.fn(),
  fetchTaskSession: vi.fn(),
  fetchUserSettings: vi.fn(),
  fetchWorkflowSnapshot: vi.fn(),
  listAgents: vi.fn(),
  listRepositories: vi.fn(),
  listTaskSessionMessages: vi.fn(),
  listTaskSessions: vi.fn(),
  listWorkflows: vi.fn(),
  listWorkspaces: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  fetchTask: mocks.fetchTask,
  fetchTaskSession: mocks.fetchTaskSession,
  fetchUserSettings: mocks.fetchUserSettings,
  fetchWorkflowSnapshot: mocks.fetchWorkflowSnapshot,
  listAgents: mocks.listAgents,
  listRepositories: mocks.listRepositories,
  listTaskSessionMessages: mocks.listTaskSessionMessages,
  listTaskSessions: mocks.listTaskSessions,
  listWorkflows: mocks.listWorkflows,
  listWorkspaces: mocks.listWorkspaces,
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  listSessionTurns: vi.fn(),
}));

vi.mock("@/lib/api/domains/user-shell-api", () => ({
  fetchTerminals: vi.fn(),
}));

import { fetchSessionDataForTask } from "./session-page-state";

const NOW = "2026-07-16T12:00:00Z";

describe("fetchSessionDataForTask workspace hydration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.fetchTask.mockResolvedValue({
      id: "task-1",
      workspace_id: "workspace-office",
      workflow_id: "",
      workflow_step_id: "",
      position: 0,
      title: "Office task",
      description: "",
      state: "TODO",
      priority: 0,
      created_at: NOW,
      updated_at: NOW,
    } as unknown as Task);
    mocks.listTaskSessions.mockResolvedValue({ sessions: [] });
    mocks.listAgents.mockResolvedValue({ agents: [] });
    mocks.listRepositories.mockResolvedValue({ repositories: [] });
    mocks.listWorkflows.mockResolvedValue({ workflows: [] });
    mocks.fetchUserSettings.mockResolvedValue(null);
    mocks.listWorkspaces.mockResolvedValue({
      workspaces: [
        {
          id: "workspace-office",
          name: "Office",
          owner_id: "",
          office_workflow_id: "workflow-office",
          created_at: NOW,
          updated_at: NOW,
        },
      ],
    });
  });

  it("preserves the Office workflow ID in task-route state", async () => {
    const result = await fetchSessionDataForTask("task-1");
    const initialState = result.initialState as unknown as Partial<AppState>;

    expect(initialState.workspaces?.items[0]?.office_workflow_id).toBe("workflow-office");
  });
});
