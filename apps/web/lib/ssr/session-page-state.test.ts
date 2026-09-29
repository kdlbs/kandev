import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, useEffect, useState, type ReactNode } from "react";

import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { mergeInitialState } from "@/lib/state/default-state";
import { createAppStore } from "@/lib/state/store";
import type { AppState } from "@/lib/state/store";
import { sessionId, taskId, workflowId } from "@/lib/types/ids";
import type { Task, TaskSession, Turn } from "@/lib/types/http";
import type { TaskNavigationIdentity } from "@/lib/state/task-navigation-reads";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import {
  clearInFlightTurnsLoadForTest,
  ensureSessionTurnsLoaded,
} from "@/hooks/domains/session/use-session-turns-hydration";

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

import {
  buildTaskNavigationShellData,
  fetchSessionDataForTask,
  fetchTaskNavigationData,
  fetchTaskNavigationEnrichment,
  OPTIONAL_HYDRATION_TIMEOUT_MS,
} from "./session-page-state";

const NOW = "2026-07-16T12:00:00Z";
const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const WORKSPACE_ID = "workspace-office";
const WORKFLOW_ID = "workflow-1";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

/** Builds a Task with default test fields, merged with the given overrides. */
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

/** Resets mocks and seeds every API mock with its default hydration response. */
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

/** Builds a completed session object for the test task and session ids. */
function makeSession(): TaskSession {
  return {
    id: sessionId(SESSION_ID),
    task_id: taskId(TASK_ID),
    state: "COMPLETED",
    started_at: NOW,
    updated_at: NOW,
  };
}

function makeTurn(id: string, overrides: Partial<Turn> = {}): Turn {
  return {
    id,
    session_id: sessionId(SESSION_ID),
    task_id: taskId(TASK_ID),
    started_at: NOW,
    created_at: NOW,
    updated_at: NOW,
    metadata: { runtime_config_snapshot: { model: "mock-fast" } },
    ...overrides,
  };
}

beforeEach(mockDefaultHydrationData);

afterEach(() => {
  vi.restoreAllMocks();
  clearInFlightTurnsLoadForTest();
  vi.useRealTimers();
});

describe("progressive task navigation state", () => {
  it("seeds a sessionless destination without waiting for optional resources", () => {
    const identity: TaskNavigationIdentity = {
      task: makeTask(),
      allSessionsResponse: { sessions: [], total: 0 },
    };

    const result = buildTaskNavigationShellData(identity);
    const state = result.initialState as unknown as Partial<AppState>;

    expect(result.task).toBe(identity.task);
    expect(result.sessionId).toBeNull();
    expect(state.tasks?.activeTaskId).toBe(TASK_ID);
    expect(state.tasks?.activeSessionId).toBeNull();
    expect(state.taskSessionsByTask?.loadedByTaskId?.[TASK_ID]).toBe(true);
    expect(state.messages).toBeUndefined();
  });

  it("enriches an existing identity without repeating task or session-list reads", async () => {
    const session = makeSession();
    const identity: TaskNavigationIdentity = {
      task: makeTask(),
      allSessionsResponse: { sessions: [session], total: 1 },
    };
    mocks.fetchTaskSession.mockResolvedValue({ session });
    mocks.listTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false });
    mocks.listSessionTurns.mockResolvedValue({ turns: [], total: 0 });

    const result = await fetchTaskNavigationEnrichment(identity);

    expect(result.task).toBe(identity.task);
    expect(result.sessionId).toBe(SESSION_ID);
    expect(mocks.fetchTask).not.toHaveBeenCalled();
    expect(mocks.listTaskSessions).not.toHaveBeenCalled();
  });

  it("lets route enrichment join the settings agent read", async () => {
    let resolveAgents!: (response: { agents: unknown[]; total: number }) => void;
    mocks.listAgents.mockReturnValue(
      new Promise((resolve) => {
        resolveAgents = resolve;
      }),
    );
    const identity: TaskNavigationIdentity = {
      task: makeTask(),
      allSessionsResponse: { sessions: [], total: 0 },
    };

    const { result } = renderHook(
      () => {
        useSettingsData();
        const store = useAppStoreApi();
        const profiles = useAppStore((state) => state.agentProfiles.items);
        const [routeEnriched, setRouteEnriched] = useState(false);
        useEffect(() => {
          void fetchTaskNavigationEnrichment(identity, undefined, { store }).then(() =>
            setRouteEnriched(true),
          );
        }, [store]);
        return { profiles, routeEnriched };
      },
      { wrapper },
    );

    await act(async () => {
      await Promise.resolve();
    });
    expect(mocks.listAgents).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveAgents({
        agents: [
          {
            id: "agent-route",
            name: "Route agent",
            profiles: [{ id: "profile-route", agentDisplayName: "Route agent", name: "Current" }],
          },
        ],
        total: 1,
      });
    });
    await waitFor(() => expect(result.current.routeEnriched).toBe(true));
    expect(result.current.profiles.map((profile) => profile.id)).toEqual(["profile-route"]);
  });
});

