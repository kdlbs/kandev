import { useCallback } from "react";
import { queueMessage } from "@/lib/api/domains/queue-api";
import type { QueueMessageParams, QueueSessionIdentity } from "@/lib/api/domains/queue-api";
import type { QueueOperationToken, SessionSlice } from "@/lib/state/slices/session/types";
import type { QueueRefetch } from "./use-queue-refetch";
import type { QueueMessageInput } from "./use-queue-types";

type BeginQueueOperation = SessionSlice["beginQueueOperation"];
type FinishQueueOperation = SessionSlice["finishQueueOperation"];

async function queueWithTerminalRefetch(
  params: QueueMessageParams,
  refetch: QueueRefetch,
  token: QueueOperationToken,
): Promise<void> {
  let mutationError: unknown;
  try {
    await queueMessage(params);
  } catch (error) {
    mutationError = error;
  }
  try {
    await refetch(params.session_id, token);
  } catch (reconcileError) {
    if (mutationError) throw mutationError;
    throw reconcileError;
  }
  if (mutationError) throw mutationError;
}

export function useQueueAdmissionAction(
  identity: QueueSessionIdentity | null,
  beginQueueOperation: BeginQueueOperation,
  finishQueueOperation: FinishQueueOperation,
  refetch: QueueRefetch,
) {
  return useCallback(
    async ({
      taskId,
      content,
      model,
      planMode,
      attachments,
      entityReferences,
      contextFilesMeta,
    }: QueueMessageInput) => {
      if (!identity || identity.task_id !== taskId) return;
      const { session_id: sessionId, session_incarnation_id: incarnationId } = identity;
      const token = beginQueueOperation(sessionId, incarnationId);
      if (!token) return;
      try {
        await queueWithTerminalRefetch(
          {
            session_id: sessionId,
            session_incarnation_id: incarnationId,
            task_id: taskId,
            content,
            model,
            plan_mode: planMode,
            attachments,
            entity_references: entityReferences,
            ...(contextFilesMeta ? { context_files: contextFilesMeta } : {}),
          },
          refetch,
          token,
        );
      } finally {
        finishQueueOperation(sessionId, token);
      }
    },
    [beginQueueOperation, finishQueueOperation, identity, refetch],
  );
}
