import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useSessionAgent } from "@/hooks/domains/session/use-session-agent";
import { createAppStore } from "@/lib/state/store";
import { sessionId, taskId } from "@/lib/types/ids";
import type { Task, TaskSession } from "@/lib/types/http";
import type { TaskNavigationIdentity } from "@/lib/state/task-navigation-reads";
import { buildTaskNavigationShellData, fetchTaskNavigationEnrichment } from "./session-page-state";

const mocks = vi.hoisted(() => ({ listAgents: vi.fn(), listTaskSessions: vi.fn() }));

vi.mock("@/lib/api", () => ({
  listAgents: mocks.listAgents,
  listTaskSessions: mocks.listTaskSessions,
  listRepositories: vi.fn().mockResolvedValue({ repositories: [] }),
  listWorkflows: vi.fn().mockResolvedValue({ workflows: [] }),
  listWorkspaces: vi.fn().mockResolvedValue({ workspaces: [] }),
  fetchUserSettings: vi.fn().mockResolvedValue(null),
}));
vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: vi.fn().mockResolvedValue({ steps: [] }),
}));

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const NOW = "2026-07-16T12:00:00Z";

function makeTask(): Task {
  return {
    id: taskId(TASK_ID),
    workspace_id: "workspace-1",
    workflow_id: "",
    workflow_step_id: "",
    position: 0,
    title: "Prepared task",
    description: "",
    state: "TODO",
    priority: "medium",
    created_at: NOW,
    updated_at: NOW,
  } as Task;
}

function makeSession(): TaskSession {
  return {
    id: sessionId(SESSION_ID),
    task_id: taskId(TASK_ID),
    state: "CREATED",
    started_at: NOW,
    updated_at: NOW,
  };
}

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.listAgents.mockResolvedValue({ agents: [], total: 0 });
  mocks.listTaskSessions.mockResolvedValue({ sessions: [], total: 0 });
});

describe("sessionless navigation enrichment", () => {
  it.each([
    ["before resolution", true],
    ["before resolution", false],
    ["before application", true],
    ["before application", false],
  ] as const)("retains new membership %s with selected=%s", async (timing, selected) => {
    const agents = Promise.withResolvers<{ agents: []; total: number }>();
    mocks.listAgents.mockReturnValue(agents.promise);
    const identity: TaskNavigationIdentity = {
      task: makeTask(),
      allSessionsResponse: { sessions: [], total: 0 },
    };
    const { result } = renderHook(
      () => ({
        store: useAppStoreApi(),
        agent: useSessionAgent(identity.task),
      }),
      { wrapper },
    );
    const store = result.current.store;
    act(() => store.getState().hydrate(buildTaskNavigationShellData(identity).initialState));
    const enrichment = fetchTaskNavigationEnrichment(identity, undefined, { store });
    await waitFor(() => expect(mocks.listAgents).toHaveBeenCalled());

    const prepared = { ...makeSession(), state: "CREATED" as const };
    const publishPrepared = () =>
      act(() => {
        store.getState().setTaskSessionsForTask(TASK_ID, [prepared], {});
        if (selected) store.getState().setActiveSessionAuto(TASK_ID, SESSION_ID);
      });
    if (timing === "before resolution") publishPrepared();
    agents.resolve({ agents: [], total: 0 });
    const enriched = await enrichment;
    if (timing === "before application") publishPrepared();
    act(() => store.getState().hydrate(enriched.initialState));

    expect(store.getState().tasks.activeSessionId).toBe(selected ? SESSION_ID : null);
    expect(store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID]).toHaveLength(1);
    expect(store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID]).toMatchObject([prepared]);
    expect(result.current.agent.taskSessionId).toBe(SESSION_ID);
  });

  it("applies recovered authoritative empty membership after an unavailable identity read", async () => {
    const identity: TaskNavigationIdentity = {
      task: makeTask(),
      allSessionsResponse: { sessions: [], total: 0 },
      sessionListUnavailable: true,
    };
    const store = createAppStore();
    store.getState().setTaskSessionsForTask(TASK_ID, [makeSession()], {});
    store.getState().setActiveSessionAuto(TASK_ID, SESSION_ID);
    mocks.listTaskSessions.mockResolvedValueOnce({ sessions: [], total: 0 });

    const enrichment = await fetchTaskNavigationEnrichment(identity, undefined, { store });
    store.getState().hydrate(enrichment.initialState);

    expect(mocks.listTaskSessions).toHaveBeenCalledTimes(1);
    expect(store.getState().tasks.activeSessionId).toBeNull();
    expect(store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID]).toEqual([]);
    expect(store.getState().taskSessionsByTask.loadedByTaskId[TASK_ID]).toBe(true);
  });
});