describe("task navigation shell turn hydration", () => {
  it("lets authoritative enrichment replace the shell without losing live turns", async () => {
    const session = makeSession();
    const identity: TaskNavigationIdentity = {
      task: makeTask(),
      allSessionsResponse: { sessions: [session], total: 1 },
    };
    const historicalTurn = makeTurn("turn-history", { completed_at: NOW });
    mocks.fetchTaskSession.mockResolvedValue({ session: null });
    mocks.listTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false });
    mocks.listSessionTurns.mockResolvedValue({ turns: [historicalTurn], total: 1 });

    const shell = buildTaskNavigationShellData(identity);
    const store = createAppStore();
    store.getState().hydrate(shell.initialState);
    expect(store.getState().turns.loadedBySession[SESSION_ID]).toBeUndefined();
    expect(store.getState().turns.bySession[SESSION_ID]).toBeUndefined();

    const enrichment = await fetchTaskNavigationEnrichment(identity);
    store.getState().hydrate(enrichment.initialState);
    expect(store.getState().turns.loadedBySession[SESSION_ID]).toBe(true);
    expect(store.getState().turns.bySession[SESSION_ID]?.[0]?.metadata).toEqual(
      historicalTurn.metadata,
    );

    store.getState().addTurn({
      ...historicalTurn,
      id: "turn-live",
      session_id: sessionId(SESSION_ID),
      task_id: taskId(TASK_ID),
      completed_at: undefined,
      started_at: "2026-07-16T12:01:00Z",
    } as Turn);
    store.getState().hydrate(enrichment.initialState);
    expect(store.getState().turns.bySession[SESSION_ID]?.map((turn) => turn.id)).toEqual([
      "turn-history",
      "turn-live",
    ]);
  });

  it("keeps failed shell enrichment retryable through the normal turn loader", async () => {
    const session = makeSession();
    const identity: TaskNavigationIdentity = {
      task: makeTask(),
      allSessionsResponse: { sessions: [session], total: 1 },
    };
    mocks.fetchTaskSession.mockResolvedValue({ session: null });
    mocks.listTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false });
    mocks.listSessionTurns
      .mockRejectedValueOnce(new Error("turns unavailable"))
      .mockResolvedValueOnce({
        turns: [makeTurn("turn-retry", { metadata: { recovered: true } })],
        total: 1,
      });

    const shell = buildTaskNavigationShellData(identity);
    const store = createAppStore();
    store.getState().hydrate(shell.initialState);
    const enrichment = await fetchTaskNavigationEnrichment(identity);
    store.getState().hydrate(enrichment.initialState);
    expect(store.getState().turns.loadedBySession[SESSION_ID]).toBeUndefined();

    await expect(ensureSessionTurnsLoaded(SESSION_ID, store as never)).resolves.toBe(true);
    expect(mocks.listSessionTurns).toHaveBeenCalledTimes(2);
    expect(store.getState().turns.loadedBySession[SESSION_ID]).toBe(true);
    expect(store.getState().turns.bySession[SESSION_ID]?.[0]?.metadata).toEqual({
      recovered: true,
    });
  });
});

