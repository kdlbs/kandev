import type { StateCreator } from "zustand";
import type { Draft } from "immer";
import type { KanbanSlice, KanbanSliceActions, KanbanSliceState } from "./types";
import { generateUUID } from "@/lib/utils";
import {
  advanceTaskNavigationRevision,
  beginTaskRemoval,
  createTaskRemovalState,
  recordTaskRemovalResult,
  releaseTaskRemoval,
} from "@/lib/state/task-removal";
import { mergeStepOrderRevisions } from "@/lib/kanban/workflow-step-order";
import {
  acknowledgeWorkflowSessionFocus,
  beginWorkflowSessionFocus,
  bindWorkflowSessionFocus,
  cancelWorkflowSessionFocus,
  createWorkflowSessionFocusState,
  reconcileWorkflowSessionFocus,
  type WorkflowSessionFocusCancelScope,
  type WorkflowSessionFocusStart,
} from "@/lib/state/workflow-session-focus";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import {
  WORKSPACE_CONTEXT_COLLECTIONS,
  type WorkspaceContextCollection,
  type WorkspaceContextReadError,
  type WorkspaceContextReadState,
} from "./types";

function createWorkspaceContextReadState(
  workspaceId: string | null = null,
  generation = 0,
  retryVersion = 0,
  retryCycle = 0,
): WorkspaceContextReadState {
  return {
    workspaceId,
    generation,
    pending: Object.fromEntries(
      WORKSPACE_CONTEXT_COLLECTIONS.map((collection) => [collection, false]),
    ) as WorkspaceContextReadState["pending"],
    errors: Object.fromEntries(
      WORKSPACE_CONTEXT_COLLECTIONS.map((collection) => [collection, null]),
    ) as WorkspaceContextReadState["errors"],
    retryAfterMs: Object.fromEntries(
      WORKSPACE_CONTEXT_COLLECTIONS.map((collection) => [collection, null]),
    ) as WorkspaceContextReadState["retryAfterMs"],
    requestIds: Object.fromEntries(
      WORKSPACE_CONTEXT_COLLECTIONS.map((collection) => [collection, null]),
    ) as WorkspaceContextReadState["requestIds"],
    snapshotPending: false,
    snapshotError: null,
    snapshotRetryAfterMs: null,
    snapshotRequestId: null,
    retryVersion,
    retryCycle,
  };
}

export const defaultKanbanState: KanbanSliceState = {
  kanban: { workflowId: null, steps: [], tasks: [] },
  kanbanMulti: {
    snapshots: {},
    isLoading: false,
    orderRevisionByStepId: {},
    pendingReorderBandKeys: {},
    withheldReorderByBandKey: {},
  },
  sidebarArchivedTasks: {
    itemsByWorkspaceId: {},
    loadedByWorkspaceId: {},
    loadingByWorkspaceId: {},
    errorByWorkspaceId: {},
    revisionByWorkspaceId: {},
  },
  workflows: { items: [], activeId: null },
  workspaceContextGeneration: 0,
  workspaceContextRead: createWorkspaceContextReadState(),
  tasks: {
    activeTaskId: null,
    activeSessionId: null,
    pinnedSessionId: null,
    lastSessionByTaskId: {},
    resumeSkippedSessionIds: {},
  },
  workflowSessionFocus: createWorkflowSessionFocusState(),
  taskRemoval: createTaskRemovalState(),
};

type KanbanSliceSet = Parameters<
  StateCreator<KanbanSlice, [["zustand/immer", never]], [], KanbanSlice>
>[0];
type KanbanSliceGet = Parameters<
  StateCreator<KanbanSlice, [["zustand/immer", never]], [], KanbanSlice>
>[1];

type SidebarArchivedTaskActions = Pick<
  KanbanSliceActions,
  | "setSidebarArchivedTasks"
  | "setSidebarArchivedTasksLoading"
  | "setSidebarArchivedTasksError"
  | "upsertSidebarArchivedTask"
  | "removeSidebarArchivedTask"
