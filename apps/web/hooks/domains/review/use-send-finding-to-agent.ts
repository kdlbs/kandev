"use client";

import { useCallback } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { appendToQueue } from "@/lib/api/domains/queue-api";
import { formatFindingAsMarkdown } from "@/lib/review/format";
import type { TaskReviewFinding } from "@/lib/types/review";
import type { Message } from "@/lib/types/http";
import { getWebSocketClient } from "@/lib/ws/connection";
import { generateUUID } from "@/lib/utils";

type Params = {
  taskId: string | null | undefined;
  sessionId: string | null | undefined;
};

function isAgentBusy(state: string | undefined): boolean {
  return state === "STARTING" || state === "RUNNING";
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

  return useCallback(
    async (finding: TaskReviewFinding) => {
      if (!taskId || !sessionId) return;
      const state = storeApi.getState();
      const session = state.taskSessions.items[sessionId] ?? null;
      const content = formatFindingAsMarkdown(finding);
      const planMode = state.chatInput.planModeBySessionId[sessionId] ?? false;

      try {
        if (isAgentBusy(session?.state)) {
          await appendToQueue({
            session_id: sessionId,
            task_id: taskId,
            content,
            ...(planMode ? { plan_mode: true } : {}),
          });
          toast({ title: "Finding queued for the agent", variant: "success" });
          return;
        }
        const client = getWebSocketClient();
        if (!client) throw new Error("WebSocket client unavailable");
        const created = await client.request<Message | undefined>(
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
        // Add the returned message directly so the chat updates even if the
        // broadcast is missed; addMessage is idempotent on id.
        if (created?.id && created.session_id) storeApi.getState().addMessage(created);
        toast({ title: "Finding sent to the agent", variant: "success" });
      } catch (error) {
        toast({
          title: "Could not send the finding",
          description: error instanceof Error ? error.message : "An error occurred",
          variant: "error",
        });
      }
    },
    [taskId, sessionId, storeApi, toast],
  );
}
