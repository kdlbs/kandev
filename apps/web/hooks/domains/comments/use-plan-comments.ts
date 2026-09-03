import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  createTaskPlanComment,
  deleteTaskPlanComment,
  getTaskPlanComments,
  updateTaskPlanComment,
} from "@/lib/api/domains/plan-comment-api";
import { getTaskPlan } from "@/lib/api/domains/plan-api";
import type { PlanComment } from "@/lib/state/slices/comments";
import type { AppState } from "@/lib/state/store";
import type { TaskPlan, TaskPlanComment } from "@/lib/types/http";
import { generateUUID } from "@/lib/utils";
import { planCommentAdmissionConflict } from "@/lib/plan-comment-refs";

const EMPTY_COMMENTS: PlanComment[] = [];
const commentLoads = new Map<string, Promise<void>>();
const planLoads = new Map<string, Promise<TaskPlan | null>>();

function projectPlanComment(comment: TaskPlanComment): PlanComment {
  return {
    id: comment.id,
    sessionId: "",
    source: "plan",
    taskId: comment.task_id,
    planId: comment.plan_id,
    text: comment.body,
    selectedText: comment.selected_text,
    from: comment.anchor_from,
    to: comment.anchor_to,
    version: comment.version,
    createdAt: comment.created_at,
    updatedAt: comment.updated_at,
    status: "pending",
  };
}

function loadComments(
  taskId: string,
  store: ReturnType<typeof useAppStoreApi>,
  errorMessage: string,
): Promise<void> {
  const existing = commentLoads.get(taskId);
  if (existing) return existing;
  const state = store.getState();
  state.setTaskPlanCommentsLoading(taskId, true);
  state.setTaskPlanCommentsError(taskId);
  const request = getTaskPlanComments(taskId)
    .then((snapshot) => store.getState().setTaskPlanComments(taskId, snapshot))
    .catch((error) => {
      // i18n-exempt: developer diagnostic; localized copy is stored in state.
      console.error("Failed to load task plan comments:", error);
      store.getState().setTaskPlanCommentsError(taskId, errorMessage);
    })
    .finally(() => {
      commentLoads.delete(taskId);
      store.getState().setTaskPlanCommentsLoading(taskId, false);
    });
  commentLoads.set(taskId, request);
  return request;
}

function loadPlan(
  taskId: string,
  store: ReturnType<typeof useAppStoreApi>,
  errorMessage: string,
): Promise<TaskPlan | null> {
  const existing = planLoads.get(taskId);
  if (existing) return existing;
  const state = store.getState();
  const cached = state.taskPlans.byTaskId[taskId];
  if (cached !== undefined) return Promise.resolve(cached);
  state.setTaskPlanLoading(taskId, true);
  state.setTaskPlanCommentsError(taskId);
  const request = getTaskPlan(taskId)
    .then((next) => {
      store.getState().setTaskPlan(taskId, next);
      return next;
    })
    .catch((error) => {
      // i18n-exempt: developer diagnostic; localized copy is stored below.
      console.error("Failed to load task plan for comments:", error);
      store.getState().setTaskPlanCommentsError(taskId, errorMessage);
      store.getState().setTaskPlanLoading(taskId, false);
      return null;
    })
    .finally(() => planLoads.delete(taskId));
  planLoads.set(taskId, request);
  return request;
}

function applyMutationFailure(error: unknown, taskId: string, state: AppState) {
  const snapshot = planCommentAdmissionConflict(error)?.snapshot;
  if (snapshot) state.setTaskPlanComments(taskId, snapshot);
}