>;

function createSidebarArchivedTaskActions(set: KanbanSliceSet): SidebarArchivedTaskActions {
  return {
    setSidebarArchivedTasks: (workspaceId, tasks, expectedRevision) => {
      let applied = false;
      set((draft) => {
        const revision = draft.sidebarArchivedTasks.revisionByWorkspaceId[workspaceId] ?? 0;
        if (expectedRevision !== undefined && revision !== expectedRevision) return;
        const seen = new Set<string>();
        draft.sidebarArchivedTasks.itemsByWorkspaceId[workspaceId] = tasks.filter((task) => {
          if (seen.has(task.id)) return false;
          seen.add(task.id);
          return true;
        });
        draft.sidebarArchivedTasks.loadedByWorkspaceId[workspaceId] = true;
        draft.sidebarArchivedTasks.errorByWorkspaceId[workspaceId] = null;
        draft.sidebarArchivedTasks.revisionByWorkspaceId[workspaceId] = revision + 1;
        applied = true;
      });
      return applied;
    },
    setSidebarArchivedTasksLoading: (workspaceId, loading) =>
      set((draft) => {
        draft.sidebarArchivedTasks.loadingByWorkspaceId[workspaceId] = loading;
      }),
    setSidebarArchivedTasksError: (workspaceId, error) =>
      set((draft) => {
        draft.sidebarArchivedTasks.errorByWorkspaceId[workspaceId] = error;
      }),
    upsertSidebarArchivedTask: (workspaceId, task) =>
      set((draft) => {
        const items =
          draft.sidebarArchivedTasks.itemsByWorkspaceId[workspaceId] ??
          (draft.sidebarArchivedTasks.itemsByWorkspaceId[workspaceId] = []);
        const index = items.findIndex((item) => item.id === task.id);
        if (index >= 0) items[index] = task;
        else items.push(task);
        const revision = draft.sidebarArchivedTasks.revisionByWorkspaceId[workspaceId] ?? 0;
        draft.sidebarArchivedTasks.revisionByWorkspaceId[workspaceId] = revision + 1;
      }),
    removeSidebarArchivedTask: (taskId, workspaceId) =>
      set((draft) => {
        const workspaceIds = workspaceId
          ? [workspaceId]
          : Object.keys(draft.sidebarArchivedTasks.itemsByWorkspaceId);
        for (const id of workspaceIds) {
          const items = draft.sidebarArchivedTasks.itemsByWorkspaceId[id];
          if (!items || !items.some((task) => task.id === taskId)) continue;
          draft.sidebarArchivedTasks.itemsByWorkspaceId[id] = items.filter(
            (task) => task.id !== taskId,
          );
          const revision = draft.sidebarArchivedTasks.revisionByWorkspaceId[id] ?? 0;
          draft.sidebarArchivedTasks.revisionByWorkspaceId[id] = revision + 1;
        }
      }),
  };
}

type TaskActions = Pick<
  KanbanSliceActions,
  | "setActiveTask"
  | "setActiveTaskAuto"
  | "setActiveSession"
  | "setActiveSessionAuto"
  | "clearActiveSession"
  | "beginWorkflowSessionFocus"
  | "bindWorkflowSessionFocus"
  | "reconcileWorkflowSessionFocus"
  | "cancelWorkflowSessionFocus"
  | "acknowledgeWorkflowSessionFocus"
  | "setResumeSkipped"
  | "beginTaskRemoval"
  | "recordTaskRemovalResult"
  | "releaseTaskRemoval"
  | "advanceTaskNavigationRevision"
>;

type WorkspaceContextActions = Pick<
  KanbanSliceActions,
  "setWorkspaceContextRead" | "setWorkspaceSnapshotRead" | "requestWorkspaceContextRefresh"
>;

type WorkspaceContextUpdate = {
  workspaceId: string;
  generation: number;
  result: "pending" | "success" | WorkspaceContextReadError;
  retryAfterMs?: number;
  requestId?: string;
};

