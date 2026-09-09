import { useCallback, useRef } from "react";
import { getQueueStatus } from "@/lib/api/domains/queue-api";
import type { QueueSessionIdentity } from "@/lib/api/domains/queue-api";
import type {
  QueueOperationToken,
  QueueStatus,
  SessionSlice,
} from "@/lib/state/slices/session/types";

type BeginQueueOperation = SessionSlice["beginQueueOperation"];
type FinishQueueOperation = SessionSlice["finishQueueOperation"];
type SetQueueEntries = SessionSlice["setQueueEntries"];

export type QueueRefetch = (sessionId: string, token?: QueueOperationToken) => Promise<void>;

function queueStatusMatchesIdentity(status: QueueStatus, identity: QueueSessionIdentity): boolean {
  return (
    status.task_id === identity.task_id &&
    status.session_id === identity.session_id &&
    status.session_incarnation_id === identity.session_incarnation_id
  );
}

function queueStatusMeta(status: QueueStatus) {
  return {
    count: status.count,
    max: status.max,
    mergeEnabled: status.merge_enabled ?? true,
    autoRun: status.auto_run ?? true,
    taskId: status.task_id,
    sessionIncarnationId: status.session_incarnation_id,
    ...(status.status_epoch !== undefined ? { statusEpoch: status.status_epoch } : {}),
    ...(status.status_generation !== undefined
      ? { statusGeneration: status.status_generation }
      : {}),
    ...(status.auto_merge_available !== undefined
      ? { autoMergeAvailable: status.auto_merge_available }
      : {}),
    ...(status.auto_merge_enabled !== undefined
      ? { autoMergeEnabled: status.auto_merge_enabled }
      : {}),
    ...(status.auto_merge_source !== undefined
      ? { autoMergeSource: status.auto_merge_source }
      : {}),
    ...(status.auto_merge_revision !== undefined
      ? { autoMergeRevision: status.auto_merge_revision }
      : {}),
  };
}

/** Fetch and reconcile one immutable session's authoritative queue snapshot. */
export function useQueueRefetch(
  identity: QueueSessionIdentity | null,
  setQueueEntries: SetQueueEntries,
  beginQueueOperation: BeginQueueOperation,
  finishQueueOperation: FinishQueueOperation,
) {
  const refetchVersion = useRef<Record<string, number>>({});
  const invalidate = useCallback((sessionId: string) => {
    refetchVersion.current[sessionId] = (refetchVersion.current[sessionId] ?? 0) + 1;
  }, []);
  const refetch = useCallback<QueueRefetch>(
    async (sessionId, callerToken) => {
      if (!identity || identity.session_id !== sessionId) return;
      const token =
        callerToken ?? beginQueueOperation(identity.session_id, identity.session_incarnation_id);
      if (!token) return;
      const ownsToken = callerToken === undefined;
      const version = (refetchVersion.current[sessionId] ?? 0) + 1;
      refetchVersion.current[sessionId] = version;
      try {
        const status = await getQueueStatus(identity);
        if (refetchVersion.current[sessionId] !== version) return;
        if (!queueStatusMatchesIdentity(status, identity)) return;
        setQueueEntries(sessionId, status.entries ?? [], queueStatusMeta(status), {
          establishStatusEpoch: true,
        });
      } finally {
        if (ownsToken) finishQueueOperation(sessionId, token);
      }
    },
    [beginQueueOperation, finishQueueOperation, identity, setQueueEntries],
  );
  return { refetch, invalidate };
}
