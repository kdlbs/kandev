import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useCommentsStore } from "@/lib/state/slices/comments";
import { COMMENTS_STORAGE_PREFIX } from "@/lib/state/slices/comments/persistence";
import type { DiffComment, PlanComment } from "@/lib/state/slices/comments";
import type { TaskPlan, TaskPlanComment, TaskPlanCommentSnapshot } from "@/lib/types/http";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import { WebSocketRequestError } from "@/lib/ws/request-error";

const api = vi.hoisted(() => ({
  createTaskPlanComment: vi.fn(),
  updateTaskPlanComment: vi.fn(),
  getTaskPlanComments: vi.fn(),
}));
const sessionsHook = vi.hoisted(() => ({
  sessions: [{ id: "session-1" }, { id: "session-2" }],
  isLoaded: true,
  isLoading: false,
  error: null as string | null,
  loadSessions: vi.fn(),
}));

vi.mock("@/lib/api/domains/plan-comment-api", () => api);
vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => sessionsHook,
}));

import { usePlanCommentMigration } from "./use-plan-comment-migration";

const TASK_ID = "task-1";
const PLAN_ID = "plan-1";
const FOREIGN_SESSION = "foreign-session";
const PLAN_TIMESTAMP = "2026-09-02T00:00:00Z";

const taskPlan: TaskPlan = {
  id: PLAN_ID,
  task_id: TASK_ID,
  title: "Plan",
  content: "# Plan",
  created_by: "agent",
  created_at: PLAN_TIMESTAMP,
  updated_at: PLAN_TIMESTAMP,
};

function legacyPlanComment(id: string, sessionId: string, text = "Keep this"): PlanComment {
  return {
    id,
    sessionId,
    source: "plan",
    text,
    selectedText: "Plan step",
    from: 2,
    to: 11,
    createdAt: PLAN_TIMESTAMP,
    status: "pending",
  };
}

function diffComment(): DiffComment {
  return {
    id: "diff-1",
    sessionId: "session-1",
    source: "diff",
    text: "Fix this",
    filePath: "src/app.ts",
    startLine: 1,
    endLine: 1,
    side: "additions",
    codeContent: "code",
    createdAt: PLAN_TIMESTAMP,
    status: "pending",
  };
}

function serverComment(comment: PlanComment, version = 1): TaskPlanComment {
  return {
    id: comment.id,
    task_id: TASK_ID,
    plan_id: PLAN_ID,
    body: comment.text,
    selected_text: comment.selectedText,
    anchor_from: comment.from ?? 0,
    anchor_to: comment.to ?? (comment.from ?? 0) + Math.max(1, comment.selectedText.length),
    version,
    created_at: comment.createdAt,
    updated_at: comment.createdAt,
  };
}

function snapshot(
  comments: TaskPlanComment[],
  revision = comments.length,
): TaskPlanCommentSnapshot {
  return { task_id: TASK_ID, plan_id: PLAN_ID, revision, comments };
}

function writeSession(sessionId: string, values: unknown[]) {
  window.sessionStorage.setItem(`${COMMENTS_STORAGE_PREFIX}${sessionId}`, JSON.stringify(values));
}

function readSession(sessionId: string): unknown[] {
  return JSON.parse(
    window.sessionStorage.getItem(`${COMMENTS_STORAGE_PREFIX}${sessionId}`) ?? "[]",
  );
}

function wrapper({ children }: { children: React.ReactNode }) {
  return (
    <StateProvider
      initialState={{ connection: { status: "connected", error: null, issueSeverity: "none" } }}
    >
      {children}
    </StateProvider>
  );
}

function useHarness() {
  const store = useAppStoreApi();
  const migration = usePlanCommentMigration(TASK_ID);
  return { store, migration };
}

