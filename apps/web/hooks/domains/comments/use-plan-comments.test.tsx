import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { TaskPlan, TaskPlanCommentSnapshot } from "@/lib/types/http";

const api = vi.hoisted(() => ({
  getTaskPlanComments: vi.fn(),
  createTaskPlanComment: vi.fn(),
  updateTaskPlanComment: vi.fn(),
  deleteTaskPlanComment: vi.fn(),
}));
const planApi = vi.hoisted(() => ({ getTaskPlan: vi.fn() }));

vi.mock("@/lib/api/domains/plan-comment-api", () => api);
vi.mock("@/lib/api/domains/plan-api", () => planApi);

import { usePlanComments } from "./use-plan-comments";

const TASK_ID = "task-1";
const PLAN_ID = "plan-1";
const PLAN_TIMESTAMP = "2026-09-02T00:00:00Z";
const NEW_FEEDBACK = "new feedback";

const taskPlan: TaskPlan = {
  id: PLAN_ID,
  task_id: TASK_ID,
  title: "Plan",
  content: "# Plan",
  created_by: "agent",
  created_at: PLAN_TIMESTAMP,
  updated_at: PLAN_TIMESTAMP,
};

function snapshot(revision = 1, body = "shared feedback"): TaskPlanCommentSnapshot {
  return {
    task_id: TASK_ID,
    plan_id: PLAN_ID,
    revision,
    comments: [
      {
        id: "comment-1",
        task_id: TASK_ID,
        plan_id: PLAN_ID,
        body,
        selected_text: "selected",
        anchor_from: 2,
        anchor_to: 7,
        version: revision,
        created_at: PLAN_TIMESTAMP,
        updated_at: PLAN_TIMESTAMP,
      },
    ],
  };
}

