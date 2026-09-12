import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  createTaskPlanComment,
  deleteTaskPlanComment,
  updateTaskPlanComment,
} from "@/lib/api/domains/plan-comment-api";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { planCommentLoaderFor } from "./plan-comment-loading";
import type { PlanComment } from "@/lib/state/slices/comments";
import type { AppState } from "@/lib/state/store";
import type { TaskPlan, TaskPlanComment } from "@/lib/types/http";
import { generateUUID } from "@/lib/utils";
import { planCommentAdmissionConflict } from "@/lib/plan-comment-refs";

const EMPTY_COMMENTS: PlanComment[] = [];
type AppStore = ReturnType<typeof useAppStoreApi>;

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

function applyMutationFailure(error: unknown, taskId: string, state: AppState) {
  const snapshot = planCommentAdmissionConflict(error)?.snapshot;
  if (snapshot) state.setTaskPlanComments(taskId, snapshot);
}

type EditingCommentBase = {
  taskId: string;
  planId: string;
  id: string;
  version: number;
};

type PendingCommentCreate = { key: string; id: string };

function pendingCommentCreateID(
  pending: MutableRefObject<PendingCommentCreate | null>,
  key: string,
) {
  if (pending.current?.key === key) return pending.current.id;
  const id = generateUUID();
  pending.current = { key, id };
  return id;
}

function useEditingPlanComment(
  taskId: string | null | undefined,
  plan: TaskPlan | null | undefined,
  store: AppStore,
) {
  const [editingCommentId, setEditingCommentIdState] = useState<string | null>(null);
  const editingBase = useRef<EditingCommentBase | null>(null);
  useEffect(() => {
    editingBase.current = null;
    setEditingCommentIdState(null);
  }, [plan?.id, taskId]);

  const setEditingCommentId = useCallback(
    (commentId: string | null) => {
      setEditingCommentIdState(commentId);
      if (!commentId || !taskId || !plan) {
        editingBase.current = null;
        return;
      }
      const comment = store
        .getState()
        .taskPlans.commentsByTaskId[
          taskId
        ]?.comments.find((candidate) => candidate.id === commentId && candidate.plan_id === plan.id);
      editingBase.current = comment
        ? { taskId, planId: plan.id, id: comment.id, version: comment.version }
        : null;
    },
    [plan, store, taskId],
  );
  return { editingBase, editingCommentId, setEditingCommentId, setEditingCommentIdState };
}

function useDeletePlanComment({
  taskId,
  plan,
  comments,
  store,
  editingBase,
  editingCommentId,
  setEditingCommentIdState,
  setIsMutating,
  setMutationError,
}: {
  taskId: string | null | undefined;
  plan: TaskPlan | null | undefined;
  comments: PlanComment[];
  store: AppStore;
  editingBase: MutableRefObject<EditingCommentBase | null>;
  editingCommentId: string | null;
  setEditingCommentIdState: Dispatch<SetStateAction<string | null>>;
  setIsMutating: Dispatch<SetStateAction<boolean>>;
  setMutationError: Dispatch<SetStateAction<string | null>>;
}) {
  const { t } = useTranslation("task");
  return useCallback(
    async (commentId: string): Promise<boolean> => {
      if (!taskId || !plan) return false;
      const base = editingBase.current;
      const comment = comments.find((candidate) => candidate.id === commentId);
      if (!comment) return false;
      setIsMutating(true);
      setMutationError(null);
      try {
        const next = await deleteTaskPlanComment({
          taskId,
          planId: plan.id,
          id: commentId,
          expectedVersion:
            base?.taskId === taskId && base.planId === plan.id && base.id === commentId
              ? base.version
              : (comment.version ?? 0),
        });
        store.getState().setTaskPlanComments(taskId, next);
        if (editingCommentId === commentId) {
          setEditingCommentIdState(null);
          editingBase.current = null;
        }
        return true;
      } catch (error) {
        // i18n-exempt: developer diagnostic; localized copy is exposed below.
        console.error("Failed to delete task plan comment:", error);
        applyMutationFailure(error, taskId, store.getState());
        const current = store.getState().taskPlans.commentsByTaskId[taskId];
        if (current) {
          store.getState().setTaskPlanComments(taskId, {
            ...current,
            comments: [...current.comments],
          });
        }
        setMutationError(t("failedToDeletePlanComment"));
        return false;
      } finally {
        setIsMutating(false);
      }
    },
    [
      comments,
      editingBase,
      editingCommentId,
      plan,
      setEditingCommentIdState,
      setIsMutating,
      setMutationError,
      store,
      t,
      taskId,
    ],
  );
}

function usePlanCommentMutations(
  taskId: string | null | undefined,
  plan: TaskPlan | null | undefined,
  comments: PlanComment[],
  store: AppStore,
) {
  const { t } = useTranslation("task");
  const [isMutating, setIsMutating] = useState(false);
  const [mutationError, setMutationError] = useState<string | null>(null);
  const pendingCreate = useRef<PendingCommentCreate | null>(null);
  const { editingBase, editingCommentId, setEditingCommentId, setEditingCommentIdState } =
    useEditingPlanComment(taskId, plan, store);

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
        const base = editingBase.current;
        const editing = comments.find((comment) => comment.id === editingCommentId);
        const anchorFrom = from ?? 0;
        const anchorTo = to ?? 0;
        const createKey = JSON.stringify([
          taskId,
          plan.id,
          commentText,
          selectedText,
          anchorFrom,
          anchorTo,
        ]);
        const id = editing?.id ?? pendingCommentCreateID(pendingCreate, createKey);
        const next = editing
          ? await updateTaskPlanComment({
              taskId,
              planId: plan.id,
              id,
              body: commentText,
              expectedVersion:
                base?.taskId === taskId && base.planId === plan.id && base.id === id
                  ? base.version
                  : (editing.version ?? 0),
            })
          : await createTaskPlanComment({
              taskId,
              planId: plan.id,
              id,
              body: commentText,
              selectedText,
              anchorFrom,
              anchorTo,
            });
        store.getState().setTaskPlanComments(taskId, next);
        pendingCreate.current = null;
        setEditingCommentIdState(null);
        editingBase.current = null;
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

  const handleDeleteComment = useDeletePlanComment({
    taskId,
    plan,
    comments,
    store,
    editingBase,
    editingCommentId,
    setEditingCommentIdState,
    setIsMutating,
    setMutationError,
  });

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
  const loader = useMemo(
    () => (taskId ? planCommentLoaderFor(store, taskId, t("failedToLoadPlanComments")) : null),
    [store, t, taskId],
  );
  useEffect(() => loader?.attach(), [loader]);
  const refetch = useCallback(() => loader?.load(true) ?? Promise.resolve(), [loader]);
  const wake = useCallback(() => loader?.wake() ?? Promise.resolve(), [loader]);

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
    void loader?.load(false);
  }, [connectionStatus, isLoaded, isLoading, isPlanLoading, loadError, loader, taskId]);
  usePlanCommentReconnectRefresh(connectionStatus, wake);
  useForegroundRefresh(wake, Boolean(taskId) && connectionStatus === "connected", loader);

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
