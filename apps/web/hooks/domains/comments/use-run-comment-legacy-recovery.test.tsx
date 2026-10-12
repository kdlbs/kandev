import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { PlanComment } from "@/lib/state/slices/comments";
import { COMMENTS_STORAGE_PREFIX } from "@/lib/state/slices/comments/persistence";
import type { TaskPlan, TaskPlanCommentSnapshot, TaskSessionState } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";
import { planCommentMigrationFor } from "./plan-comment-migration";
import { useRunComment } from "./use-run-comment";

const api = vi.hoisted(() => ({
  createTaskPlanComment: vi.fn(),
  updateTaskPlanComment: vi.fn(),
  getTaskPlanComments: vi.fn(),
  sendMessageRequest: vi.fn(),
  queueMessage: vi.fn(),
}));
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: () => mockStore }));
vi.mock("@/lib/api/domains/plan-comment-api", () => api);
vi.mock("@/hooks/use-message-handler", () => api);
vi.mock("@/lib/api/domains/queue-api", () => ({ ...api, appendToQueue: vi.fn() }));

const TASK = taskId("task-legacy-run");
const SESSION = sessionId("session-legacy-run");
const PLAN = "plan-legacy-run";
const DATE = "2026-09-02T00:00:00Z";
const KEY = `${COMMENTS_STORAGE_PREFIX}${SESSION}`;
const legacy: PlanComment = {
  id: "legacy-selected",
  source: "plan",
  sessionId: SESSION,
  text: "Keep this feedback",
  selectedText: "Step",
  from: 1,
  to: 5,
  status: "pending",
  createdAt: DATE,
};
const persisted: PlanComment = {
  ...legacy,
  taskId: TASK,
  planId: PLAN,
  version: 1,
};
const plan: TaskPlan = {
  id: PLAN,
  task_id: TASK,
  title: "Plan",
  content: "Step",
  created_by: "agent",
  created_at: DATE,
  updated_at: DATE,
};
let mockStore: ReturnType<typeof createAppStore>;
let detachments: Array<() => void>;
let admitted: boolean;
let serverHasComment: boolean;

function serverSnapshot(): TaskPlanCommentSnapshot {
  return {
    task_id: TASK,
    plan_id: PLAN,
    revision: serverHasComment ? 1 : 2,
    comments: serverHasComment
      ? [
          {
            id: legacy.id,
            task_id: TASK,
            plan_id: PLAN,
            body: legacy.text,
            selected_text: legacy.selectedText,
            anchor_from: 1,
            anchor_to: 5,
            version: 1,
            created_at: DATE,
            updated_at: DATE,
          },
        ]
      : [],
  };
}

async function mountRecovery(state: TaskSessionState = "WAITING_FOR_INPUT") {
  mockStore = createAppStore();
  mockStore.getState().setTaskPlan(TASK, plan);
  mockStore.getState().setTaskSession({
    id: SESSION,
    task_id: TASK,
    state,
    is_primary: true,
    queue_incarnation_id: "inc-legacy-run",
    started_at: DATE,
    updated_at: DATE,
  });
  mockStore.getState().setConnectionStatus("connected");
  const recovery = planCommentMigrationFor(mockStore, TASK);
  const detach = recovery.attach(vi.fn());
  detachments.push(detach);
  recovery.update({ sessionIds: [SESSION], complete: true, loading: false });
  await vi.advanceTimersByTimeAsync(0);
  const hook = renderHook(() => useRunComment({ sessionId: SESSION, taskId: TASK }));
  return { ...hook, detach, store: mockStore };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.resetAllMocks();
  window.sessionStorage.clear();
  window.sessionStorage.setItem(KEY, JSON.stringify([legacy]));
  detachments = [];
  admitted = false;
  serverHasComment = false;
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  api.createTaskPlanComment.mockImplementation(async () => {
    // Replayed admission returns the current snapshot, never recreating a consumed UUID.
    if (!admitted) serverHasComment = true;
    admitted = true;
    return serverSnapshot();
  });
  api.getTaskPlanComments.mockImplementation(async () => serverSnapshot());
  const consume = async () => {
    serverHasComment = false;
  };
  api.sendMessageRequest.mockImplementation(consume);
  api.queueMessage.mockImplementation(consume);
});

afterEach(() => {
  cleanup();
  for (const detach of detachments) detach();
  vi.restoreAllMocks();
  vi.useRealTimers();
  window.sessionStorage.clear();
});

describe("Run during selected legacy recovery", () => {
  // @covers AC-TASKS-PLAN-COMMENTS-004.3, AC-TASKS-PLAN-COMMENTS-004.7
  it.each(["WAITING_FOR_INPUT", "RUNNING"] as const)(
    "waits for selected cleanup before Run and reload in %s",
    async (state) => {
      const removal = vi.spyOn(window.sessionStorage, "removeItem").mockImplementation(() => {
        throw new DOMException("denied", "SecurityError");
      });
      const first = await mountRecovery(state);
      expect(first.store.getState().taskPlans.commentsByTaskId[TASK]?.comments).toHaveLength(1);
      await expect(first.result.current.runComment(persisted)).rejects.toMatchObject({
        code: "plan-comment-not-persisted",
      });
      expect(api.sendMessageRequest).not.toHaveBeenCalled();
      expect(api.queueMessage).not.toHaveBeenCalled();
      expect(JSON.parse(window.sessionStorage.getItem(KEY)!)).toEqual([legacy]);
      first.unmount();
      first.detach();
      removal.mockRestore();

      const reloaded = await mountRecovery(state);
      expect(window.sessionStorage.getItem(KEY)).toBeNull();
      await expect(reloaded.result.current.runComment(persisted)).resolves.toEqual({
        queued: state === "RUNNING",
      });
      expect(api.sendMessageRequest.mock.calls.length + api.queueMessage.mock.calls.length).toBe(1);
      expect(serverHasComment).toBe(false);
      reloaded.unmount();
      reloaded.detach();

      const afterRun = await mountRecovery(state);
      expect(api.createTaskPlanComment).toHaveBeenCalledTimes(2);
      expect(afterRun.store.getState().taskPlans.commentsMigrationByTaskId[TASK]).toMatchObject({
        status: "complete",
        pendingCount: 0,
      });
      expect(window.sessionStorage.getItem(KEY)).toBeNull();
    },
  );

  it("allows Run once its own row is cleaned while another legacy row is pending", async () => {
    const other = { ...legacy, id: "legacy-other" };
    window.sessionStorage.setItem(KEY, JSON.stringify([legacy, other]));
    const create = api.createTaskPlanComment.getMockImplementation()!;
    api.createTaskPlanComment.mockImplementation(async (input: { id: string }) => {
      if (input.id === other.id) throw new Error("temporarily unavailable");
      return create(input);
    });
    const { store, result } = await mountRecovery();
    expect(store.getState().taskPlans.commentsMigrationByTaskId[TASK]?.pendingCount).toBe(1);
    await expect(result.current.runComment(persisted)).resolves.toEqual({ queued: false });
    expect(api.sendMessageRequest).toHaveBeenCalledOnce();
    expect(JSON.parse(window.sessionStorage.getItem(KEY)!)).toEqual([other]);
    expect(store.getState().taskPlans.commentsMigrationByTaskId[TASK]?.pendingCount).toBe(1);
  });
});