type WorkspaceContextCollectionUpdate = WorkspaceContextUpdate & {
  collection: WorkspaceContextCollection;
};

function prepareWorkspaceContextRead(
  draft: Draft<KanbanSlice>,
  update: WorkspaceContextUpdate,
): Draft<WorkspaceContextReadState> | null {
  if (draft.workspaceContextGeneration !== update.generation) return null;
  const current = draft.workspaceContextRead;
  if (current.workspaceId !== null && current.workspaceId !== update.workspaceId) return null;
  if (current.workspaceId === update.workspaceId && current.generation === update.generation) {
    return current;
  }
  if (update.result !== "pending") return null;
  draft.workspaceContextRead = createWorkspaceContextReadState(
    update.workspaceId,
    update.generation,
    current.retryVersion,
    current.retryCycle,
  );
  return draft.workspaceContextRead;
}

function ownsWorkspaceContextRequest(
  currentRequestId: string | null,
  result: WorkspaceContextUpdate["result"],
  requestId: string | undefined,
): boolean {
  if (requestId === undefined) return currentRequestId === null;
  return result === "pending" || currentRequestId === requestId;
}

function nextWorkspaceContextRequestId(
  result: WorkspaceContextUpdate["result"],
  requestId: string | undefined,
): string | null {
  if (requestId === undefined) return null;
  return result === "pending" ? requestId : null;
}

function workspaceContextError(
  result: WorkspaceContextUpdate["result"],
): WorkspaceContextReadError | null {
  return result === "pending" || result === "success" || result === "cancelled" ? null : result;
}

function applyWorkspaceContextRead(
  draft: Draft<KanbanSlice>,
  update: WorkspaceContextCollectionUpdate,
) {
  const read = prepareWorkspaceContextRead(draft, update);
  if (
    !read ||
    !ownsWorkspaceContextRequest(
      read.requestIds[update.collection],
      update.result,
      update.requestId,
    )
  ) {
    return;
  }
  read.requestIds[update.collection] = nextWorkspaceContextRequestId(
    update.result,
    update.requestId,
  );
  read.pending[update.collection] = update.result === "pending";
  read.errors[update.collection] = workspaceContextError(update.result);
  read.retryAfterMs[update.collection] =
    update.result === "transient" ? (update.retryAfterMs ?? null) : null;
}

function applyWorkspaceSnapshotRead(draft: Draft<KanbanSlice>, update: WorkspaceContextUpdate) {
  const read = prepareWorkspaceContextRead(draft, update);
  if (
    !read ||
    !ownsWorkspaceContextRequest(read.snapshotRequestId, update.result, update.requestId)
  ) {
    return;
  }
  read.snapshotRequestId = nextWorkspaceContextRequestId(update.result, update.requestId);
  read.snapshotPending = update.result === "pending";
  read.snapshotError = workspaceContextError(update.result);
  read.snapshotRetryAfterMs = update.result === "transient" ? (update.retryAfterMs ?? null) : null;
}

function createWorkspaceContextActions(set: KanbanSliceSet): WorkspaceContextActions {
  return {
    setWorkspaceContextRead: (...args) => {
      const [collection, workspaceId, generation, result, retryAfterMs, requestId] = args;
      set((draft) =>
        applyWorkspaceContextRead(draft, {
          collection,
          workspaceId,
          generation,
          result,
          retryAfterMs,
          requestId,
        }),
      );
    },
    setWorkspaceSnapshotRead: (workspaceId, generation, result, retryAfterMs, requestId) =>
      set((draft) =>
        applyWorkspaceSnapshotRead(draft, {
          workspaceId,
          generation,
          result,
          retryAfterMs,
          requestId,
        }),
      ),
    requestWorkspaceContextRefresh: (resetRetryCycle = true) =>
      set((draft) => {
        draft.workspaceContextRead.retryVersion += 1;
        if (resetRetryCycle) draft.workspaceContextRead.retryCycle += 1;
      }),
  };
}

