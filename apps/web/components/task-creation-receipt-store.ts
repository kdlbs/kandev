"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  createTaskCreationReceipt,
  loadTaskCreationReceipt,
  receiptStorageKey,
  removeTaskCreationReceipt,
  saveTaskCreationReceipt,
  type TaskCreationReceipt,
  type TaskCreationScope,
} from "@/lib/task-creation-receipt";
import { generateUUID } from "@/lib/uuid";
import { createDebugLogger } from "@/lib/debug/log";
import type { createTask } from "@/lib/api";
import type { RecoveryFence } from "@/components/task-creation-recovery-fence";

type CreateTaskParams = Parameters<typeof createTask>[0];
const recoveryDebug = createDebugLogger("task-create:recovery");

export type BeginSubmissionResult = {
  receipt: TaskCreationReceipt;
  retrying: boolean;
  /** Owner/open-cycle generation that admitted this submission. */
  generation: number;
};

export function receiptMatchesScope(
  receipt: TaskCreationReceipt,
  scope: TaskCreationScope,
): boolean {
  return (
    receipt.userId === scope.userId &&
    receipt.workspaceId === scope.workspaceId &&
    receipt.draftId === scope.draftId
  );
}

export function sameReceipt(left: TaskCreationReceipt, right: TaskCreationReceipt): boolean {
  return (
    left.submissionId === right.submissionId &&
    left.userId === right.userId &&
    left.workspaceId === right.workspaceId &&
    left.draftId === right.draftId
  );
}

function useReceiptRestore(args: {
  enabled: boolean;
  open: boolean;
  userId: string | null;
  workspaceId: string | null;
  draftId: string;
  receiptRef: React.RefObject<TaskCreationReceipt | null>;
  memoryReceiptsRef: React.RefObject<Map<string, TaskCreationReceipt>>;
  setReceipt: React.Dispatch<React.SetStateAction<TaskCreationReceipt | null>>;
  setStorageUnavailable: React.Dispatch<React.SetStateAction<boolean>>;
  readRetained: (target: TaskCreationScope) => TaskCreationReceipt | null;
}) {
  const {
    enabled,
    open,
    userId,
    workspaceId,
    draftId,
    receiptRef,
    memoryReceiptsRef,
    setReceipt,
    setStorageUnavailable,
    readRetained,
  } = args;
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
  }, [
    enabled,
    open,
    userId,
    workspaceId,
    draftId,
    readRetained,
    receiptRef,
    memoryReceiptsRef,
    setReceipt,
    setStorageUnavailable,
  ]);
}

export function useReceiptPersistence(args: {
  enabled: boolean;
  open: boolean;
  userId: string | null;
  workspaceId: string | null;
  draftId: string;
}) {
  const { enabled, open, userId, workspaceId, draftId } = args;
  const [receipt, setReceipt] = useState<TaskCreationReceipt | null>(null);
  const [storageUnavailable, setStorageUnavailable] = useState(false);
  const receiptRef = useRef<TaskCreationReceipt | null>(null);
  const memoryReceiptsRef = useRef(new Map<string, TaskCreationReceipt>());

  const scope = useCallback(
    (): TaskCreationScope | null => (workspaceId ? { userId, workspaceId, draftId } : null),
    [userId, workspaceId, draftId],
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

  useReceiptRestore({
    enabled,
    open,
    userId,
    workspaceId,
    draftId,
    receiptRef,
    memoryReceiptsRef,
    setReceipt,
    setStorageUnavailable,
    readRetained,
  });
  return {
    receipt,
    storageUnavailable,
    receiptRef,
    scope,
    publish,
    readRetained,
    patchRetained,
    retire,
  };
}

export function useSubmissionGuard() {
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
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
  return { busy, beginOperation, endOperation };
}

export function useReceiptLedger(args: {
  fence: RecoveryFence;
  store: ReturnType<typeof useReceiptPersistence>;
}) {
  const { fence, store } = args;
  const guard = useSubmissionGuard();

  const beginSubmission = useCallback(
    (payload: CreateTaskParams, draftTitle?: string): BeginSubmissionResult | null => {
      const currentScope = store.scope();
      if (!currentScope) return null;
      const current = store.receiptRef.current;
      const retained =
        current && receiptMatchesScope(current, currentScope)
          ? current
          : store.readRetained(currentScope);
      if (retained) {
        if (current !== retained) {
          store.receiptRef.current = retained;
          store.publish(retained);
        }
        return { receipt: retained, retrying: true, generation: fence.getGeneration() };
      }
      const next = createTaskCreationReceipt({
        scope: currentScope,
        payload,
        runtimeId: generateUUID(),
        draftTitle,
      });
      store.publish(next);
      return { receipt: next, retrying: false, generation: fence.getGeneration() };
    },
    [fence, store],
  );

  const acknowledge = useCallback(
    (candidate: TaskCreationReceipt) => {
      if (candidate.state === "accepted") store.retire(candidate);
    },
    [store],
  );

  const markUncertain = useCallback(
    (candidate: TaskCreationReceipt) => {
      store.patchRetained(candidate, { state: "unknown", everUncertain: true });
    },
    [store],
  );

  const markHandoffAttempted = useCallback(
    (candidate: TaskCreationReceipt) => {
      store.patchRetained(candidate, { handoffAttempted: true });
    },
    [store],
  );

  return {
    receipt: store.receipt,
    storageUnavailable: store.storageUnavailable,
    ...guard,
    beginSubmission,
    markUncertain,
    markHandoffAttempted,
    acknowledge,
    retire: store.retire,
    getReceipt: () => store.receiptRef.current,
  };
}

export type ReceiptStore = ReturnType<typeof useReceiptPersistence>;
export type ReceiptLedger = ReturnType<typeof useReceiptLedger>;