describe("fetchSessionDataForTask initial hydration", () => {
  it("preserves the Office workflow ID in task-route state", async () => {
    const result = await fetchSessionDataForTask(TASK_ID);
    const initialState = result.initialState as unknown as Partial<AppState>;

    expect(initialState.workspaces?.items[0]?.office_workflow_id).toBe("workflow-office");
  });

  it("hydrates the requested session only when it belongs to the route task", async () => {
    const primary = makeSession();
    const requested = { ...makeSession(), id: sessionId("session-requested") };
    mocks.fetchTask.mockResolvedValue(makeTask({ primary_session_id: sessionId(primary.id) }));
    mocks.listTaskSessions.mockResolvedValue({ sessions: [primary, requested], total: 2 });
    mocks.fetchTaskSession.mockResolvedValue({ session: requested });

    const result = await fetchSessionDataForTask(TASK_ID, requested.id);

    expect(mocks.fetchTaskSession).toHaveBeenCalledWith(requested.id, { cache: "no-store" });
    expect(result.sessionId).toBe(requested.id);
  });

  it("rejects a requested session that is not in the route task's session list", async () => {
    const primary = makeSession();
    mocks.fetchTask.mockResolvedValue(makeTask({ primary_session_id: sessionId(primary.id) }));
    mocks.listTaskSessions.mockResolvedValue({
      sessions: [
        primary,
        { ...makeSession(), id: sessionId("other-task-session"), task_id: "task-other" },
      ],
      total: 2,
    });
    mocks.fetchTaskSession.mockResolvedValue({ session: primary });

    const result = await fetchSessionDataForTask(TASK_ID, sessionId("other-task-session"));

    expect(mocks.fetchTaskSession).toHaveBeenCalledWith(primary.id, { cache: "no-store" });
    expect(result.sessionId).toBe(primary.id);
  });
});

describe("fetchSessionDataForTask per-turn hydration", () => {
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
    // SSR-hydrated turn lists are complete snapshots: the session must be
    // marked loaded so turn-derived UI never mistakes WS-seeded live turns
    // for the full history or re-fetches unnecessarily.
    expect(hydratedState.turns.loadedBySession[session.id]).toBe(true);
  });

  it("leaves the hydration marker absent when the SSR turns fetch fails", async () => {
    // A rejected optional turns hydration must not mark the session loaded,
    // so the client hook can fetch turns on first render.
    const session = makeSession();
    mocks.listTaskSessions.mockResolvedValue({ sessions: [session], total: 1 });
    mocks.fetchTaskSession.mockResolvedValue({ session });
    mocks.listSessionTurns.mockRejectedValue(new Error("turns unavailable"));
    mocks.listTaskSessionMessages.mockResolvedValue(null);

    const result = await fetchSessionDataForTask(TASK_ID);
    const hydratedState = mergeInitialState(result.initialState);

    expect(hydratedState.turns.bySession[SESSION_ID]).toBeUndefined();
    expect(hydratedState.turns.loadedBySession[SESSION_ID]).toBeUndefined();
    expect(hydratedState.turns.loadedBySession).toEqual({});
  });
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

describe("fetchTaskNavigationData", () => {
  it("hydrates only essential task/session state without optional boot requests", async () => {
    const session = makeSession();
    mocks.listTaskSessions.mockResolvedValue({ sessions: [session] });
    const result = await fetchTaskNavigationData(TASK_ID);
    expect(result.sessionId).toBe(SESSION_ID);
    expect(result.initialState.taskSessions?.items[SESSION_ID]).toEqual(session);
    expect(result.initialState.taskSessionsByTask?.loadedByTaskId[TASK_ID]).toBe(true);
    expect(result.initialState.messages).toBeUndefined();
    expect(result.initialState.turns).toBeUndefined();
    for (const [name, mock] of Object.entries(mocks)) {
      if (name === "fetchTask" || name === "listTaskSessions") continue;
      expect(mock, name).not.toHaveBeenCalled();
    }
  });

  it("ignores a requested session owned by another task", async () => {
    const session = makeSession();
    mocks.listTaskSessions.mockResolvedValue({
      sessions: [{ ...session, id: "foreign-session", task_id: "foreign-task" }, session],
    });
    const result = await fetchTaskNavigationData(TASK_ID, "foreign-session");
    expect(result.sessionId).toBe(SESSION_ID);
    expect(Object.keys(result.initialState.taskSessions!.items)).toEqual([SESSION_ID]);
  });

  it("keeps a sessionless task sessionless without waiting for enrichment", async () => {
    const result = await fetchTaskNavigationData(TASK_ID);
    expect(result.sessionId).toBeNull();
    expect(result.initialState.taskSessionsByTask?.itemsByTaskId[TASK_ID]).toEqual([]);
    expect(result.initialState.messages).toBeUndefined();
    expect(mocks.listAgents).not.toHaveBeenCalled();
  });
});