type FocusLookupState = {
  taskSessionsByTask?: {
    itemsByTaskId?: Record<string, Array<{ id: string }>>;
  };
  taskSessions?: {
    items?: Record<string, { id: string; task_id?: string }>;
  };
};

type WorkflowSessionFocusActions = Pick<
  TaskActions,
  | "beginWorkflowSessionFocus"
  | "bindWorkflowSessionFocus"
  | "reconcileWorkflowSessionFocus"
  | "cancelWorkflowSessionFocus"
  | "acknowledgeWorkflowSessionFocus"
>;

function createWorkflowSessionFocusActions(
  set: KanbanSliceSet,
  get: KanbanSliceGet,
): WorkflowSessionFocusActions {
  const knownSessionIdsForTask = (taskId: string): string[] => {
    const state = get() as KanbanSlice & FocusLookupState;
    const ids = new Set(
      state.taskSessionsByTask?.itemsByTaskId?.[taskId]?.map((session) => session.id) ?? [],
    );
    for (const session of Object.values(state.taskSessions?.items ?? {})) {
      if (session.task_id === taskId) ids.add(session.id);
    }
    return [...ids];
  };

  const taskProjectionForTask = (
    taskId: string,
  ): { metadata: unknown; updatedAt?: string; workflowStepId?: string } => {
    const state = get();
    const tasks = [
      ...state.kanban.tasks,
      ...Object.values(state.kanbanMulti.snapshots).flatMap((snapshot) => snapshot.tasks),
    ].filter((candidate) => candidate.id === taskId);
    let task = tasks[0];
    for (const candidate of tasks.slice(1)) {
      const selectedTime = parseTurnTimestamp(task?.updatedAt);
      const candidateTime = parseTurnTimestamp(candidate.updatedAt);
      if (
        task === undefined ||
        (selectedTime === null && candidateTime !== null) ||
        (candidateTime !== null && selectedTime !== null && candidateTime > selectedTime)
      ) {
        task = candidate;
      }
    }
    return {
      metadata: task?.metadata,
      updatedAt: task?.updatedAt,
      workflowStepId: task?.workflowStepId,
    };
  };

  return {
    beginWorkflowSessionFocus: (input: WorkflowSessionFocusStart) => {
      let requestId: number | null = null;
      set((draft) => {
        const result = beginWorkflowSessionFocus(draft.workflowSessionFocus, input);
        draft.workflowSessionFocus = result.state;
        requestId = result.requestId;
      });
      return requestId;
    },
    bindWorkflowSessionFocus: (input) =>
      set((draft) => {
        draft.workflowSessionFocus = bindWorkflowSessionFocus(draft.workflowSessionFocus, input);
      }),
    reconcileWorkflowSessionFocus: (taskId, responseProjection) =>
      set((draft) => {
        const liveProjection = taskProjectionForTask(taskId);
        const result = reconcileWorkflowSessionFocus(draft.workflowSessionFocus, {
          activeTaskId: draft.tasks.activeTaskId,
          navigationRevision: draft.taskRemoval.navigationRevision,
          routeMetadata: liveProjection.metadata,
          routeUpdatedAt: liveProjection.updatedAt,
          workflowStepId: liveProjection.workflowStepId,
          responseProjection,
          knownSessionIds: knownSessionIdsForTask(taskId),
        });
        draft.workflowSessionFocus = result.state;
        if (!result.sessionId) return;
        draft.tasks.activeSessionId = result.sessionId;
        draft.tasks.pinnedSessionId = null;
        draft.tasks.lastSessionByTaskId[taskId] = result.sessionId;
      }),
    cancelWorkflowSessionFocus: (scope?: WorkflowSessionFocusCancelScope) =>
      set((draft) => {
        draft.workflowSessionFocus = cancelWorkflowSessionFocus(draft.workflowSessionFocus, scope);
      }),
    acknowledgeWorkflowSessionFocus: (requestId) =>
      set((draft) => {
        draft.workflowSessionFocus = acknowledgeWorkflowSessionFocus(
          draft.workflowSessionFocus,
          requestId,
        );
      }),
  };
}

