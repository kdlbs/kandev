import { useCallback } from "react";
import { sendQueuedNow, setQueueAutoRun, setQueueAutoMerge } from "@/lib/api/domains/queue-api";
import type { QueueSessionIdentity } from "@/lib/api/domains/queue-api";
import type { SessionSlice } from "@/lib/state/slices/session/types";
import type { QueueRefetch } from "./use-queue-refetch";

type BeginQueueOperation = SessionSlice["beginQueueOperation"];
type FinishQueueOperation = SessionSlice["finishQueueOperation"];
async function runQueueMutation(
  identity: QueueSessionIdentity | null,
  beginQueueOperation: BeginQueueOperation,
  finishQueueOperation: FinishQueueOperation,
  refetch: QueueRefetch,
  mutate: (identity: QueueSessionIdentity) => Promise<unknown>,
): Promise<void> {
  if (!identity) return;
  const { session_id: sessionId, session_incarnation_id: incarnationId } = identity;
  const token = beginQueueOperation(sessionId, incarnationId);
  if (!token) return;
  let mutationError: unknown;
  try {
    try {
      await mutate(identity);
    } catch (error) {
      mutationError = error;
    }
    try {
      await refetch(sessionId, token);
    } catch (reconcileError) {
      if (mutationError) throw mutationError;
      throw reconcileError;
    }
    if (mutationError) throw mutationError;
  } finally {
    finishQueueOperation(sessionId, token);
  }
}

export function useSendNowAction(
  identity: QueueSessionIdentity | null,
  beginQueueOperation: BeginQueueOperation,
  finishQueueOperation: FinishQueueOperation,
  refetch: QueueRefetch,
) {
  return useCallback(
    (entryId: string) =>
      runQueueMutation(identity, beginQueueOperation, finishQueueOperation, refetch, (current) =>
        sendQueuedNow({ ...current, scope: "entry", entry_id: entryId }),
      ),
    [beginQueueOperation, finishQueueOperation, identity, refetch],
  );
}

type BooleanQueueMutation = (identity: QueueSessionIdentity, enabled: boolean) => Promise<unknown>;

function useBooleanQueueAction(
  identity: QueueSessionIdentity | null,
  beginQueueOperation: BeginQueueOperation,
  finishQueueOperation: FinishQueueOperation,
  refetch: QueueRefetch,
  mutate: BooleanQueueMutation,
) {
  return useCallback(
    (enabled: boolean) =>
      runQueueMutation(identity, beginQueueOperation, finishQueueOperation, refetch, (current) =>
        mutate(current, enabled),
      ),
    [beginQueueOperation, finishQueueOperation, identity, mutate, refetch],
  );
}

export function useQueueAutomationActions({
  identity,
  beginQueueOperation,
  finishQueueOperation,
  refetch,
}: {
  identity: QueueSessionIdentity | null;
  beginQueueOperation: BeginQueueOperation;
  finishQueueOperation: FinishQueueOperation;
  refetch: QueueRefetch;
}) {
  const setAutoRun = useBooleanQueueAction(
    identity,
    beginQueueOperation,
    finishQueueOperation,
    refetch,
    setQueueAutoRun,
  );
  const setAutoMerge = useBooleanQueueAction(
    identity,
    beginQueueOperation,
    finishQueueOperation,
    refetch,
    setQueueAutoMerge,
  );
  return { setAutoRun, setAutoMerge };
}
