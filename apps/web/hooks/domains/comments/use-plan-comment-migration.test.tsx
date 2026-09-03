import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useCommentsStore } from "@/lib/state/slices/comments";
import { COMMENTS_STORAGE_PREFIX } from "@/lib/state/slices/comments/persistence";
import type { DiffComment, PlanComment } from "@/lib/state/slices/comments";
import type { TaskPlan, TaskPlanComment, TaskPlanCommentSnapshot } from "@/lib/types/http";

const api = vi.hoisted(() => ({
  createTaskPlanComment: vi.fn(),
  getTaskPlanComments: vi.fn(),
}));
const sessionsHook = vi.hoisted(() => ({
  sessions: [{ id: "session-1" }, { id: "session-2" }],
  isLoaded: true,
  isLoading: false,
  loadSessions: vi.fn(),
}));

vi.mock("@/lib/api/domains/plan-comment-api", () => api);
vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => sessionsHook,
}));

import { usePlanCommentMigration } from "./use-plan-comment-migration";

const TASK_ID = "task-1";
const PLAN_ID = "plan-1";
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
    anchor_to: comment.to ?? comment.selectedText.length,
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
  return <StateProvider>{children}</StateProvider>;
}

function useHarness() {
  const store = useAppStoreApi();
  const migration = usePlanCommentMigration(TASK_ID);
  return { store, migration };
}

describe("usePlanCommentMigration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.sessionStorage.clear();
    useCommentsStore.setState({
      byId: {},
      bySession: {},
      pendingForChat: [],
      editingCommentId: null,
    });
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
    writeSession("session-1", [first]);
    writeSession("session-2", [second]);
    api.createTaskPlanComment
      .mockResolvedValueOnce(snapshot([serverComment(first)], 1))
      .mockRejectedValueOnce(new Error("offline"));
    api.getTaskPlanComments.mockResolvedValue(snapshot([serverComment(first)], 1));

    const { result } = renderHook(useHarness, { wrapper });
    act(() => result.current.store.getState().setTaskPlan(TASK_ID, taskPlan));
    await waitFor(() => expect(result.current.migration.status).toBe("failed"));

    expect(readSession("session-1")).toEqual([]);
    expect(readSession("session-2")).toEqual([second]);

    const finalSnapshot = snapshot([serverComment(first), serverComment(second)], 2);
    api.createTaskPlanComment.mockResolvedValueOnce(finalSnapshot);
    api.getTaskPlanComments.mockResolvedValue(finalSnapshot);
    act(() => result.current.migration.retry());

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
});