function createTaskActions(set: KanbanSliceSet, get: KanbanSliceGet): TaskActions {
  return {
    ...createWorkflowSessionFocusActions(set, get),
    setActiveTask: (taskId) =>
      set((draft) => {
        draft.workflowSessionFocus = cancelWorkflowSessionFocus(draft.workflowSessionFocus);
        draft.taskRemoval = advanceTaskNavigationRevision(draft.taskRemoval);
        draft.tasks.activeTaskId = taskId;
        draft.tasks.activeSessionId = null;
        // New task → drop any pin; the pin only applies within a single task.
        draft.tasks.pinnedSessionId = null;
      }),
    setActiveTaskAuto: (taskId) =>
      set((draft) => {
        if (draft.tasks.activeTaskId !== taskId) {
          draft.workflowSessionFocus = cancelWorkflowSessionFocus(draft.workflowSessionFocus);
        }
        draft.tasks.activeTaskId = taskId;
        draft.tasks.activeSessionId = null;
        draft.tasks.pinnedSessionId = null;
      }),
    setActiveSession: (taskId, sessionId) =>
      set((draft) => {
        draft.workflowSessionFocus = cancelWorkflowSessionFocus(draft.workflowSessionFocus);
        draft.taskRemoval = advanceTaskNavigationRevision(draft.taskRemoval);
        draft.tasks.activeTaskId = taskId;
        draft.tasks.activeSessionId = sessionId;
        // User-initiated selection: pin so WS auto-replace handoff respects it.
        draft.tasks.pinnedSessionId = sessionId;
        draft.tasks.lastSessionByTaskId[taskId] = sessionId;
      }),
    setActiveSessionAuto: (taskId, sessionId) =>
      set((draft) => {
        if (draft.tasks.activeTaskId !== taskId) {
          draft.workflowSessionFocus = cancelWorkflowSessionFocus(draft.workflowSessionFocus);
          draft.tasks.pinnedSessionId = null;
        }
        draft.tasks.activeTaskId = taskId;
        draft.tasks.activeSessionId = sessionId;
        draft.tasks.lastSessionByTaskId[taskId] = sessionId;
      }),
    beginTaskRemoval: (input) => {
      let token: string | null = null;
      set((draft) => {
        const candidate = generateUUID();
        const next = beginTaskRemoval(draft.taskRemoval, { ...input, token: candidate });
        if (!next) return;
        draft.taskRemoval = next;
        token = candidate;
      });
      return token;
    },
    recordTaskRemovalResult: (token, taskIds, outcome) =>
      set((draft) => {
        draft.taskRemoval = recordTaskRemovalResult(draft.taskRemoval, token, taskIds, outcome);
      }),
    releaseTaskRemoval: (token) =>
      set((draft) => {
        draft.taskRemoval = releaseTaskRemoval(draft.taskRemoval, token);
      }),
    advanceTaskNavigationRevision: () =>
      set((draft) => {
        draft.workflowSessionFocus = cancelWorkflowSessionFocus(draft.workflowSessionFocus);
        draft.taskRemoval = advanceTaskNavigationRevision(draft.taskRemoval);
      }),
    clearActiveSession: () =>
      set((draft) => {
        draft.workflowSessionFocus = cancelWorkflowSessionFocus(draft.workflowSessionFocus);
        draft.tasks.activeSessionId = null;
        draft.tasks.pinnedSessionId = null;
      }),
    setResumeSkipped: (sessionId, skipped) =>
      set((draft) => {
        if (skipped) {
          // Recording is guarded at the call site (use-session-resumption's
          // skip branch reads the live session row with typed store access),
          // so a stale status can never mark a starting/running session as
          // resume-skipped. The slice stays a plain keyed write.
          draft.tasks.resumeSkippedSessionIds[sessionId] = true;
        } else {
          delete draft.tasks.resumeSkippedSessionIds[sessionId];
        }
      }),
  };
}

