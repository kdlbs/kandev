"use client";

import { useCallback } from "react";
import { ApiError } from "@/lib/api/client";
import { fetchTask, fetchTaskByExternalId } from "@/lib/api";
import {
  classifyTaskLookup,
  taskDetailSessionId,
  withRenewedDiscardConsent,
  type NativeCreateObservation,
  type TaskCreationReceipt,
  type TaskLookupObservation,
} from "@/lib/task-creation-receipt";
import type { Task } from "@/lib/types/http";
import { sameReceipt, type ReceiptStore } from "@/components/task-creation-receipt-store";

export type RecoveryLookupResult = TaskLookupObservation | { kind: "error" };

export function useReceiptLookup() {
  return useCallback(async (candidate: TaskCreationReceipt) => {
    try {
      const response = await fetchTaskByExternalId(candidate.workspaceId, candidate.externalId);
      return classifyTaskLookup(response, candidate);
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) return { kind: "absent" } as const;
      return { kind: "error" } as const;
    }
  }, []);
}

export function useReceiptOutcomes({ store }: { store: ReceiptStore }) {
  const applyLookupMiss = useCallback(
    (candidate: TaskCreationReceipt) => {
      // A miss or read failure does not change what an already observed task
      // is. Keep its accepted, incomplete, or unavailable state and only mark
      // the submission as still uncertain.
      return store.patchRetained(
        candidate,
        candidate.taskId ? { everUncertain: true } : { state: "unknown", everUncertain: true },
      );
    },
    [store],
  );

  const applyLookupResult = useCallback(
    (candidate: TaskCreationReceipt, observation: RecoveryLookupResult) => {
      if (observation.kind === "absent") return applyLookupMiss(candidate);
      if (observation.kind === "error" || observation.kind === "mismatch") {
        return applyLookupMiss(candidate);
      }
      return store.patchRetained(candidate, {
        state: observation.kind,
        creationComplete: observation.kind === "accepted",
        taskId: observation.taskId,
      });
    },
    [applyLookupMiss, store],
  );

  const applyCreateResult = useCallback(
    (candidate: TaskCreationReceipt, observation: NativeCreateObservation) => {
      if (observation.kind === "unknown") {
        const patch: Partial<TaskCreationReceipt> = {
          state: "unknown",
          everUncertain: true,
        };
        if (observation.taskId) {
          patch.taskId = observation.taskId;
          patch.replayable = false;
        }
        return store.patchRetained(candidate, patch);
      }
      return store.patchRetained(candidate, {
        state: observation.kind,
        everUncertain: true,
        taskId: observation.taskId,
        creationComplete: observation.kind === "accepted",
        taskSessionId: observation.taskSessionId ?? candidate.taskSessionId,
        queuedForStepId: observation.queuedForStepId ?? candidate.queuedForStepId,
      });
    },
    [store],
  );

  const updateConsent = useCallback(
    (candidate: TaskCreationReceipt, consentedDirtyFiles: string[]) => {
      return (
        store.patchRetained(candidate, {
          payload: withRenewedDiscardConsent(candidate.payload, consentedDirtyFiles),
        }) ?? candidate
      );
    },
    [store],
  );

  return { applyLookupMiss, applyLookupResult, applyCreateResult, updateConsent };
}

export function useReceiptHydration({ store }: { store: ReceiptStore }) {
  const hydrateTask = useCallback(
    async (candidate: TaskCreationReceipt): Promise<Task | null> => {
      if (!candidate.taskId) return null;
      try {
        return await fetchTask(candidate.taskId);
      } catch (error) {
        if (error instanceof ApiError && (error.status === 404 || error.status === 403)) {
          store.patchRetained(candidate, { state: "unavailable", everUncertain: true });
        }
        return null;
      }
    },
    [store],
  );

  const hydrateSessionId = useCallback(
    async (candidate: TaskCreationReceipt): Promise<string | null> => {
      const current = store.receiptRef.current;
      if (current && sameReceipt(current, candidate) && current.taskSessionId) {
        return current.taskSessionId;
      }
      const task = await hydrateTask(candidate);
      if (!task) return null;
      const sessionId = taskDetailSessionId(task);
      if (sessionId) store.patchRetained(candidate, { taskSessionId: sessionId });
      return sessionId;
    },
    [hydrateTask, store],
  );

  return { hydrateTask, hydrateSessionId };
}
