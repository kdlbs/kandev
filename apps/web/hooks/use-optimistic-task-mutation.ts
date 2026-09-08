"use client";

import { createContext, useCallback, useContext } from "react";
import { toast } from "@/lib/toast/sonner";
import { useAppStoreApi } from "@/components/state-provider";
import type { Task, TaskStatus } from "@/app/office/tasks/[id]/types";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import { t } from "@/lib/i18n";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { ApprovalGateError } from "@/lib/api/domains/office-status-gate";
import {
  beginWrite,
  endWrite,
  getCanonicalValue,
  nextTaskSequence,
  recordWriteSettled,
  recordWriteSuccess,
  shouldRestoreAfterFailedWrite,
  TASK_SCOPE,
} from "@/lib/state/office-task-content-sync";

/**
 * Context for the local (page-level) task representation. The office store
 * holds the canonical OfficeTask but the detail page maintains a richer
 * Task object with extra fields (reviewers, approvers, blockedBy, etc.).
 *
 * Pickers live inside <TaskOptimisticProvider> on the detail page; the
 * provider exposes a way to patch / restore the local task state so
 * optimistic updates flow into the visible UI without prop drilling.
 */
export type TaskOptimisticContextValue = {
  task: Task;
  applyPatch: (patch: Partial<Task>) => void;
  restore: (snapshot: Task) => void;
};

const TaskOptimisticContext = createContext<TaskOptimisticContextValue | null>(null);

export const TaskOptimisticContextProvider = TaskOptimisticContext.Provider;

export function useTaskOptimisticContext(): TaskOptimisticContextValue {
  const ctx = useContext(TaskOptimisticContext);
  if (!ctx) {
    throw new Error("useTaskOptimisticContext must be used within <TaskOptimisticContextProvider>");
  }
  return ctx;
}

/**
 * Returns a function that performs an optimistic mutation on the current
 * task. Snapshots the local + store state, applies the patch immediately,
 * runs the API call, and rolls back + toasts on failure. On success the
 * optimistic patch is left in place; the canonical reconciliation happens
 * via the `office.task.updated` WS handler (re-fetches the task DTO).
 */
export function useOptimisticTaskMutation() {
  const ctx = useTaskOptimisticContext();
  const storeApi = useAppStoreApi();

  return useCallback(
    async (
      taskId: string,
      patch: Partial<Task>,
      apiCall: () => Promise<unknown>,
    ): Promise<void> => {
      const snapshot = ctx.task;
      const storePatch = toOfficeTaskPatch(patch);
      const storeSnapshot = storeApi.getState().office.tasks.items.find((t) => t.id === taskId);

      const sequence = nextTaskSequence(taskId);
      beginWrite(taskId, TASK_SCOPE, sequence);

      // Apply optimistic patches (local + store).
      ctx.applyPatch(patch);
      if (storeSnapshot) {
        storeApi.getState().patchTaskInStore(taskId, storePatch);
      }

      try {
        await apiCall();
        recordWriteSettled(taskId, TASK_SCOPE, sequence);
        endWrite(taskId, TASK_SCOPE, sequence);
      } catch (err) {
        if (err instanceof ApprovalGateError) {
          // The backend has already persisted the redirected status at this
          // write's sequence regardless of whether the UI ends up showing it,
          // so a later-failing, lower-sequence write must see this as settled
          // rather than treating it as unresolved and clobbering it.
          recordWriteSettled(taskId, TASK_SCOPE, sequence);
        }
        // Only restore if no later-sequenced mutation on this task has
        // already succeeded or is still in flight — otherwise this stale
        // failure's rollback would clobber newer, server-confirmed state.
        const shouldRestore = shouldRestoreAfterFailedWrite(taskId, TASK_SCOPE, sequence);
        endWrite(taskId, TASK_SCOPE, sequence);
        if (shouldRestore) {
          if (err instanceof ApprovalGateError) {
            // The backend already redirected and persisted this status
            // server-side before returning the error, so a plain rollback
            // would show a status the server no longer holds. Settle on the
            // redirected status instead — status-only, not the whole
            // snapshot, so a stale `rawStatus` on the snapshot can't
            // re-normalize the card back to its pre-mutation column.
            const redirectPatch: Partial<Task> = { status: err.redirectedStatus as TaskStatus };
            ctx.applyPatch(redirectPatch);
            if (storeSnapshot) {
              storeApi.getState().patchTaskInStore(taskId, toOfficeTaskPatch(redirectPatch));
            }
          } else {
            // Rollback both layers. This hook never patches title/description
            // (see `toOfficeTaskPatch` above), so the rollback must not touch
            // them either — those two fields are governed exclusively by the
            // per-field guard in office-task-content-sync.ts and have their own
            // dedicated writers (useCommitTaskTitle/useCommitTaskDescription).
            // Restoring the full pre-mutation snapshot here would silently
            // revert a confirmed title/description edit whenever an unrelated
            // picker mutation fails (AC-61: exactly two writers may touch a
            // guarded field).
            ctx.restore(snapshot);
            if (storeSnapshot) {
              const { title: _title, description: _description, ...storeRollback } = storeSnapshot;
              storeApi.getState().patchTaskInStore(taskId, storeRollback);
            }
          }
        }
        toastUpdateFailure(err);
        throw err;
      }
    },
    [ctx, storeApi],
  );
}

