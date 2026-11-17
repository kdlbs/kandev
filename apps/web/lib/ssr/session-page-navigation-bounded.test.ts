import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore, type AppState } from "@/lib/state/store";
import { sessionId, taskId, workflowId } from "@/lib/types/ids";
import type { Task, TaskSession } from "@/lib/types/http";
import type { TaskNavigationIdentity } from "@/lib/state/task-navigation-reads";
const mocks = vi.hoisted(() => ({
  listAgents: vi.fn(),
  listWorkflowSteps: vi.fn(),
  fetchWorkflowSnapshot: vi.fn(),
  fetchTaskSession: vi.fn(),
  listTaskSessionMessages: vi.fn(),
  listSessionTurns: vi.fn(),
}));
vi.mock("@/lib/api/domains/workflow-api", () => ({ listWorkflowSteps: mocks.listWorkflowSteps }));
vi.mock("@/lib/api/domains/session-api", () => ({ listSessionTurns: mocks.listSessionTurns }));

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  ...mocks,
  listRepositories: vi.fn().mockResolvedValue({ repositories: [] }),
  listWorkspaces: vi.fn().mockResolvedValue({ workspaces: [] }),
  listWorkflows: vi.fn().mockResolvedValue({ workflows: [] }),
  fetchUserSettings: vi.fn().mockResolvedValue(null),
}));
import { buildTaskNavigationShellData, fetchTaskNavigationEnrichment } from "./session-page-state";
const TASK_ID = "task-1";
const WORKFLOW_ID = "workflow-1";
const NOW = "2026-07-16T12:00:00Z";
function makeTask(overrides: Partial<Task> = {}): Task {
  return {
    id: taskId(TASK_ID),
    workspace_id: "workspace-1",
    workflow_id: workflowId(WORKFLOW_ID),
    title: "Task",
    description: "",
    position: 0,
    state: "TODO",
    priority: "medium",
    created_at: NOW,
    updated_at: NOW,
    ...overrides,
  } as Task;
}
function makeSession(): TaskSession {
  return {
    id: sessionId("session-1"),
    task_id: taskId(TASK_ID),
    state: "COMPLETED",
    started_at: NOW,
    updated_at: NOW,
  };
}
beforeEach(() => {
  vi.clearAllMocks();
  mocks.listAgents.mockResolvedValue({ agents: [] });
  mocks.listSessionTurns.mockResolvedValue({ turns: [], total: 0 });
});
describe("bounded task-entry enrichment", () => {
  it.each([false, true])(
    "keeps workflow tasks bounded during navigation (session=%s)",
    async (hasSession) => {
      const task = makeTask({ workflow_id: workflowId(WORKFLOW_ID), workflow_step_id: "step-1" });
      const session = makeSession();
      const identity: TaskNavigationIdentity = {
        task,
        allSessionsResponse: { sessions: hasSession ? [session] : [], total: hasSession ? 1 : 0 },
      };
      mocks.fetchTaskSession.mockResolvedValue({ session });
      mocks.listTaskSessionMessages.mockResolvedValue({
        messages: [],
        has_more: false,
        turns: [],
        turn_coverage: { complete: false, active_turn_id: null, turn_ids: [] },
      });
      mocks.listWorkflowSteps.mockResolvedValue({
        steps: [{ id: "step-1", name: "Working", position: 1 }],
      });
      mocks.fetchWorkflowSnapshot.mockResolvedValue({
        workflow: { id: WORKFLOW_ID },
        steps: [],
        tasks: Array.from({ length: 10000 }, (_, index) =>
          makeTask({ id: taskId(`other-${index}`) }),
        ),
      });

      const result = await fetchTaskNavigationEnrichment(identity);
      const state = result.initialState as Partial<AppState>;
      expect(mocks.fetchWorkflowSnapshot).not.toHaveBeenCalled();
      expect(mocks.listWorkflowSteps).toHaveBeenCalledWith(
        WORKFLOW_ID,
        expect.objectContaining({
          init: expect.objectContaining({ signal: expect.any(AbortSignal) }),
        }),
      );
      expect(state.kanban?.tasks.map((item) => item.id)).toEqual([TASK_ID]);
      expect(state.kanban?.steps.map((item) => item.id)).toEqual(["step-1"]);
      expect(state.kanban?.taskCoverage?.complete).toBe(false);
      expect(state.kanbanMulti).toBeUndefined();
    },
  );
});

describe("task-entry enrichment selection", () => {
  it.each([false, true])(
    "preserves a newer selected session while enriching an identity with sessions: %s",
    async (hasSession) => {
      const snapshotSession = makeSession();
      const identity: TaskNavigationIdentity = {
        task: makeTask(),
        allSessionsResponse: {
          sessions: hasSession ? [snapshotSession] : [],
          total: hasSession ? 1 : 0,
        },
      };
      mocks.fetchTaskSession.mockResolvedValue({ session: snapshotSession });
      mocks.listTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false });
      let resolveAgents!: (response: { agents: unknown[]; total: number }) => void;
      mocks.listAgents.mockReturnValue(
        new Promise((resolve) => {
          resolveAgents = resolve;
        }),
      );
      const store = createAppStore();
      store.getState().hydrate(buildTaskNavigationShellData(identity).initialState);
      const pending = fetchTaskNavigationEnrichment(identity);

      const preparedSession = {
        ...snapshotSession,
        id: sessionId("session-prepared"),
        state: "CREATED" as const,
      };
      const sessions = hasSession ? [snapshotSession, preparedSession] : [preparedSession];
      store.getState().setTaskSessionsForTask(TASK_ID, sessions, {});
      store.getState().setActiveSession(TASK_ID, preparedSession.id);
      store.getState().setResumeSkipped(preparedSession.id, true);
      const selection = store.getState().tasks;
      const collection = store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID];

      resolveAgents({ agents: [], total: 0 });
      const enrichment = await pending;
      store.getState().hydrate(enrichment.initialState);

      expect(store.getState().tasks).toEqual(selection);
      expect(store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID]).toEqual(collection);
    },
  );
});
