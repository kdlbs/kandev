import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AppState } from "@/lib/state/store";
import { sessionId, taskId, workflowId } from "@/lib/types/ids";
import type { Task, TaskSession } from "@/lib/types/http";
import type { TaskNavigationIdentity } from "@/lib/state/task-navigation-reads";
const mocks = vi.hoisted(() => ({
  listWorkflowSteps: vi.fn(),
  fetchWorkflowSnapshot: vi.fn(),
  fetchTaskSession: vi.fn(),
  listTaskSessionMessages: vi.fn(),
}));
vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  ...mocks,
  listAgents: vi.fn().mockResolvedValue({ agents: [] }),
  listRepositories: vi.fn().mockResolvedValue({ repositories: [] }),
  listWorkspaces: vi.fn().mockResolvedValue({ workspaces: [] }),
  listWorkflows: vi.fn().mockResolvedValue({ workflows: [] }),
  fetchUserSettings: vi.fn().mockResolvedValue(null),
}));
import { fetchTaskNavigationEnrichment } from "./session-page-state";
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
beforeEach(() => vi.clearAllMocks());
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