function wrapper({ children }: { children: React.ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

function useTwoTaskCommentConsumers() {
  const store = useAppStoreApi();
  const first = usePlanComments(TASK_ID);
  const second = usePlanComments(TASK_ID);
  return { store, first, second };
}

function setUpApis() {
  vi.clearAllMocks();
  planApi.getTaskPlan.mockResolvedValue(taskPlan);
  api.getTaskPlanComments.mockResolvedValue(snapshot());
}

// eslint-disable-next-line max-lines-per-function -- Loading and reconnect cases share one task-plan store fixture.
describe("usePlanComments loading", () => {
  beforeEach(setUpApis);

  it("loads the current plan before comments when only a task composer is mounted", async () => {
    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return { store, comments: usePlanComments(TASK_ID) };
      },
      { wrapper },
    );
    act(() => {
      result.current.store.setState({
        connection: { status: "connected", error: null, issueSeverity: "none" },
      });
    });

    await waitFor(() => expect(result.current.comments.comments).toHaveLength(1));
    await waitFor(() => expect(result.current.comments.isLoading).toBe(false));

    expect(planApi.getTaskPlan).toHaveBeenCalledWith(TASK_ID);
    expect(api.getTaskPlanComments).toHaveBeenCalledWith(TASK_ID);
  });

  it("deduplicates loading and projects one task snapshot into every consumer", async () => {
    const { result } = renderHook(useTwoTaskCommentConsumers, { wrapper });
    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, taskPlan);
      result.current.store.setState({
        connection: { status: "connected", error: null, issueSeverity: "none" },
      });
    });

    await waitFor(() => expect(result.current.first.comments).toHaveLength(1));
    await waitFor(() => expect(result.current.first.isLoading).toBe(false));

    expect(api.getTaskPlanComments).toHaveBeenCalledTimes(1);
    expect(result.current.first.comments).toEqual(result.current.second.comments);
    expect(result.current.first.comments[0]).toMatchObject({
      taskId: TASK_ID,
      planId: PLAN_ID,
      text: "shared feedback",
      version: 1,
    });
  });

  it("refreshes an already-loaded snapshot when the WebSocket reconnects", async () => {
    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return { store, comments: usePlanComments(TASK_ID) };
      },
      { wrapper },
    );
    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, taskPlan);
      result.current.store.getState().setTaskPlanComments(TASK_ID, snapshot());
      result.current.store.setState({
        connection: { status: "connected", error: null, issueSeverity: "none" },
      });
    });

    await waitFor(() => expect(api.getTaskPlanComments).toHaveBeenCalledWith(TASK_ID));
    expect(result.current.comments.comments).toHaveLength(1);
  });

  it("authoritatively discovers a plan created while disconnected", async () => {
    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return { store, comments: usePlanComments(TASK_ID) };
      },
      { wrapper },
    );
    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, null);
      result.current.store.setState({
        connection: { status: "disconnected", error: null, issueSeverity: "none" },
      });
    });

    act(() => {
      result.current.store.setState({
        connection: { status: "connected", error: null, issueSeverity: "none" },
      });
    });

    await waitFor(() => expect(result.current.comments.comments).toHaveLength(1));
    expect(planApi.getTaskPlan).toHaveBeenCalledWith(TASK_ID);
  });

  it("replaces a stale cached plan before loading reconnect comments", async () => {
    const replacement = { ...taskPlan, id: "plan-2", content: "# Replacement" };
    const replacementSnapshot = { ...snapshot(2), plan_id: "plan-2" };
    replacementSnapshot.comments = replacementSnapshot.comments.map((comment) => ({
      ...comment,
      plan_id: "plan-2",
      body: "replacement feedback",
    }));
    planApi.getTaskPlan.mockResolvedValue(replacement);
    api.getTaskPlanComments.mockResolvedValue(replacementSnapshot);
    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return { store, comments: usePlanComments(TASK_ID) };
      },
      { wrapper },
    );
    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, taskPlan);
      result.current.store.getState().setTaskPlanComments(TASK_ID, snapshot());
      result.current.store.setState({
        connection: { status: "disconnected", error: null, issueSeverity: "none" },
      });
    });

    act(() => {
      result.current.store.setState({
        connection: { status: "connected", error: null, issueSeverity: "none" },
      });
    });

    await waitFor(() =>
      expect(result.current.comments.comments[0]?.text).toBe("replacement feedback"),
    );
    expect(result.current.store.getState().taskPlans.byTaskId[TASK_ID]?.id).toBe("plan-2");
  });

  it("does not let a stale plan fetch clear the replacement plan loading state", async () => {
    let rejectOld!: (error: Error) => void;
    api.getTaskPlanComments.mockImplementationOnce(
      () => new Promise((_resolve, reject) => (rejectOld = reject)),
    );
    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return { store, comments: usePlanComments(TASK_ID) };
      },
      { wrapper },
    );
    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, taskPlan);
      result.current.store.setState({
        connection: { status: "connected", error: null, issueSeverity: "none" },
      });
    });
    await waitFor(() => expect(result.current.comments.isLoading).toBe(true));
    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, { ...taskPlan, id: "plan-2" });
      result.current.store.getState().setTaskPlanCommentsLoading(TASK_ID, true);
      rejectOld(new Error("old plan failed"));
    });

    await waitFor(() => {
      expect(result.current.store.getState().taskPlans.commentsLoadingByTaskId[TASK_ID]).toBe(true);
      expect(
        result.current.store.getState().taskPlans.commentsErrorByTaskId[TASK_ID],
      ).toBeUndefined();
    });
  });
});