export const createKanbanSlice: StateCreator<
  KanbanSlice,
  [["zustand/immer", never]],
  [],
  KanbanSlice
> = (set, get) => ({
  ...defaultKanbanState,
  ...createSidebarArchivedTaskActions(set),
  ...createTaskActions(set, get),
  ...createWorkspaceContextActions(set),
  resetKanbanWorkspaceContext: () =>
    set((draft) => {
      const nextGeneration = draft.workspaceContextGeneration + 1;
      Object.assign(draft, defaultKanbanState);
      draft.workspaceContextGeneration = nextGeneration;
    }),
  setActiveWorkflow: (workflowId) =>
    set((draft) => {
      if (draft.workflows.activeId === workflowId) return;
      draft.workflows.activeId = workflowId;
    }),
  setWorkflows: (workflows) =>
    set((draft) => {
      draft.workflows.items = workflows;
    }),
  reorderWorkflowItems: (workflowIds) =>
    set((draft) => {
      const byId = new Map(draft.workflows.items.map((w) => [w.id, w]));
      const reordered = workflowIds
        .map((id) => byId.get(id))
        .filter((w): w is NonNullable<typeof w> => w != null);
      // Append any items not in the reorder list (shouldn't happen, but defensive)
      for (const item of draft.workflows.items) {
        if (!workflowIds.includes(item.id)) reordered.push(item);
      }
      draft.workflows.items = reordered;
    }),
  setWorkflowSnapshot: (workflowId, data) =>
    set((draft) => {
      draft.kanbanMulti.snapshots[workflowId] = data;
      // Seed this workflow's steps into orderRevisionByStepId even when it
      // is not the currently-active board (hydrate() only walks the active
      // kanban.steps), so a task.reordered WS event for a background
      // "All Workflows" column is still gated on a real revision rather
      // than the no-recorded-revision fallback.
      draft.kanbanMulti.orderRevisionByStepId = mergeStepOrderRevisions(
        draft.kanbanMulti.orderRevisionByStepId,
        data.steps,
      );
    }),
  setKanbanMultiLoading: (loading) =>
    set((draft) => {
      draft.kanbanMulti.isLoading = loading;
    }),
  clearKanbanMulti: () =>
    set((draft) => {
      draft.kanbanMulti.snapshots = {};
      draft.kanbanMulti.isLoading = false;
    }),
  updateMultiTask: (workflowId, task) =>
    set((draft) => {
      const snapshot = draft.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;
      const idx = snapshot.tasks.findIndex((t) => t.id === task.id);
      if (idx >= 0) {
        snapshot.tasks[idx] = task;
      } else {
        snapshot.tasks.push(task);
      }
    }),
  removeMultiTask: (workflowId, taskId) =>
    set((draft) => {
      const snapshot = draft.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;
      snapshot.tasks = snapshot.tasks.filter((t) => t.id !== taskId);
    }),
  setStepOrderRevision: (stepId, revision) =>
    set((draft) => {
      draft.kanbanMulti.orderRevisionByStepId[stepId] = revision;
    }),
  setBandReorderPending: (stepId, band, pending) =>
    set((draft) => {
      const key = `${stepId}:${band}`;
      if (pending) {
        draft.kanbanMulti.pendingReorderBandKeys[key] = true;
      } else {
        delete draft.kanbanMulti.pendingReorderBandKeys[key];
      }
    }),
  setWithheldReorder: (stepId, band, payload) =>
    set((draft) => {
      const key = `${stepId}:${band}`;
      if (payload) {
        draft.kanbanMulti.withheldReorderByBandKey[key] = payload;
      } else {
        delete draft.kanbanMulti.withheldReorderByBandKey[key];
      }
    }),
});
