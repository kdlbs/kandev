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
  tolerateCommittedRefetchFailure: boolean,
): Promise<true> {
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
    if (tolerateCommittedRefetchFailure) return true;
    throw reconcileError;
  }
  if (mutationError) throw mutationError;
  return true;
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
      clientQueueId,
      planCommentRefs,
      requirePrimarySession,
    }: QueueMessageInput) => {
      if (!identity || identity.task_id !== taskId) return false;
      const { session_id: sessionId, session_incarnation_id: incarnationId } = identity;
      const token = beginQueueOperation(sessionId, incarnationId);
      if (!token) return false;
      try {
        return await queueWithTerminalRefetch(
          {
            session_id: sessionId,
            session_incarnation_id: incarnationId,
            task_id: taskId,
            content,
            model,
            plan_mode: planMode,
            attachments,
            entity_references: entityReferences,
            ...(clientQueueId ? { client_queue_id: clientQueueId } : {}),
            ...(planCommentRefs?.length ? { plan_comment_refs: planCommentRefs } : {}),
            ...(requirePrimarySession ? { require_primary_session: true } : {}),
            ...(contextFilesMeta ? { context_files: contextFilesMeta } : {}),
          },
          refetch,
          token,
          Boolean(clientQueueId),
        );
      } finally {
        finishQueueOperation(sessionId, token);
      }
    },
    [beginQueueOperation, finishQueueOperation, identity, refetch],
  );
}
