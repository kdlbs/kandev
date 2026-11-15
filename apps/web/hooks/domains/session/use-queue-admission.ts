import { useCallback } from "react";
import { notifyTurnErrorAcknowledgementFailure } from "@/lib/task-turn-error-feedback";
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
): Promise<{ acknowledgementFailed: boolean }> {
  let mutationError: unknown;
  let acknowledgementFailed = false;
  try {
    const accepted = await queueMessage(params);
    acknowledgementFailed = accepted.turn_error_acknowledgement_failed === true;
  } catch (error) {
    mutationError = error;
  }
  try {
    await refetch(params.session_id, token);
  } catch (reconcileError) {
    if (mutationError) throw mutationError;
    if (tolerateCommittedRefetchFailure) return { acknowledgementFailed };
    throw reconcileError;
  }
  if (mutationError) throw mutationError;
  return { acknowledgementFailed };
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
      previewFeedbackRefs,
      requirePrimarySession,
      observedTurnErrorStamp,
    }: QueueMessageInput) => {
      if (!identity || identity.task_id !== taskId) return false;
      const { session_id: sessionId, session_incarnation_id: incarnationId } = identity;
      const token = beginQueueOperation(sessionId, incarnationId);
      if (!token) return false;
      try {
        const queueResult = await queueWithTerminalRefetch(
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
            ...(previewFeedbackRefs?.length ? { preview_feedback_refs: previewFeedbackRefs } : {}),
            ...(requirePrimarySession ? { require_primary_session: true } : {}),
            ...(contextFilesMeta ? { context_files: contextFilesMeta } : {}),
            ...(observedTurnErrorStamp
              ? { observed_turn_error_stamp: observedTurnErrorStamp }
              : {}),
          },
          refetch,
          token,
          Boolean(clientQueueId),
        );
        notifyTurnErrorAcknowledgementFailure(queueResult.acknowledgementFailed);
        return true;
      } finally {
        finishQueueOperation(sessionId, token);
      }
    },
    [beginQueueOperation, finishQueueOperation, identity, refetch],
  );
}
