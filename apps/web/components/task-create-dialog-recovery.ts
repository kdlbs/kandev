"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import { fetchTask, fetchTaskByExternalId } from "@/lib/api";
import {
  classifyTaskLookup,
  createTaskCreationReceipt,
  loadTaskCreationReceipt,
  receiptStorageKey,
  removeTaskCreationReceipt,
  saveTaskCreationReceipt,
  taskDetailSessionId,
  withRenewedDiscardConsent,
  type NativeCreateObservation,
  type TaskCreationReceipt,
  type TaskCreationScope,
  type TaskLookupObservation,
} from "@/lib/task-creation-receipt";
import { generateUUID } from "@/lib/uuid";
import { createDebugLogger } from "@/lib/debug/log";
import type { createTask } from "@/lib/api";
import type { Task } from "@/lib/types/http";

type CreateTaskParams = Parameters<typeof createTask>[0];

const recoveryDebug = createDebugLogger("task-create:recovery");

export type RecoveryLookupResult = TaskLookupObservation | { kind: "error" };

export type BeginSubmissionResult = {
  receipt: TaskCreationReceipt;
  retrying: boolean;
  /** Owner/open-cycle generation that admitted this submission. */
  generation: number;
};

function receiptMatchesScope(receipt: TaskCreationReceipt, scope: TaskCreationScope): boolean {
  return (
    receipt.userId === scope.userId &&
    receipt.workspaceId === scope.workspaceId &&
    receipt.draftId === scope.draftId
  );
}

function sameReceipt(left: TaskCreationReceipt, right: TaskCreationReceipt): boolean {
  return (
    left.submissionId === right.submissionId &&
    left.userId === right.userId &&
    left.workspaceId === right.workspaceId &&
    left.draftId === right.draftId
  );
}

/**
 * Owns one native creation receipt: the in-memory guard, the exact captured
 * payload, persistence scoped to user/workspace/draft, and read-only lookup.
 * Acceptance callbacks stay with the submit module so this hook never
 * launches, navigates, or clears a draft by itself.
 */