// eslint-disable-next-line max-lines-per-function -- Optimistic concurrency cases share one task-plan store fixture.
describe("usePlanComments mutations", () => {
  beforeEach(setUpApis);

  it("awaits create acknowledgement before exposing the new snapshot", async () => {
    api.createTaskPlanComment.mockImplementation(async ({ id }: { id: string }) => {
      const created = snapshot(2, NEW_FEEDBACK);
      created.comments[0]!.id = id;
      return created;
    });
    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return { store, comments: usePlanComments(TASK_ID) };
      },
      { wrapper },
    );
    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, taskPlan);
      result.current.store.getState().setTaskPlanComments(TASK_ID, snapshot());
    });

    await act(async () => {
      await expect(
        result.current.comments.handleAddComment(NEW_FEEDBACK, "selection", 3, 8),
      ).resolves.toMatchObject({ version: 2, text: NEW_FEEDBACK });
    });

    expect(api.createTaskPlanComment).toHaveBeenCalledWith(
      expect.objectContaining({ taskId: TASK_ID, planId: PLAN_ID, body: NEW_FEEDBACK }),
    );
    expect(result.current.comments.comments[0]?.text).toBe(NEW_FEEDBACK);
    expect(result.current.comments.mutationError).toBeNull();
  });

  it("preserves the snapshot and reports a retryable mutation failure", async () => {
    api.createTaskPlanComment.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return { store, comments: usePlanComments(TASK_ID) };
      },
      { wrapper },
    );
    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, taskPlan);
      result.current.store.getState().setTaskPlanComments(TASK_ID, snapshot());
    });

    await act(async () => {
      await expect(
        result.current.comments.handleAddComment("unsaved", "selection", 3, 8),
      ).resolves.toBeNull();
    });

    expect(result.current.comments.comments[0]?.text).toBe("shared feedback");
    expect(result.current.comments.mutationError).toBeTruthy();
  });

  it("keeps the edit base version when a live snapshot arrives", async () => {
    api.updateTaskPlanComment.mockRejectedValue(new Error("version conflict"));
    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return { store, comments: usePlanComments(TASK_ID) };
      },
      { wrapper },
    );
    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, taskPlan);
      result.current.store.getState().setTaskPlanComments(TASK_ID, snapshot(1));
    });
    await waitFor(() => expect(result.current.comments.comments).toHaveLength(1));
    act(() => {
      result.current.comments.setEditingCommentId("comment-1");
    });
    act(() => result.current.store.getState().setTaskPlanComments(TASK_ID, snapshot(2, "remote")));

    await act(async () => {
      await result.current.comments.handleAddComment("local draft", "selected", 2, 7);
    });

    expect(api.updateTaskPlanComment).toHaveBeenCalledWith(
      expect.objectContaining({ id: "comment-1", expectedVersion: 1, body: "local draft" }),
    );
  });

  it("keeps the delete base version when a live snapshot arrives", async () => {
    api.deleteTaskPlanComment.mockRejectedValue(new Error("version conflict"));
    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return { store, comments: usePlanComments(TASK_ID) };
      },
      { wrapper },
    );
    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, taskPlan);
      result.current.store.getState().setTaskPlanComments(TASK_ID, snapshot(1));
    });
    await waitFor(() => expect(result.current.comments.comments).toHaveLength(1));
    act(() => {
      result.current.comments.setEditingCommentId("comment-1");
    });
    act(() => result.current.store.getState().setTaskPlanComments(TASK_ID, snapshot(2, "remote")));

    await act(async () => {
      await result.current.comments.handleDeleteComment("comment-1");
    });

    expect(api.deleteTaskPlanComment).toHaveBeenCalledWith(
      expect.objectContaining({ id: "comment-1", expectedVersion: 1 }),
    );
  });

  it("reprojects the saved snapshot when delete transport fails", async () => {
    api.deleteTaskPlanComment.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return { store, comments: usePlanComments(TASK_ID) };
      },
      { wrapper },
    );
    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, taskPlan);
      result.current.store.getState().setTaskPlanComments(TASK_ID, snapshot(1));
    });
    const before = result.current.store.getState().taskPlans.commentsByTaskId[TASK_ID];

    await act(async () => {
      await result.current.comments.handleDeleteComment("comment-1");
    });

    const after = result.current.store.getState().taskPlans.commentsByTaskId[TASK_ID];
    expect(after).not.toBe(before);
    expect(after?.comments).toHaveLength(1);
  });
});