// eslint-disable-next-line max-lines-per-function -- Migration recovery cases share browser-storage and store fixtures.
describe("usePlanCommentMigration", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    sessionsHook.sessions = [{ id: "session-1" }, { id: "session-2" }];
    sessionsHook.isLoaded = true;
    sessionsHook.error = null;
    sessionsHook.loadSessions.mockResolvedValue(undefined);
    window.sessionStorage.clear();
    useCommentsStore.setState({
      byId: {},
      bySession: {},
      pendingForChat: [],
      editingCommentId: null,
    });
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("does not request a migration refresh or block Send when storage is empty", async () => {
    api.getTaskPlanComments.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(useHarness, { wrapper });
    expect(result.current.migration.isBlocking).toBe(false);
    act(() => result.current.store.getState().setTaskPlan(TASK_ID, taskPlan));

    await waitFor(() => expect(result.current.migration.status).toBe("complete"));
    expect(api.getTaskPlanComments).not.toHaveBeenCalled();
    expect(api.createTaskPlanComment).not.toHaveBeenCalled();
    expect(result.current.migration.isBlocking).toBe(false);
  });

  it("automatically retries a transient legacy upload without manual Retry", async () => {
    vi.useFakeTimers();
    const comment = legacyPlanComment("comment-1", "session-1");
    writeSession("session-1", [comment]);
    api.createTaskPlanComment
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(snapshot([serverComment(comment)]));
    api.getTaskPlanComments.mockResolvedValue(snapshot([serverComment(comment)]));
    const { result } = renderHook(useHarness, { wrapper });
    await act(async () => result.current.store.getState().setTaskPlan(TASK_ID, taskPlan));

    expect(readSession("session-1")).toEqual([comment]);
    expect(result.current.migration.isBlocking).toBe(true);
    await act(async () => vi.advanceTimersByTimeAsync(1000));

    expect(result.current.migration.status).toBe("complete");
    expect(api.createTaskPlanComment).toHaveBeenCalledTimes(2);
    expect(readSession("session-1")).toEqual([]);
  });

  it("migrates every known session row by UUID and preserves non-plan records", async () => {
    const first = legacyPlanComment("comment-1", "session-1");
    const second = legacyPlanComment("comment-2", "session-2");
    const diff = diffComment();
    writeSession("session-1", [first, diff]);
    writeSession("session-2", [second]);
    useCommentsStore.getState().hydrateSession("session-1");
    useCommentsStore.getState().hydrateSession("session-2");
    const finalSnapshot = snapshot([serverComment(first), serverComment(second)], 2);
    api.createTaskPlanComment
      .mockResolvedValueOnce(snapshot([serverComment(first)], 1))
      .mockResolvedValueOnce(finalSnapshot);
    api.getTaskPlanComments.mockResolvedValue(finalSnapshot);

    const { result } = renderHook(useHarness, { wrapper });
    act(() => result.current.store.getState().setTaskPlan(TASK_ID, taskPlan));

    await waitFor(() => expect(result.current.migration.status).toBe("complete"));

    expect(api.createTaskPlanComment.mock.calls.map(([input]) => input.id)).toEqual([
      "comment-1",
      "comment-2",
    ]);
    expect(api.createTaskPlanComment).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ id: first.id, body: first.text, selectedText: first.selectedText }),
    );
    expect(readSession("session-1")).toEqual([diff]);
    expect(readSession("session-2")).toEqual([]);
    expect(useCommentsStore.getState().byId).toEqual({ "diff-1": diff });
    expect(result.current.store.getState().taskPlans.commentsByTaskId[TASK_ID]).toEqual(
      finalSnapshot,
    );
  });

  it("preserves failed rows and retries only what remains", async () => {
    const first = legacyPlanComment("comment-1", "session-1");
    const second = legacyPlanComment("comment-2", "session-2");
    const diff = diffComment();
    writeSession("session-1", [first, diff]);
    writeSession("session-2", [second]);
    api.createTaskPlanComment
      .mockResolvedValueOnce(snapshot([serverComment(first)], 1))
      .mockRejectedValueOnce(new Error("offline"));
    api.getTaskPlanComments.mockResolvedValue(snapshot([serverComment(first)], 1));

    const { result } = renderHook(useHarness, { wrapper });
    act(() => result.current.store.getState().setTaskPlan(TASK_ID, taskPlan));
    await waitFor(() => expect(result.current.migration.status).toBe("retrying"));

    expect(readSession("session-1")).toEqual([diff]);
    expect(readSession("session-2")).toEqual([second]);

    const finalSnapshot = snapshot([serverComment(first), serverComment(second)], 2);
    api.createTaskPlanComment.mockResolvedValueOnce(finalSnapshot);
    api.getTaskPlanComments.mockResolvedValue(finalSnapshot);
    await act(async () => result.current.migration.retry());

    await waitFor(() => expect(result.current.migration.status).toBe("complete"));
    expect(api.createTaskPlanComment.mock.calls.map(([input]) => input.id)).toEqual([
      "comment-1",
      "comment-2",
      "comment-2",
    ]);
    expect(readSession("session-2")).toEqual([]);
  });

  it("keeps legacy rows when the task has no current plan", async () => {
    const comment = legacyPlanComment("comment-1", "session-1");
    writeSession("session-1", [comment]);

    const { result } = renderHook(useHarness, { wrapper });
    act(() => result.current.store.getState().setTaskPlan(TASK_ID, null));

    await waitFor(() => expect(result.current.migration.status).toBe("waiting_for_plan"));
    expect(api.createTaskPlanComment).not.toHaveBeenCalled();
    expect(readSession("session-1")).toEqual([comment]);
  });

  it("derives a missing legacy anchor end from its start", async () => {
    const comment = { ...legacyPlanComment("comment-1", "session-1"), from: 100, to: undefined };
    writeSession("session-1", [comment]);
    api.createTaskPlanComment.mockResolvedValue(snapshot([serverComment(comment)]));
    api.getTaskPlanComments.mockResolvedValue(snapshot([serverComment(comment)]));

    const { result } = renderHook(useHarness, { wrapper });
    act(() => result.current.store.getState().setTaskPlan(TASK_ID, taskPlan));

    await waitFor(() => expect(result.current.migration.status).toBe("complete"));
    expect(api.createTaskPlanComment).toHaveBeenCalledWith(
      expect.objectContaining({ anchorFrom: 100, anchorTo: 109 }),
    );
  });

  it("preserves a legacy row when its UUID belongs to different server content", async () => {
    const legacy = legacyPlanComment("comment-1", "session-1", "old body");
    const authoritative = serverComment({ ...legacy, text: "edited remotely" }, 2);
    const authoritativeSnapshot = snapshot([authoritative], 2);
    writeSession("session-1", [legacy]);
    api.createTaskPlanComment.mockRejectedValue(
      new WebSocketRequestError("Task plan comments changed", "plan_comments_changed", {
        snapshot: authoritativeSnapshot,
      }),
    );
    api.getTaskPlanComments.mockResolvedValue(authoritativeSnapshot);

    const { result } = renderHook(useHarness, { wrapper });
    act(() => result.current.store.getState().setTaskPlan(TASK_ID, taskPlan));

    await waitFor(() => expect(result.current.migration.status).toBe("failed"));
    expect(readSession("session-1")).toEqual([legacy]);
    expect(result.current.store.getState().taskPlans.commentsByTaskId[TASK_ID]).toEqual(
      authoritativeSnapshot,
    );
  });

  it("does not block plain Send when session discovery fails without identified drafts", async () => {
    sessionsHook.isLoaded = false;
    sessionsHook.error = "offline";
    const { result, rerender } = renderHook(useHarness, { wrapper });
    act(() => result.current.store.getState().setTaskPlan(TASK_ID, taskPlan));

    expect(result.current.migration.isBlocking).toBe(false);
    sessionsHook.error = null;
    sessionsHook.isLoaded = true;
    rerender();

    await waitFor(() => expect(result.current.migration.status).toBe("complete"));
  });

  it("protects already-hydrated task drafts when the session list is unavailable", async () => {
    sessionsHook.sessions = [];
    sessionsHook.isLoaded = false;
    sessionsHook.error = "offline";
    const legacy = legacyPlanComment("comment-1", "session-1");
    const unrelated = legacyPlanComment("comment-2", FOREIGN_SESSION);
    writeSession("session-1", [legacy]);
    writeSession(FOREIGN_SESSION, [unrelated]);
    const { result } = renderHook(useHarness, { wrapper });
    act(() => {
      result.current.store.getState().setConnectionStatus("disconnected");
      for (const [id, task_id] of [
        ["session-1", TASK_ID],
        [FOREIGN_SESSION, "other-task"],
      ]) {
        result.current.store.getState().setTaskSession({
          id: toSessionId(id),
          task_id: toTaskId(task_id),
          state: "WAITING_FOR_INPUT",
          started_at: PLAN_TIMESTAMP,
          updated_at: PLAN_TIMESTAMP,
        });
      }
    });
    expect(
      result.current.store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID],
    ).toBeUndefined();
    expect(result.current.migration.pendingCount).toBe(1);
    expect(result.current.migration.isBlocking).toBe(true);
    expect(readSession("session-1")).toEqual([legacy]);
    expect(readSession(FOREIGN_SESSION)).toEqual([unrelated]);
  });

  it("failed plan discovery does not block plain Send before or after a null plan", async () => {
    const { result } = renderHook(useHarness, { wrapper });
    act(() => result.current.store.getState().setTaskPlanCommentsError(TASK_ID, "offline"));

    expect(result.current.migration.isBlocking).toBe(false);
    act(() => result.current.store.getState().setTaskPlan(TASK_ID, null));
    expect(result.current.migration.isBlocking).toBe(false);
  });

  it("successful empty snapshot recovery leaves no obsolete migration restriction", async () => {
    const { result } = renderHook(useHarness, { wrapper });
    act(() => result.current.store.getState().setTaskPlanCommentsError(TASK_ID, "offline"));
    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, taskPlan);
      result.current.store.getState().setTaskPlanComments(TASK_ID, snapshot([]));
    });
    await waitFor(() => expect(result.current.migration.isBlocking).toBe(false));
    expect(api.createTaskPlanComment).not.toHaveBeenCalled();
  });
});
