import { useCallback } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStoreApi } from "@/components/state-provider";
import { useQueue } from "./domains/session/use-queue";
import type { MessageAttachment } from "@/components/task/chat/chat-input-container";
import type { ActiveDocument } from "@/lib/state/slices/ui/types";
import type { PlanComment } from "@/lib/state/slices/comments";
import { toBlockquote } from "@/lib/state/slices/comments/format";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { CustomPrompt, Message } from "@/lib/types/http";

function buildDocumentContext(
  activeDocument: ActiveDocument | null,
  planModeEnabled: boolean,
  planComments?: PlanComment[],
): string {
  if (!activeDocument) return "";

  if (activeDocument.type === "plan") {
    if (!planModeEnabled) return "";

    let context = `\n\n<kandev-system>\nACTIVE DOCUMENT: The user is editing the task plan side-by-side with this chat.\nRead the current plan using the plan_get MCP tool to understand the context before responding.\nAny plan modifications should use the plan_update MCP tool.`;

    if (planComments && planComments.length > 0) {
      context += `\n\nUser comments on the plan:\n`;
      for (const c of planComments) {
        if (c.selectedText) {
          context += "```\n" + c.selectedText + "\n```\n";
        }
        context += toBlockquote(c.text) + "\n\n";
      }
    }

    context += `\n</kandev-system>`;
    return context;
  }

  return `\n\n<kandev-system>\nACTIVE DOCUMENT: The user is editing "${activeDocument.name}" (${activeDocument.path}) side-by-side with this chat.\nRead this file to understand the context before responding.\n</kandev-system>`;
}

function buildContextFilesContext(contextFiles: ContextFile[], prompts: CustomPrompt[]): string {
  const files = contextFiles.filter(
    (f) => !f.path.startsWith("prompt:") && f.path !== "plan:context",
  );
  const promptFiles = contextFiles.filter((f) => f.path.startsWith("prompt:"));

  let context = "";

  if (files.length > 0) {
    const fileList = files.map((f) => `- ${f.path}`).join("\n");
    context += `\n\n<kandev-system>\nCONTEXT FILES: The user has attached the following files as context. Read these files to understand what the user is referring to:\n${fileList}\n</kandev-system>`;
  }

  if (promptFiles.length > 0) {
    const promptsById = new Map(prompts.map((p) => [p.id, p]));
    const resolved = promptFiles
      .map((f) => {
        const id = f.path.replace("prompt:", "");
        const prompt = promptsById.get(id);
        return prompt ? `### ${prompt.name}\n${prompt.content}` : null;
      })
      .filter(Boolean);

    if (resolved.length > 0) {
      context += `\n\n<kandev-system>\nCONTEXT PROMPTS: The user has included the following prompt instructions as context:\n${resolved.join("\n\n")}\n</kandev-system>`;
    }
  }

  return context;
}

export interface UseMessageHandlerParams {
  resolvedSessionId: string | null;
  taskId: string | null;
  sessionModel: string | null;
  activeModel: string | null;
  planModeEnabled?: boolean;
  isAgentBusy?: boolean;
  activeDocument?: ActiveDocument | null;
  planComments?: PlanComment[];
  contextFiles?: ContextFile[];
  prompts?: CustomPrompt[];
}

type SendMessagePayload = {
  taskId: string;
  resolvedSessionId: string;
  finalMessage: string;
  modelToSend: string | undefined;
  planMode: boolean;
  hasReviewComments?: boolean;
  attachments?: MessageAttachment[];
  contextFilesMeta?: Array<{ path: string; name: string }>;
};

async function sendMessageRequest(payload: SendMessagePayload): Promise<Message | undefined> {
  const client = getWebSocketClient();
  if (!client) return undefined;

  const {
    taskId,
    resolvedSessionId,
    finalMessage,
    modelToSend,
    planMode,
    hasReviewComments,
    attachments,
    contextFilesMeta,
  } = payload;
  const hasAttachments = attachments && attachments.length > 0;

  return client.request<Message | undefined>(
    "message.add",
    {
      task_id: taskId,
      session_id: resolvedSessionId,
      content: finalMessage,
      ...(modelToSend && { model: modelToSend }),
      ...(planMode && { plan_mode: true }),
      ...(hasReviewComments && { has_review_comments: true }),
      ...(hasAttachments && { attachments }),
      ...(contextFilesMeta && { context_files: contextFilesMeta }),
    },
    hasAttachments ? 30000 : 10000,
  );
}

export function useMessageHandler({
  resolvedSessionId,
  taskId,
  sessionModel,
  activeModel,
  planModeEnabled = false,
  isAgentBusy = false,
  activeDocument = null,
  planComments = [],
  contextFiles = [],
  prompts = [],
}: UseMessageHandlerParams) {
  const { queue } = useQueue(resolvedSessionId);
  const storeApi = useAppStoreApi();

  const buildFinalMessage = useCallback(
    (message: string, inlineMentions?: ContextFile[]) => {
      const allContextFiles = [...contextFiles, ...(inlineMentions || [])];
      const documentContext = buildDocumentContext(activeDocument, planModeEnabled, planComments);
      const contextFilesContext = buildContextFilesContext(allContextFiles, prompts);
      return {
        finalMessage: message.trim() + documentContext + contextFilesContext,
        allContextFiles,
      };
    },
    [contextFiles, activeDocument, planModeEnabled, planComments, prompts],
  );

  const handleSendMessage = useCallback(
    async (
      message: string,
      attachments?: MessageAttachment[],
      hasReviewComments?: boolean,
      inlineMentions?: ContextFile[],
    ) => {
      if (!taskId || !resolvedSessionId) {
        console.error("No active task session. Start an agent before sending a message.");
        return;
      }

      const { finalMessage, allContextFiles } = buildFinalMessage(message, inlineMentions);
      const modelToSend = activeModel && activeModel !== sessionModel ? activeModel : undefined;
      const realFiles = allContextFiles.filter(
        (f) => !f.path.startsWith("prompt:") && f.path !== "plan:context",
      );
      const contextFilesMeta =
        realFiles.length > 0 ? realFiles.map((f) => ({ path: f.path, name: f.name })) : undefined;

      if (isAgentBusy) {
        const queueAttachments = attachments?.map((att) => ({
          type: att.type,
          data: att.data,
          mime_type: att.mime_type,
          name: att.name,
        }));
        await queue(taskId, finalMessage, modelToSend, planModeEnabled, queueAttachments);
        return;
      }

      // Add the returned message to the store directly so the chat updates
      // even if the session.message.added broadcast is missed (subscription
      // gap, dropped frame, etc.). addMessage is idempotent on id.
      const created = await sendMessageRequest({
        taskId,
        resolvedSessionId,
        finalMessage,
        modelToSend,
        planMode: planModeEnabled,
        hasReviewComments,
        attachments,
        contextFilesMeta,
      });
      if (created && created.id && created.session_id) {
        storeApi.getState().addMessage(created);
      }
    },
    [
      resolvedSessionId,
      taskId,
      activeModel,
      sessionModel,
      planModeEnabled,
      isAgentBusy,
      queue,
      buildFinalMessage,
      storeApi,
    ],
  );

  return { handleSendMessage };
}
