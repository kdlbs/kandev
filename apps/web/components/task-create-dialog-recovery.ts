"use client";

import { useCallback } from "react";
import {
  sameReceipt,
  useReceiptLedger,
  useReceiptPersistence,
  type BeginSubmissionResult,
} from "@/components/task-creation-receipt-store";
import {
  useReceiptHydration,
  useReceiptLookup,
  useReceiptOutcomes,
  type RecoveryLookupResult,
} from "@/components/task-creation-receipt-resolution";
import { useRecoveryFence } from "@/components/task-creation-recovery-fence";
import type { TaskCreationReceipt } from "@/lib/task-creation-receipt";

export type { BeginSubmissionResult, RecoveryLookupResult };

/**
 * Owns one native creation receipt: the in-memory guard, the exact captured
 * payload, persistence scoped to user/workspace/draft, and read-only lookup.
 * Acceptance callbacks stay with the submit module so this hook never
 * launches, navigates, or clears a draft by itself.
 */
export function useTaskCreationRecovery(args: {
  enabled: boolean;
  open: boolean;
  userId: string | null;
  workspaceId: string | null;
  draftId: string;
}) {
  const fence = useRecoveryFence(args);
  const store = useReceiptPersistence(args);
  const ledger = useReceiptLedger({ fence, store });
  const outcomes = useReceiptOutcomes({ store });
  const lookup = useReceiptLookup();
  const hydration = useReceiptHydration({ store });

  const isCurrent = useCallback(
    (candidate: TaskCreationReceipt, generation?: number) => {
      const current = store.receiptRef.current;
      const same = Boolean(current && sameReceipt(current, candidate));
      return same && (generation === undefined || fence.isOwnerCurrent(generation));
    },
    [fence, store],
  );

  return {
    ...ledger,
    ...outcomes,
    ...hydration,
    lookup,
    isCurrent,
    isOwnerCurrent: fence.isOwnerCurrent,
    getGeneration: fence.getGeneration,
  };
}

export type TaskCreateRecovery = ReturnType<typeof useTaskCreationRecovery>;

/** Whether the dialog must replace ordinary create actions with recovery. */
export function isRecoverySubmitBlocked(receipt: TaskCreationReceipt | null): boolean {
  return Boolean(receipt && receipt.state !== "submitting");
}

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
