import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AppState } from "@/lib/state/store";
import { sessionId, taskId } from "@/lib/types/ids";
import type { Task, TaskSession } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  fetchTask: vi.fn(),
  fetchTaskSession: vi.fn(),
  fetchUserSettings: vi.fn(),
  fetchWorkflowSnapshot: vi.fn(),
  listAgents: vi.fn(),
  listAvailableAgents: vi.fn(),
  listExecutors: vi.fn(),
  listRepositories: vi.fn(),
  listSessionTurns: vi.fn(),
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
  listAvailableAgents: mocks.listAvailableAgents,
  listExecutors: mocks.listExecutors,
  listRepositories: mocks.listRepositories,
  listTaskSessionMessages: mocks.listTaskSessionMessages,
  listTaskSessions: mocks.listTaskSessions,
  listWorkflows: mocks.listWorkflows,
  listWorkspaces: mocks.listWorkspaces,
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  listSessionTurns: mocks.listSessionTurns,
}));

vi.mock("@/lib/api/domains/user-shell-api", () => ({
  fetchTerminals: vi.fn(),
}));

import { fetchSessionDataForTask } from "./session-page-state";

const NOW = "2026-07-16T12:00:00Z";
const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const WORKSPACE_ID = "workspace-office";

function makeTask(overrides: Partial<Task> = {}): Task {
  return {
    id: TASK_ID,
    workspace_id: WORKSPACE_ID,
    workflow_id: "",
    workflow_step_id: "",
    position: 0,
    title: "Office task",
    description: "",
    state: "TODO",
    priority: "medium",
    created_at: NOW,
    updated_at: NOW,
    ...overrides,
  } as Task;
}

function makeSession(): TaskSession {
  return {
    id: sessionId(SESSION_ID),
    task_id: taskId(TASK_ID),
    state: "COMPLETED",
    started_at: NOW,
    updated_at: NOW,
  };
}

function mockDefaultHydrationData() {
  vi.clearAllMocks();
  mocks.fetchTask.mockResolvedValue(makeTask());
  mocks.listTaskSessions.mockResolvedValue({ sessions: [] });
  mocks.listAgents.mockResolvedValue({ agents: [] });
  mocks.listAvailableAgents.mockReturnValue(new Promise(() => {}));
  mocks.listExecutors.mockResolvedValue({ executors: [] });
  mocks.listRepositories.mockResolvedValue({ repositories: [] });
  mocks.listWorkflows.mockResolvedValue({ workflows: [] });
  mocks.fetchUserSettings.mockResolvedValue(null);
  mocks.listWorkspaces.mockResolvedValue({
    workspaces: [
      {
        id: WORKSPACE_ID,
        name: "Office",
        owner_id: "",
        office_workflow_id: "workflow-office",
        created_at: NOW,
        updated_at: NOW,
      },
    ],
  });
}

beforeEach(mockDefaultHydrationData);

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("fetchSessionDataForTask model hydration", () => {
  it("hydrates the persisted model selector before the first render", async () => {
    const session = {
      ...makeSession(),
      metadata: {
        runtime_config: {
          model: "mock-fast",
          config_options: { effort: "medium" },
        },
        runtime_config_overrides: {
          config_options: { effort: "high" },
        },
        acp_config_baseline: { model: "mock-fast", effort: "medium" },
        acp_model_state: {
          current_model_id: "mock-fast",
          models: [{ model_id: "mock-fast", name: "Mock Fast" }],
          config_options: [
            {
              type: "select",
              id: "effort",
              name: "Effort",
              current_value: "medium",
              options: [
                { value: "medium", name: "Medium" },
                { value: "high", name: "High" },
              ],
            },
          ],
        },
      },
    };
    mocks.listTaskSessions.mockResolvedValue({ sessions: [session], total: 1 });
    mocks.fetchTaskSession.mockResolvedValue({ session });

    const result = await fetchSessionDataForTask(TASK_ID);
    const initialState = result.initialState as unknown as Partial<AppState>;

    expect(initialState.sessionModels?.bySessionId[session.id]).toMatchObject({
      currentModelId: "mock-fast",
      configBaseline: { model: "mock-fast", effort: "medium" },
      configOptions: [expect.objectContaining({ id: "effort", currentValue: "high" })],
    });
  });

  it("hydrates provider-restored effective model and mode over saved launch overrides", async () => {
    const session = {
      ...makeSession(),
      metadata: {
        runtime_config: { model: "saved-model", mode: "saved-mode" },
        runtime_config_overrides: { model: "saved-override-model", mode: "saved-override-mode" },
        acp_model_state: {
          settings_policy: "provider_restored",
          current_model_id: "effective-model",
          current_mode_id: "effective-mode",
          models: [{ model_id: "effective-model", name: "Effective Model" }],
          config_options: [],
        },
      },
    };
    mocks.listTaskSessions.mockResolvedValue({ sessions: [session], total: 1 });
    mocks.fetchTaskSession.mockResolvedValue({ session });

    const result = await fetchSessionDataForTask(TASK_ID);
    const initialState = result.initialState as unknown as Partial<AppState>;

    expect(initialState.sessionModels?.bySessionId[session.id]).toMatchObject({
      currentModelId: "effective-model",
      settingsPolicy: "provider_restored",
    });
    expect(initialState.sessionMode?.bySessionId[session.id]).toMatchObject({
      currentModeId: "effective-mode",
      settingsPolicy: "provider_restored",
    });
  });

  it("keeps unknown provider-restored selectors empty instead of reviving saved inputs", async () => {
    const session = {
      ...makeSession(),
      metadata: {
        runtime_config: { model: "saved-model", mode: "saved-mode" },
        runtime_config_overrides: { model: "saved-override-model", mode: "saved-override-mode" },
        acp_model_state: {
          settings_policy: "provider_restored",
          current_model_id: "",
          current_mode_id: "",
          models: [],
          config_options: [],
        },
      },
    };
    mocks.listTaskSessions.mockResolvedValue({ sessions: [session], total: 1 });
    mocks.fetchTaskSession.mockResolvedValue({ session });

    const result = await fetchSessionDataForTask(TASK_ID);
    const initialState = result.initialState as unknown as Partial<AppState>;

    expect(initialState.sessionModels?.bySessionId[session.id]).toMatchObject({
      currentModelId: "",
      settingsPolicy: "provider_restored",
    });
    expect(initialState.sessionMode?.bySessionId[session.id]).toMatchObject({
      currentModeId: "",
      settingsPolicy: "provider_restored",
    });
  });
});
