import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { mergeInitialState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import { workflowId } from "@/lib/types/ids";
import type { Task } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  fetchTask: vi.fn(),
  fetchTaskSession: vi.fn(),
  fetchUserSettings: vi.fn(),
  fetchWorkflowSnapshot: vi.fn(),
  listAgents: vi.fn(),
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

import { fetchSessionDataForTask, OPTIONAL_HYDRATION_TIMEOUT_MS } from "./session-page-state";

const NOW = "2026-07-16T12:00:00Z";
const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const WORKSPACE_ID = "workspace-office";
const WORKFLOW_ID = "workflow-1";

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
    priority: 0,
    created_at: NOW,
    updated_at: NOW,
    ...overrides,
  } as Task;
}

function mockDefaultHydrationData() {
  vi.clearAllMocks();
  mocks.fetchTask.mockResolvedValue(makeTask());
  mocks.listTaskSessions.mockResolvedValue({ sessions: [] });
  mocks.listAgents.mockResolvedValue({ agents: [] });
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

function makeSession() {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    state: "completed",
    started_at: NOW,
    updated_at: NOW,
  };
}

beforeEach(mockDefaultHydrationData);

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("fetchSessionDataForTask initial hydration", () => {
  it("preserves the Office workflow ID in task-route state", async () => {
    const result = await fetchSessionDataForTask(TASK_ID);
    const initialState = result.initialState as unknown as Partial<AppState>;

    expect(initialState.workspaces?.items[0]?.office_workflow_id).toBe("workflow-office");
  });

  it("hydrates persisted per-turn runtime configuration after a page reload", async () => {
    const session = makeSession();
    const runtimeConfigSnapshot = {
      model: "gpt-5.6-sol",
      mode: "agent",
      config_options: [
        {
          id: "reasoning_effort",
          name: "Reasoning effort",
          value: "high",
          value_name: "High",
        },
      ],
      config_baseline: { reasoning_effort: "medium" },
    };

    mocks.listTaskSessions.mockResolvedValue({ sessions: [session], total: 1 });
    mocks.fetchTaskSession.mockResolvedValue({ session });
    mocks.listSessionTurns.mockResolvedValue({
      turns: [
        {
          id: "turn-1",
          session_id: session.id,
          task_id: TASK_ID,
          started_at: NOW,
          completed_at: NOW,
          metadata: { runtime_config_snapshot: runtimeConfigSnapshot },
          created_at: NOW,
          updated_at: NOW,
        },
      ],
      total: 1,
    });
    mocks.listTaskSessionMessages.mockResolvedValue(null);

    const result = await fetchSessionDataForTask(TASK_ID);
    const hydratedState = mergeInitialState(result.initialState);

    expect(hydratedState.turns.bySession[session.id]?.[0]?.metadata).toEqual({
      runtime_config_snapshot: runtimeConfigSnapshot,
    });
  });
});

describe("fetchSessionDataForTask agent hydration", () => {
  it("renders core task and session state when optional agent hydration never settles", async () => {
    vi.useFakeTimers();
    const session = makeSession();
    mocks.listTaskSessions.mockResolvedValue({ sessions: [session], total: 1 });
    mocks.fetchTaskSession.mockResolvedValue({ session });
    mocks.listAgents.mockReturnValue(new Promise(() => {}));
    mocks.listSessionTurns.mockResolvedValue({ turns: [], total: 0 });
    mocks.listTaskSessionMessages.mockResolvedValue(null);

    let result: Awaited<ReturnType<typeof fetchSessionDataForTask>> | undefined;
    void fetchSessionDataForTask(TASK_ID).then((next) => {
      result = next;
    });

    await vi.advanceTimersByTimeAsync(OPTIONAL_HYDRATION_TIMEOUT_MS);

    expect(result).toBeDefined();
    const hydratedState = mergeInitialState(result!.initialState);

    expect(hydratedState.taskSessionsByTask.itemsByTaskId[TASK_ID]?.[0]?.id).toBe(SESSION_ID);
  });

  it("renders core task state when optional agent hydration fails", async () => {
    mocks.listAgents.mockRejectedValue(new Error("agents unavailable"));

    const result = await fetchSessionDataForTask(TASK_ID);

    expect(result.task.id).toBe(TASK_ID);
  });
});