// eslint-disable-next-line max-lines-per-function -- one receipt owner keeps identity, persistence, and owner fencing in one place.
export function useTaskCreationRecovery(args: {
  enabled: boolean;
  open: boolean;
  userId: string | null;
  workspaceId: string | null;
  draftId: string;
}) {
  const { enabled, open, userId, workspaceId, draftId } = args;
  const [receipt, setReceipt] = useState<TaskCreationReceipt | null>(null);
  const [storageUnavailable, setStorageUnavailable] = useState(false);
  const [busy, setBusy] = useState(false);
  const receiptRef = useRef<TaskCreationReceipt | null>(null);
  const busyRef = useRef(false);
  const memoryReceiptsRef = useRef(new Map<string, TaskCreationReceipt>());
  const ownerKey = `${enabled}:${open}:${userId ?? ""}:${workspaceId ?? ""}:${draftId}`;
  const ownerKeyRef = useRef(ownerKey);
  const generationRef = useRef(0);
  if (ownerKeyRef.current !== ownerKey) {
    ownerKeyRef.current = ownerKey;
    generationRef.current += 1;
  }

  const scope = useCallback(
    (): TaskCreationScope | null => (workspaceId ? { userId, workspaceId, draftId } : null),
    [userId, workspaceId, draftId],
  );

  const getGeneration = useCallback(() => generationRef.current, []);

  const isOwnerCurrent = useCallback(
    (generation: number) => generation === generationRef.current && open && enabled,
    [enabled, open],
  );

  const publish = useCallback((next: TaskCreationReceipt | null) => {
    receiptRef.current = next;
    setReceipt(next);
    if (!next) return;
    memoryReceiptsRef.current.set(receiptStorageKey(next), next);
    recoveryDebug("receipt.state", {
      state: next.state,
      task_id: next.taskId,
      replayable: next.replayable,
      uncertain: next.everUncertain,
      handoff_attempted: next.handoffAttempted,
    });
    const retained = saveTaskCreationReceipt(next);
    setStorageUnavailable(!retained);
  }, []);

  const readRetained = useCallback((target: TaskCreationScope) => {
    const key = receiptStorageKey(target);
    const inMemory = memoryReceiptsRef.current.get(key);
    if (inMemory) return inMemory;
    const stored = loadTaskCreationReceipt(target);
    if (stored) memoryReceiptsRef.current.set(key, stored);
    return stored;
  }, []);

  const patchRetained = useCallback(
    (candidate: TaskCreationReceipt, patch: Partial<TaskCreationReceipt>) => {
      const current = receiptRef.current;
      if (current && sameReceipt(current, candidate)) {
        const next: TaskCreationReceipt = {
          ...current,
          ...patch,
          updatedAt: new Date().toISOString(),
        };
        publish(next);
        return next;
      }
      const retained = readRetained(candidate);
      if (!retained || !sameReceipt(retained, candidate)) return null;
      const next: TaskCreationReceipt = {
        ...retained,
        ...patch,
        updatedAt: new Date().toISOString(),
      };
      memoryReceiptsRef.current.set(receiptStorageKey(next), next);
      saveTaskCreationReceipt(next);
      return next;
    },
    [publish, readRetained],
  );

  const isCurrent = useCallback(
    (candidate: TaskCreationReceipt, generation?: number) => {
      const current = receiptRef.current;
      const same = Boolean(current && sameReceipt(current, candidate));
      return same && (generation === undefined || isOwnerCurrent(generation));
    },
    [isOwnerCurrent],
  );

  useEffect(() => {
    return () => {
      generationRef.current += 1;
    };
  }, []);

  useEffect(() => {
    if (!enabled) {
      receiptRef.current = null;
      setReceipt(null);
      return;
    }
    if (!open || !workspaceId) return;
    const target = { userId, workspaceId, draftId };
    const current = receiptRef.current;
    const preferred =
      current && receiptMatchesScope(current, target) ? current : readRetained(target);
    if (!preferred) {
      receiptRef.current = null;
      setReceipt(null);
      return;
    }
    // A reload cannot know whether the persisted in-flight request allocated
    // a task, so it becomes uncertain rather than becoming create permission.
    const restored: TaskCreationReceipt =
      preferred.state === "submitting"
        ? {
            ...preferred,
            state: "unknown",
            everUncertain: true,
            updatedAt: new Date().toISOString(),
          }
        : preferred;
    receiptRef.current = restored;
    setReceipt(restored);
    memoryReceiptsRef.current.set(receiptStorageKey(restored), restored);
    const retained = saveTaskCreationReceipt(restored);
    setStorageUnavailable(!retained);
  }, [enabled, open, userId, workspaceId, draftId, readRetained]);

  const beginOperation = useCallback(() => {
    if (busyRef.current) return false;
    busyRef.current = true;
    setBusy(true);
    return true;
  }, []);

  const endOperation = useCallback(() => {
    busyRef.current = false;
    setBusy(false);
  }, []);

  const beginSubmission = useCallback(
    (payload: CreateTaskParams): BeginSubmissionResult | null => {
      const currentScope = scope();
      if (!currentScope) return null;
      const current = receiptRef.current;
      const retained =
        current && receiptMatchesScope(current, currentScope)
          ? current
          : readRetained(currentScope);
      if (retained) {
        if (current !== retained) {
          receiptRef.current = retained;
          setReceipt(retained);
        }
        return { receipt: retained, retrying: true, generation: generationRef.current };
      }
      const next = createTaskCreationReceipt({
        scope: currentScope,
        payload,
        runtimeId: generateUUID(),
      });
      publish(next);
      return { receipt: next, retrying: false, generation: generationRef.current };
    },
    [publish, readRetained, scope],
  );

  const markUncertain = useCallback(
    (candidate: TaskCreationReceipt) => {
      patchRetained(candidate, { state: "unknown", everUncertain: true });
    },
    [patchRetained],
  );

  const markUnavailable = useCallback(
    (candidate: TaskCreationReceipt) => {
      patchRetained(candidate, { state: "unavailable", everUncertain: true });
    },
    [patchRetained],
  );

  const markHandoffAttempted = useCallback(
    (candidate: TaskCreationReceipt) => {
      patchRetained(candidate, { handoffAttempted: true });
    },
    [patchRetained],
  );

  const retire = useCallback((candidate: TaskCreationReceipt) => {
    recoveryDebug("receipt.retired", {
      state: candidate.state,
      task_id: candidate.taskId,
    });
    memoryReceiptsRef.current.delete(receiptStorageKey(candidate));
    removeTaskCreationReceipt(candidate);
    const current = receiptRef.current;
    if (current && sameReceipt(current, candidate)) {
      receiptRef.current = null;
      setReceipt(null);
    }
  }, []);

  const acknowledge = useCallback(
    (candidate: TaskCreationReceipt) => {
      if (candidate.state === "accepted") retire(candidate);
    },
    [retire],
  );

  const lookup = useCallback(async (candidate: TaskCreationReceipt) => {
    try {
      const response = await fetchTaskByExternalId(candidate.workspaceId, candidate.externalId);
      return classifyTaskLookup(response, candidate);
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) return { kind: "absent" } as const;
      return { kind: "error" } as const;
    }
  }, []);

  const applyLookupMiss = useCallback(
    (candidate: TaskCreationReceipt) => {
      return patchRetained(candidate, { state: "unknown", everUncertain: true });
    },
    [patchRetained],
  );

  const applyLookupResult = useCallback(
    (candidate: TaskCreationReceipt, observation: RecoveryLookupResult) => {
      if (observation.kind === "absent") return applyLookupMiss(candidate);
      if (observation.kind === "error" || observation.kind === "mismatch") {
        return applyLookupMiss(candidate);
      }
      return patchRetained(candidate, {
        state: observation.kind,
        creationComplete: observation.kind === "accepted",
        taskId: observation.taskId,
      });
    },
    [applyLookupMiss, patchRetained],
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
        return patchRetained(candidate, patch);
      }
      return patchRetained(candidate, {
        state: observation.kind,
        everUncertain: true,
        taskId: observation.taskId,
        creationComplete: observation.kind === "accepted",
        taskSessionId: observation.taskSessionId ?? candidate.taskSessionId,
        queuedForStepId: observation.queuedForStepId ?? candidate.queuedForStepId,
      });
    },
    [patchRetained],
  );

  const updateConsent = useCallback(
    (candidate: TaskCreationReceipt, consentedDirtyFiles: string[]) => {
      return (
        patchRetained(candidate, {
          payload: withRenewedDiscardConsent(candidate.payload, consentedDirtyFiles),
        }) ?? candidate
      );
    },
    [patchRetained],
  );

  /** Reads an existing task's detail; a missing task becomes unavailable. */
  const hydrateTask = useCallback(
    async (candidate: TaskCreationReceipt): Promise<Task | null> => {
      if (!candidate.taskId) return null;
      try {
        return await fetchTask(candidate.taskId);
      } catch (error) {
        if (error instanceof ApiError && (error.status === 404 || error.status === 403)) {
          markUnavailable(candidate);
        }
        return null;
      }
    },
    [markUnavailable],
  );

  const hydrateSessionId = useCallback(
    async (candidate: TaskCreationReceipt): Promise<string | null> => {
      const current = receiptRef.current;
      if (current && sameReceipt(current, candidate) && current.taskSessionId) {
        return current.taskSessionId;
      }
      const task = await hydrateTask(candidate);
      if (!task) return null;
      const sessionId = taskDetailSessionId(task);
      if (sessionId) patchRetained(candidate, { taskSessionId: sessionId });
      return sessionId;
    },
    [hydrateTask, patchRetained],
  );

  return {
    receipt,
    busy,
    storageUnavailable,
    beginOperation,
    endOperation,
    beginSubmission,
    markUncertain,
    markHandoffAttempted,
    acknowledge,
    retire,
    lookup,
    applyLookupResult,
    applyLookupMiss,
    applyCreateResult,
    updateConsent,
    hydrateTask,
    hydrateSessionId,
    isCurrent,
    isOwnerCurrent,
    getGeneration,
    getReceipt: () => receiptRef.current,
  };
}

export type TaskCreateRecovery = ReturnType<typeof useTaskCreationRecovery>;

/** The submit-owner projection rendered by the dialog status and footer. */
export type TaskCreateRecoveryView = {
  receipt: TaskCreationReceipt | null;
  busy: boolean;
  storageUnavailable: boolean;
  checkStatus: () => Promise<void>;
  retry: () => Promise<void>;
  openTask: () => Promise<void>;
  close: () => void;
  canRetry: boolean;
  canOpen: boolean;
  canCheck: boolean;
};