function usePlanCommentMutations(
  taskId: string | null | undefined,
  plan: TaskPlan | null | undefined,
  comments: PlanComment[],
  store: ReturnType<typeof useAppStoreApi>,
) {
  const { t } = useTranslation("task");
  const [editingCommentId, setEditingCommentId] = useState<string | null>(null);
  const [isMutating, setIsMutating] = useState(false);
  const [mutationError, setMutationError] = useState<string | null>(null);

  const handleAddComment = useCallback(
    async (
      commentText: string,
      selectedText: string,
      from?: number,
      to?: number,
    ): Promise<PlanComment | null> => {
      if (!taskId || !plan) return null;
      setIsMutating(true);
      setMutationError(null);
      try {
        const editing = comments.find((comment) => comment.id === editingCommentId);
        const id = editing?.id ?? generateUUID();
        const next = editing
          ? await updateTaskPlanComment({
              taskId,
              planId: plan.id,
              id,
              body: commentText,
              expectedVersion: editing.version ?? 0,
            })
          : await createTaskPlanComment({
              taskId,
              planId: plan.id,
              id,
              body: commentText,
              selectedText,
              anchorFrom: from ?? 0,
              anchorTo: to ?? 0,
            });
        store.getState().setTaskPlanComments(taskId, next);
        setEditingCommentId(null);
        const saved = next.comments.find((candidate) => candidate.id === id);
        return saved ? projectPlanComment(saved) : null;
      } catch (error) {
        // i18n-exempt: developer diagnostic; localized copy is exposed below.
        console.error("Failed to save task plan comment:", error);
        applyMutationFailure(error, taskId, store.getState());
        setMutationError(t("failedToSavePlanComment"));
        return null;
      } finally {
        setIsMutating(false);
      }
    },
    [comments, editingCommentId, plan, store, t, taskId],
  );

  const handleDeleteComment = useCallback(
    async (commentId: string): Promise<boolean> => {
      if (!taskId || !plan) return false;
      const comment = comments.find((candidate) => candidate.id === commentId);
      if (!comment) return false;
      setIsMutating(true);
      setMutationError(null);
      try {
        const next = await deleteTaskPlanComment({
          taskId,
          planId: plan.id,
          id: commentId,
          expectedVersion: comment.version ?? 0,
        });
        store.getState().setTaskPlanComments(taskId, next);
        if (editingCommentId === commentId) setEditingCommentId(null);
        return true;
      } catch (error) {
        // i18n-exempt: developer diagnostic; localized copy is exposed below.
        console.error("Failed to delete task plan comment:", error);
        applyMutationFailure(error, taskId, store.getState());
        setMutationError(t("failedToDeletePlanComment"));
        return false;
      } finally {
        setIsMutating(false);
      }
    },
    [comments, editingCommentId, plan, store, t, taskId],
  );

  return {
    isMutating,
    mutationError,
    editingCommentId,
    setEditingCommentId,
    handleAddComment,
    handleDeleteComment,
  };
}

function usePlanCommentReconnectRefresh(connectionStatus: string, refetch: () => Promise<void>) {
  const previousStatus = useRef(connectionStatus);
  useEffect(() => {
    const previous = previousStatus.current;
    previousStatus.current = connectionStatus;
    if (connectionStatus === "connected" && previous !== "connected") void refetch();
  }, [connectionStatus, refetch]);
}

/** Task-owned current-plan comments shared by every session surface. */
export function usePlanComments(taskId: string | null | undefined) {
  const { t } = useTranslation("task");
  const store = useAppStoreApi();
  const plan = useAppStore((state) => (taskId ? state.taskPlans.byTaskId[taskId] : undefined));
  const snapshot = useAppStore((state) =>
    taskId ? state.taskPlans.commentsByTaskId[taskId] : undefined,
  );
  const isLoading = useAppStore((state) =>
    taskId ? (state.taskPlans.commentsLoadingByTaskId[taskId] ?? false) : false,
  );
  const isLoaded = useAppStore((state) =>
    taskId ? (state.taskPlans.commentsLoadedByTaskId[taskId] ?? false) : false,
  );
  const loadError = useAppStore((state) =>
    taskId ? (state.taskPlans.commentsErrorByTaskId[taskId] ?? null) : null,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const isPlanLoading = useAppStore((state) =>
    taskId ? (state.taskPlans.loadingByTaskId[taskId] ?? false) : false,
  );
  const refetch = useCallback(async () => {
    if (!taskId) return;
    const resolvedPlan = plan ?? (await loadPlan(taskId, store, t("failedToLoadPlanComments")));
    if (!resolvedPlan) return;
    await loadComments(taskId, store, t("failedToLoadPlanComments"));
  }, [plan, store, t, taskId]);

  useEffect(() => {
    if (
      connectionStatus !== "connected" ||
      !taskId ||
      isLoaded ||
      isLoading ||
      isPlanLoading ||
      loadError
    ) {
      return;
    }
    void refetch();
  }, [connectionStatus, isLoaded, isLoading, isPlanLoading, loadError, refetch, taskId]);
  usePlanCommentReconnectRefresh(connectionStatus, refetch);

  const comments = useMemo(() => {
    if (!snapshot || snapshot.comments.length === 0) return EMPTY_COMMENTS;
    return snapshot.comments.map(projectPlanComment);
  }, [snapshot]);
  const mutations = usePlanCommentMutations(taskId, plan, comments, store);

  return {
    comments,
    snapshot,
    isLoading,
    isLoaded,
    loadError,
    refetch,
    ...mutations,
  };
}