/**
 * Maps a Task patch to the subset of fields that exist on OfficeTask, so we
 * can keep both the local and store representations in sync.
 */
function toOfficeTaskPatch(patch: Partial<Task>): Partial<OfficeTask> {
  const out: Partial<OfficeTask> = {};
  if (patch.status !== undefined) out.status = patch.status;
  if (patch.priority !== undefined) out.priority = patch.priority;
  if (patch.assigneeAgentProfileId !== undefined) {
    out.assigneeAgentProfileId = patch.assigneeAgentProfileId;
  }
  // hasOwnProperty, not !== undefined: clearing the human assignee sends
  // undefined, and dropping it here would leave the stale name on screen
  // until the refetch lands.
  if (Object.prototype.hasOwnProperty.call(patch, "assigneeUserId")) {
    out.assigneeUserId = patch.assigneeUserId;
  }
  if (patch.projectId !== undefined) out.projectId = patch.projectId;
  if (Object.prototype.hasOwnProperty.call(patch, "parentId")) out.parentId = patch.parentId;
  if (patch.labels !== undefined) out.labels = patch.labels;
  if (patch.blockedBy !== undefined) out.blockedBy = patch.blockedBy;
  return out;
}

function toastUpdateFailure(err: unknown): void {
  const message = err instanceof Error ? err.message : t("task:updateFailed");
  toast.error(message);
}

/**
 * Commits a title edit (AC-3, AC-9, AC-11, AC-44, AC-53, AC-54, AC-55, AC-64,
 * AC-65). Optimistic at issue time: the local task and the Office task store
 * entry are patched with the trimmed draft before the request resolves. On
 * failure, restores the field's *currently recorded* canonical value — never
 * a commit-time snapshot — but only when `shouldRestoreAfterFailedWrite`
 * says this commit is still the "last word standing" for the field.
 */
export function useCommitTaskTitle() {
  const ctx = useTaskOptimisticContext();
  const storeApi = useAppStoreApi();

  return useCallback(
    async (taskId: string, trimmedTitle: string): Promise<void> => {
      const sequence = nextTaskSequence(taskId);
      beginWrite(taskId, "title", sequence);
      ctx.applyPatch({ title: trimmedTitle });
      storeApi.getState().patchTaskInStore(taskId, { title: trimmedTitle });

      try {
        const updated = await updateTask(taskId, { title: trimmedTitle });
        recordWriteSuccess(taskId, "title", updated.title, updated.updated_at, sequence);
        endWrite(taskId, "title", sequence);
      } catch (err) {
        const shouldRestore = shouldRestoreAfterFailedWrite(taskId, "title", sequence);
        endWrite(taskId, "title", sequence);
        if (shouldRestore) {
          const canonical = getCanonicalValue(taskId, "title");
          if (canonical !== undefined) {
            ctx.applyPatch({ title: canonical });
            storeApi.getState().patchTaskInStore(taskId, { title: canonical });
          }
        }
        toastUpdateFailure(err);
      }
    },
    [ctx, storeApi],
  );
}

/**
 * Commits a description save (AC-16, AC-17, AC-18, AC-42, AC-56). Not
 * optimistic: neither the local task nor the Office task store entry is
 * patched until the write succeeds, and a failure needs no rollback because
 * nothing was applied. The caller (the description editor) is responsible
 * for AC-17/AC-42's "did the draft change while the save was in flight"
 * comparison against the returned value.
 */
export function useCommitTaskDescription() {
  const ctx = useTaskOptimisticContext();
  const storeApi = useAppStoreApi();

  return useCallback(
    async (
      taskId: string,
      trimmedDescription: string,
    ): Promise<{ ok: true; value: string } | { ok: false }> => {
      const sequence = nextTaskSequence(taskId);
      beginWrite(taskId, "description", sequence);

      try {
        const updated = await updateTask(taskId, { description: trimmedDescription });
        const accepted = recordWriteSuccess(
          taskId,
          "description",
          updated.description,
          updated.updated_at,
          sequence,
        );
        endWrite(taskId, "description", sequence);
        if (!accepted) {
          // A newer write or refetch already superseded this response
          // (AC-60/AC-62): applying it here would show a stale description.
          // The already-recorded newer value reaches the page/store via the
          // subscribeField deferred-apply listener once the guard clears.
          const canonical = getCanonicalValue(taskId, "description") ?? updated.description;
          return { ok: true, value: canonical };
        }
        ctx.applyPatch({ description: updated.description });
        storeApi.getState().patchTaskInStore(taskId, { description: updated.description });
        return { ok: true, value: updated.description };
      } catch (err) {
        endWrite(taskId, "description", sequence);
        toastUpdateFailure(err);
        return { ok: false };
      }
    },
    [ctx, storeApi],
  );
}