describe("fetchSessionDataForTask optional enrichment", () => {
  it("omits failed optional workflow, agent, and repository state for client recovery", async () => {
    mocks.fetchTask.mockResolvedValue(makeTask({ workflow_id: workflowId(WORKFLOW_ID) }));
    mocks.fetchWorkflowSnapshot.mockRejectedValue(new Error("workflow unavailable"));
    mocks.listAgents.mockRejectedValue(new Error("agents unavailable"));
    mocks.listRepositories.mockRejectedValue(new Error("repositories unavailable"));

    const result = await fetchSessionDataForTask(TASK_ID);
    const initialState = result.initialState as Partial<AppState>;

    expect(initialState.kanban).toBeUndefined();
    expect(initialState.settingsData).toBeUndefined();
    expect(initialState.repositories).toBeUndefined();
  });

  it("keeps the destination workspace active when workspace enrichment fails", async () => {
    mocks.listWorkspaces.mockRejectedValue(new Error("workspaces unavailable"));

    const result = await fetchSessionDataForTask(TASK_ID);
    const initialState = result.initialState as Partial<AppState>;

    expect(initialState.workspaces).toEqual({ activeId: WORKSPACE_ID });
  });

  it("keeps the destination workspace active without marking workspace items loaded after timeout", async () => {
    vi.useFakeTimers();
    mocks.listWorkspaces.mockReturnValue(new Promise(() => {}));

    const resultPromise = fetchSessionDataForTask(TASK_ID);
    await vi.advanceTimersByTimeAsync(OPTIONAL_HYDRATION_TIMEOUT_MS);

    const result = await resultPromise;
    const initialState = result.initialState as Partial<AppState>;

    expect(initialState.workspaces).toEqual({ activeId: WORKSPACE_ID });
  });

  it("hydrates workspace items alongside the destination workspace when enrichment succeeds", async () => {
    const result = await fetchSessionDataForTask(TASK_ID);
    const initialState = result.initialState as Partial<AppState>;

    expect(initialState.workspaces).toMatchObject({
      activeId: WORKSPACE_ID,
      items: [expect.objectContaining({ id: WORKSPACE_ID, name: "Office" })],
    });
  });

  it("retains successful optional enrichment as authoritative initial state", async () => {
    mocks.fetchTask.mockResolvedValue(makeTask({ workflow_id: workflowId(WORKFLOW_ID) }));
    mocks.fetchWorkflowSnapshot.mockResolvedValue({
      workflow: { id: "workflow-1" },
      steps: [],
      tasks: [],
    });

    const result = await fetchSessionDataForTask(TASK_ID);
    const initialState = result.initialState as Partial<AppState>;

    expect(initialState.kanban?.workflowId).toBe(WORKFLOW_ID);
    expect(initialState.settingsData?.agentsLoaded).toBe(true);
    expect(initialState.repositories?.loadedByWorkspaceId?.[WORKSPACE_ID]).toBe(true);
  });

  it("omits timed-out optional slices so client hooks can recover them", async () => {
    vi.useFakeTimers();
    mocks.fetchTask.mockResolvedValue(makeTask({ workflow_id: workflowId(WORKFLOW_ID) }));
    mocks.fetchWorkflowSnapshot.mockReturnValue(new Promise(() => {}));
    mocks.listAgents.mockReturnValue(new Promise(() => {}));
    mocks.listRepositories.mockReturnValue(new Promise(() => {}));

    const resultPromise = fetchSessionDataForTask(TASK_ID);
    await vi.advanceTimersByTimeAsync(OPTIONAL_HYDRATION_TIMEOUT_MS);

    const result = await resultPromise;
    const initialState = result.initialState as Partial<AppState>;
    const hydratedState = mergeInitialState(initialState);

    expect(initialState.kanban).toBeUndefined();
    expect(initialState.settingsData).toBeUndefined();
    expect(initialState.repositories).toBeUndefined();
    expect(hydratedState.settingsData.agentsLoaded).toBe(false);
  });
});

describe("fetchSessionDataForTask timeout behavior", () => {
  it("uses one deadline when active-session and enrichment requests both stall", async () => {
    vi.useFakeTimers();
    const session = makeSession();
    let rejectAgentRequest!: (error: Error) => void;
    const stalledAgentRequest = new Promise<never>((_, reject) => {
      rejectAgentRequest = reject;
    });
    mocks.listTaskSessions.mockResolvedValue({ sessions: [session], total: 1 });
    mocks.fetchTaskSession.mockReturnValue(new Promise(() => {}));
    mocks.listAgents.mockReturnValue(stalledAgentRequest);
    mocks.listSessionTurns.mockResolvedValue({ turns: [], total: 0 });
    mocks.listTaskSessionMessages.mockResolvedValue(null);

    let result: Awaited<ReturnType<typeof fetchSessionDataForTask>> | undefined;
    const resultPromise = fetchSessionDataForTask(TASK_ID).then((next) => {
      result = next;
    });

    await vi.advanceTimersByTimeAsync(OPTIONAL_HYDRATION_TIMEOUT_MS - 1);
    expect(result).toBeUndefined();

    await vi.advanceTimersByTimeAsync(1);
    expect(result).toBeDefined();

    rejectAgentRequest(new Error("late agent failure"));
    await Promise.resolve();
    await resultPromise;
  });

  it("warns only once when an optional request rejects after timing out", async () => {
    vi.useFakeTimers();
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    let rejectWorkspaces!: (error: Error) => void;
    mocks.listWorkspaces.mockReturnValue(
      new Promise<never>((_, reject) => {
        rejectWorkspaces = reject;
      }),
    );

    const resultPromise = fetchSessionDataForTask(TASK_ID);
    await vi.advanceTimersByTimeAsync(1);
    expect(rejectWorkspaces).toBeTypeOf("function");
    await vi.advanceTimersByTimeAsync(OPTIONAL_HYDRATION_TIMEOUT_MS - 1);
    await resultPromise;

    rejectWorkspaces(new Error("late workspace failure"));
    await vi.advanceTimersByTimeAsync(0);

    const workspaceWarnings = warn.mock.calls.filter(([message]) =>
      String(message).includes("optional workspaces"),
    );
    expect(workspaceWarnings).toHaveLength(1);
  });
});
