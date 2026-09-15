import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  clearTaskPreviewFeedback,
  createTaskPreviewFeedback,
  deleteTaskPreviewFeedback,
  getTaskPreviewFeedback,
  updateTaskPreviewFeedback,
  type CreateTaskPreviewFeedbackInput,
} from "@/lib/api/domains/preview-feedback-api";
import type { AppState } from "@/lib/state/store";
import type { TaskPreviewFeedbackSnapshot } from "@/lib/types/http";
import { generateUUID } from "@/lib/utils";

type AppStore = ReturnType<typeof useAppStoreApi>;
const loadsByStore = new WeakMap<AppStore, Map<string, Promise<void>>>();

function storeLoads(store: AppStore) {
  let loads = loadsByStore.get(store);
  if (!loads) {
    loads = new Map();
    loadsByStore.set(store, loads);
  }
  return loads;
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function loadPreviewFeedback(store: AppStore, taskId: string, force: boolean) {
  const state = store.getState();
  if (!force && state.previewFeedback.loadedByTaskId[taskId]) return Promise.resolve();
  const loads = storeLoads(store);
  const current = loads.get(taskId);
  if (current) return current;

  state.setTaskPreviewFeedbackLoading(taskId, true);
  state.setTaskPreviewFeedbackError(taskId);
  const request = getTaskPreviewFeedback(taskId)
    .then((snapshot) => store.getState().setTaskPreviewFeedback(taskId, snapshot))
    .catch((error: unknown) => {
      store.getState().setTaskPreviewFeedbackError(taskId, errorMessage(error));
    })
    .finally(() => loads.delete(taskId));
  loads.set(taskId, request);
  return request;
}

function conflictSnapshot(error: unknown): TaskPreviewFeedbackSnapshot | undefined {
  if (!error || typeof error !== "object") return undefined;
  const details = (error as { details?: { snapshot?: TaskPreviewFeedbackSnapshot } }).details;
  return details?.snapshot;
}

type CreateDraft = Omit<CreateTaskPreviewFeedbackInput, "taskId" | "id">;

function usePreviewFeedbackMutations(taskId: string | null | undefined, store: AppStore) {
  const [isMutating, setIsMutating] = useState(false);
  const [mutationError, setMutationError] = useState<string | null>(null);
  const pendingCreate = useRef<{ key: string; id: string } | null>(null);

  const apply = useCallback(
    async (operation: () => Promise<TaskPreviewFeedbackSnapshot>) => {
      if (!taskId) return null;
      setIsMutating(true);
      setMutationError(null);
      try {
        const snapshot = await operation();
        store.getState().setTaskPreviewFeedback(taskId, snapshot);
        return snapshot;
      } catch (error) {
        const snapshot = conflictSnapshot(error);
        if (snapshot) store.getState().setTaskPreviewFeedback(taskId, snapshot);
        setMutationError(errorMessage(error));
        return null;
      } finally {
        setIsMutating(false);
      }
    },
    [store, taskId],
  );

  const create = useCallback(
    async (draft: CreateDraft) => {
      if (!taskId) return null;
      const key = JSON.stringify([taskId, draft]);
      if (pendingCreate.current?.key !== key) {
        pendingCreate.current = { key, id: generateUUID() };
      }
      const result = await apply(() =>
        createTaskPreviewFeedback({
          ...draft,
          taskId,
          id: pendingCreate.current?.id ?? generateUUID(),
        }),
      );
      if (result) pendingCreate.current = null;
      return result;
    },
    [apply, taskId],
  );

  const update = useCallback(
    (id: string, comment: string, expectedVersion: number) =>
      apply(() =>
        updateTaskPreviewFeedback({ taskId: taskId ?? "", id, comment, expectedVersion }),
      ),
    [apply, taskId],
  );
  const remove = useCallback(
    (id: string, expectedVersion: number) =>
      apply(() => deleteTaskPreviewFeedback({ taskId: taskId ?? "", id, expectedVersion })),
    [apply, taskId],
  );
  const clear = useCallback(
    (expectedRevision: number) =>
      apply(() => clearTaskPreviewFeedback(taskId ?? "", expectedRevision)),
    [apply, taskId],
  );

  return { create, update, remove, clear, isMutating, mutationError };
}

/** Task-owned pending rendered-page feedback shared by every task surface. */
export function usePreviewFeedback(taskId: string | null | undefined) {
  const store = useAppStoreApi();
  const snapshot = useAppStore((state) =>
    taskId ? state.previewFeedback.byTaskId[taskId] : undefined,
  );
  const isLoading = useAppStore((state) =>
    taskId ? (state.previewFeedback.loadingByTaskId[taskId] ?? false) : false,
  );
  const isLoaded = useAppStore((state) =>
    taskId ? (state.previewFeedback.loadedByTaskId[taskId] ?? false) : false,
  );
  const loadError = useAppStore((state) =>
    taskId ? (state.previewFeedback.errorByTaskId[taskId] ?? null) : null,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const previousConnection = useRef(connectionStatus);

  useEffect(() => {
    const previous = previousConnection.current;
    previousConnection.current = connectionStatus;
    if (!taskId || connectionStatus !== "connected") return;
    void loadPreviewFeedback(store, taskId, previous !== "connected");
  }, [connectionStatus, store, taskId]);

  const refetch = useCallback(
    () => (taskId ? loadPreviewFeedback(store, taskId, true) : Promise.resolve()),
    [store, taskId],
  );
  const mutations = usePreviewFeedbackMutations(taskId, store);

  return {
    snapshot,
    items: snapshot?.items ?? [],
    isLoading,
    isLoaded,
    loadError,
    refetch,
    ...mutations,
  };
}

export function applyPreviewFeedbackSnapshot(
  state: Pick<AppState, "setTaskPreviewFeedback">,
  taskId: string,
  snapshot: TaskPreviewFeedbackSnapshot,
) {
  state.setTaskPreviewFeedback(taskId, snapshot);
}
