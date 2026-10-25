"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { appendToQueue } from "@/lib/api/domains/queue-api";
import { formatFindingAsMarkdown } from "@/lib/review/format";
import type { TaskReviewFinding } from "@/lib/types/review";
import type { Message, TaskSession } from "@/lib/types/http";
import { getWebSocketClient } from "@/lib/ws/connection";
import { generateUUID } from "@/lib/utils";

type Params = {
  taskId: string | null | undefined;
  sessionId: string | null | undefined;
};

function isAgentBusy(state: string | undefined): boolean {
  return state === "STARTING" || state === "RUNNING";
}

async function queueFinding(
  taskId: string,
  session: TaskSession | null,
  sessionId: string,
  content: string,
  planMode: boolean,
) {
  if (!session?.queue_incarnation_id) {
    throw new Error("Session is not available for input");
  }
  await appendToQueue({
    session_id: sessionId,
    session_incarnation_id: session.queue_incarnation_id,
    task_id: taskId,
    content,
    ...(planMode ? { plan_mode: true } : {}),
  });
}

async function sendFinding(taskId: string, sessionId: string, content: string, planMode: boolean) {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket client unavailable");
  return client.request<Message | undefined>(
    "message.add",
    {
      task_id: taskId,
      session_id: sessionId,
      client_message_id: generateUUID(),
      content,
      has_review_comments: true,
      ...(planMode ? { plan_mode: true } : {}),
    },
    10000,
  );
}

/**
 * Sends a finding to the active session as follow-up context.
 *
 * The finding stays `open`: handing it to an agent is not the same as the human
 * accepting it, so only an explicit Resolve or Dismiss changes its status.
 * Follows the same busy-queue / direct-send split as diff comments.
 */
export function useSendFindingToAgent({ taskId, sessionId }: Params) {
  const storeApi = useAppStoreApi();
  const { toast } = useToast();
  const { t } = useTranslation("review");

  return useCallback(
    async (finding: TaskReviewFinding) => {
      if (!taskId || !sessionId) return;
      const state = storeApi.getState();
      const session = state.taskSessions.items[sessionId] ?? null;
      const content = formatFindingAsMarkdown(finding);
      const planMode = state.chatInput.planModeBySessionId[sessionId] ?? false;

      try {
        if (isAgentBusy(session?.state)) {
          await queueFinding(taskId, session, sessionId, content, planMode);
          toast({ title: t("review:findingQueuedForAgent"), variant: "success" });
          return;
        }
        const created = await sendFinding(taskId, sessionId, content, planMode);
        if (created?.id && created.session_id) storeApi.getState().addMessage(created);
        toast({ title: t("review:findingSentToAgent"), variant: "success" });
      } catch (error) {
        toast({
          title: t("review:couldNotSendFinding"),
          description: error instanceof Error ? error.message : t("common:anErrorOccurred"),
          variant: "error",
        });
      }
    },
    [taskId, sessionId, storeApi, t, toast],
  );
}
