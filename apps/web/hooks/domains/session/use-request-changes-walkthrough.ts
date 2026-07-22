import { useCallback } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { listPrompts } from "@/lib/api";
import { queueMessage } from "@/lib/api/domains/queue-api";
import { getWebSocketClient } from "@/lib/ws/connection";
import {
  buildChangesWalkthroughPrompt,
  CHANGES_WALKTHROUGH_PROMPT_NAME,
} from "@/lib/walkthrough-request";
import type { Message } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import { deriveSessionInputMode } from "./session-input-mode";

type UseRequestChangesWalkthroughParams = {
  taskId: string | null | undefined;
  sessionId: string | null | undefined;
  ready?: boolean;
};

function planModePayload(enabled: boolean): { plan_mode?: true } {
  return enabled ? { plan_mode: true } : {};
}

async function loadChangesWalkthroughPromptTemplate(): Promise<string> {
  const { prompts } = await listPrompts({ cache: "no-store" });
  const prompt = prompts.find((p) => p.name === CHANGES_WALKTHROUGH_PROMPT_NAME);
  const content = prompt?.content?.trim();
  if (!content) {
    throw new Error(`${CHANGES_WALKTHROUGH_PROMPT_NAME} prompt is not available`);
  }
  return content;
}

async function queueWalkthroughRequest(params: {
  taskId: string;
  sessionId: string;
  content: string;
  planModeEnabled: boolean;
}) {
  await queueMessage({
    session_id: params.sessionId,
    task_id: params.taskId,
    content: params.content,
    ...planModePayload(params.planModeEnabled),
  });
}

async function sendWalkthroughRequest(params: {
  taskId: string;
  sessionId: string;
  content: string;
  planModeEnabled: boolean;
  state: AppState;
}) {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket client unavailable");
  const created = await client.request<Message | undefined>(
    "message.add",
    {
      task_id: params.taskId,
      session_id: params.sessionId,
      content: params.content,
      ...planModePayload(params.planModeEnabled),
    },
    10000,
  );
  if (created?.id && created.session_id) params.state.addMessage(created);
}

export function useRequestChangesWalkthrough({
  taskId,
  sessionId,
  ready = true,
}: UseRequestChangesWalkthroughParams) {
  const storeApi = useAppStoreApi();
  const { toast } = useToast();

  return useCallback(async () => {
    if (!taskId || !sessionId) return;

    const state = storeApi.getState();
    const activeSession = state.taskSessions.items[sessionId] ?? null;
    const inputMode = deriveSessionInputMode(activeSession);
    const planModeEnabled = state.chatInput.planModeBySessionId[sessionId] ?? false;
    if (!ready) {
      toast({ title: "Changes are still loading", variant: "error" });
      return;
    }
    if (inputMode === "unavailable") {
      toast({ title: "Session is not available for input", variant: "error" });
      return;
    }
    try {
      const template = await loadChangesWalkthroughPromptTemplate();
      const content = buildChangesWalkthroughPrompt(template);
      if (inputMode === "queue") {
        await queueWalkthroughRequest({
          taskId,
          sessionId,
          content,
          planModeEnabled,
        });
        toast({ title: "Walkthrough request queued", variant: "success" });
        return;
      }

      await sendWalkthroughRequest({ taskId, sessionId, content, planModeEnabled, state });
      toast({ title: "Walkthrough request sent", variant: "success" });
    } catch (error) {
      console.error("Failed to request walkthrough:", error);
      toast({ title: "Failed to request walkthrough", variant: "error" });
    }
  }, [ready, sessionId, storeApi, taskId, toast]);
}
