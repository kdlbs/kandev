import { useCallback } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { MessageSendError } from "@/lib/chat/message-send-error";
import { generateUUID } from "@/lib/utils";
import { useAppStoreApi } from "@/components/state-provider";
import { useQueue } from "./domains/session/use-queue";
import type {
  ChatSubmitPayload,
  MessageAttachment,
} from "@/components/task/chat/chat-input-container";
import type { ActiveDocument } from "@/lib/state/slices/ui/types";
import type { PlanComment } from "@/lib/state/slices/comments";
import { toBlockquote } from "@/lib/state/slices/comments/format";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { CustomPrompt, Message } from "@/lib/types/http";
import type { TaskMentionData } from "@/hooks/use-inline-mention";
import type { AppState } from "@/lib/state/store";
import type { EntityReference } from "@/lib/types/entity-reference";
import {
  collectPromptReferenceExpansions,
  formatPromptReferenceExpansions,
} from "@/lib/prompts/expand-prompt-references";
import {
  deriveSessionInputMode,
  type SessionInputMode,
} from "./domains/session/session-input-mode";
import { t } from "@/lib/i18n";

export function buildDocumentContext(
  activeDocument: ActiveDocument | null,
  planModeEnabled: boolean,
  planComments?: PlanComment[],
): string {
  if (!activeDocument) return "";

  if (activeDocument.type === "plan") {
    if (!planModeEnabled) return "";

    let context = `\n\n<kandev-system>\nACTIVE DOCUMENT: The user is editing the task plan side-by-side with this chat.\nRead the current plan using the get_task_plan_kandev MCP tool to understand the context before responding.\nAny plan modifications should use the update_task_plan_kandev MCP tool.`;

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

  // i18n-exempt: agent-facing prompt sent verbatim to the model, never rendered.
  return `\n\n<kandev-system>\nACTIVE DOCUMENT: The user is editing "${activeDocument.name}" (${activeDocument.path}) side-by-side with this chat.\nRead this file to understand the context before responding.\n</kandev-system>`;
}

function resolveStepTitle(stepId: string, state: AppState): string {
  const step = state.kanban.steps.find((s) => s.id === stepId);
  if (step) return step.title;
  for (const snap of Object.values(state.kanbanMulti.snapshots)) {
    const found = (snap.steps ?? []).find((s) => s.id === stepId);
    if (found) return found.title;
  }
  return t("common:step");
}

// Strips characters that could break out of the <kandev-system> block when
// task strings are interpolated verbatim — newlines (close-tag injection)
// and angle brackets. Task titles can come from Jira/Linear sync or other
// users in a shared workspace, so the data is not trusted.
function sanitizeForPrompt(value: string): string {
  return value.replace(/[\r\n<>]/g, " ");
}

export function buildTaskMentionsContext(tasks: TaskMentionData[], state: AppState): string {
  if (tasks.length === 0) return "";
  const lines = tasks.map((t) => {
    const stepTitle = resolveStepTitle(t.workflowStepId, state);
    const title = sanitizeForPrompt(t.title);
    const taskId = sanitizeForPrompt(t.taskId);
    const workflowId = sanitizeForPrompt(t.workflowId);
    const step = sanitizeForPrompt(stepTitle);
    const stateSuffix = t.state ? `, state: ${sanitizeForPrompt(t.state)}` : "";
    return `- ${title} (id: ${taskId}, workflow_id: ${workflowId}, step: ${step}${stateSuffix})`;
  });
  return (
    `\n\n<kandev-system>\n` +
    `REFERENCED TASKS: The user mentioned the following tasks. Use these IDs with the kandev MCP tools ` +
    `(e.g. \`get_task_conversation_kandev\`, \`update_task_kandev\`, \`get_task_plan_kandev\`) when the user asks you to act on them.\n` +
    lines.join("\n") +
    `\n</kandev-system>`
  );
}

export function buildContextFilesContext(
  contextFiles: ContextFile[],
  prompts: CustomPrompt[],
): string {
  const files = contextFiles.filter(
    (f) => !f.path.startsWith("prompt:") && f.path !== "plan:context",
  );
  const promptFiles = contextFiles.filter((f) => f.path.startsWith("prompt:"));

  let context = "";

  if (files.length > 0) {
    const pathList = files
      .map((f) => `- ${f.isDirectory ? "directory" : "file"}: ${sanitizeForPrompt(f.path)}`)
      .join("\n");
    context += `\n\n<kandev-system>\nCONTEXT PATHS: The user has attached the following file and directory paths as context. Inspect these paths to understand what the user is referring to:\n${pathList}\n</kandev-system>`;
  }

  if (promptFiles.length > 0) {
    const promptsById = new Map(prompts.map((p) => [p.id, p]));
    const selectedPrompts = promptFiles
      .map((f) => promptsById.get(f.path.replace("prompt:", "")))
      .filter((prompt): prompt is CustomPrompt => Boolean(prompt));
    const selectedPromptNames = new Set(selectedPrompts.map((prompt) => prompt.name));
    const promptExpansions = new Map<string, string>();
    const resolved = selectedPrompts
      .map((prompt) => {
        for (const expansion of collectPromptReferenceExpansions(
          prompt.content,
          prompts,
          prompt.name,
          selectedPromptNames,
        )) {
          if (!promptExpansions.has(expansion.name)) {
            promptExpansions.set(expansion.name, expansion.content);
          }
        }
        return `### ${prompt.name}\n${prompt.content}`;
      })
      .filter(Boolean);

    if (resolved.length > 0) {
      const expansions = Array.from(promptExpansions, ([name, content]) => ({ name, content }));
      const expansionContext = formatPromptReferenceExpansions(expansions);
      context += `\n\n<kandev-system>\nCONTEXT PROMPTS: The user has included the following prompt instructions as context:\n${resolved.join("\n\n")}${expansionContext ? "\n\n" + expansionContext : ""}\n</kandev-system>`;
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
  hasPendingClarification?: boolean;
  activeDocument?: ActiveDocument | null;
  planComments?: PlanComment[];
  contextFiles?: ContextFile[];
  prompts?: CustomPrompt[];
}

type SendMessagePayload = {
  taskId: string;
  resolvedSessionId: string;
  clientMessageId?: string;
  finalMessage: string;
  modelToSend: string | undefined;
  planMode: boolean;
  hasReviewComments?: boolean;
  attachments?: MessageAttachment[];
  contextFilesMeta?: Array<{ path: string; name: string; is_directory?: boolean }>;
  entityReferences?: EntityReference[];
};

type MessageListResponse = { messages?: Message[] };

function isUncertainMessageTransportError(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  const message = error.message.toLowerCase();
  return (
    message.includes("websocket request timed out") || message === "websocket connection closed"
  );
}

async function findMessageByID(
  client: ReturnType<typeof getWebSocketClient>,
  sessionId: string,
  messageId: string,
): Promise<Message | undefined> {
  if (!client) return undefined;
  try {
    const response = await client.request<MessageListResponse>(
      "message.list",
      { session_id: sessionId, limit: 100, sort: "desc" },
      5000,
    );
    return response.messages?.find((message) => message.id === messageId);
  } catch {
    return undefined;
  }
}

async function waitForConnected(client: NonNullable<ReturnType<typeof getWebSocketClient>>) {
  const getStatus = client.getStatus?.bind(client);
  if (!getStatus || getStatus() === "connected") return true;
  const deadline = Date.now() + 3000;
  while (Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, 100));
    if (getStatus() === "connected") return true;
  }
  return false;
}

async function reconcileUncertainMessage(
  client: NonNullable<ReturnType<typeof getWebSocketClient>>,
  sessionId: string,
  messageId: string,
  request: () => Promise<Message | undefined>,
  originalError: unknown,
) {
  const committed = await findMessageByID(client, sessionId, messageId);
  if (committed) return committed;
  if (!(await waitForConnected(client))) throw originalError;

  try {
    return await request();
  } catch (retryError) {
    const retriedMessage = await findMessageByID(client, sessionId, messageId);
    if (retriedMessage) return retriedMessage;
    throw retryError;
  }
}

export async function sendMessageRequest(
  payload: SendMessagePayload,
): Promise<Message | undefined> {
  const client = getWebSocketClient();
  if (!client) {
    throw new MessageSendError(
      "connection-unavailable",
      "Connection unavailable. Reconnect and try again.",
    );
  }

  const {
    taskId,
    resolvedSessionId,
    clientMessageId,
    finalMessage,
    modelToSend,
    planMode,
    hasReviewComments,
    attachments,
    contextFilesMeta,
    entityReferences,
  } = payload;
  const hasAttachments = attachments && attachments.length > 0;
  const stableMessageId = clientMessageId ?? generateUUID();
  const requestPayload = {
    task_id: taskId,
    session_id: resolvedSessionId,
    client_message_id: stableMessageId,
    content: finalMessage,
    ...(modelToSend && { model: modelToSend }),
    ...(planMode && { plan_mode: true }),
    ...(hasReviewComments && { has_review_comments: true }),
    ...(hasAttachments && { attachments }),
    ...(contextFilesMeta && { context_files: contextFilesMeta }),
    ...(entityReferences && { entity_references: entityReferences }),
  };

  const request = () =>
    client.request<Message | undefined>(
      "message.add",
      requestPayload,
      hasAttachments ? 30000 : 10000,
    );

  try {
    return await request();
  } catch (error) {
    if (!isUncertainMessageTransportError(error)) throw error;
    return reconcileUncertainMessage(client, resolvedSessionId, stableMessageId, request, error);
  }
}

const TERMINAL_SESSION_STATES = new Set(["FAILED", "CANCELLED", "COMPLETED"]);

function requireSessionInputMode(state: AppState, selectedSessionId: string): SessionInputMode {
  const selectedSession = state.taskSessions.items[selectedSessionId] ?? null;
  const inputMode = deriveSessionInputMode(selectedSession);
  if (inputMode === "unavailable") {
    // A terminal session row (agent process has exited) gets the backend's
    // actionable copy; a missing row keeps the generic message since there is
    // nothing session-specific to say.
    const message =
      selectedSession && TERMINAL_SESSION_STATES.has(selectedSession.state)
        ? t("task:sessionEndedCreateNew")
        : t("task:sessionNotAvailableForInput");
    throw new MessageSendError("session-unavailable", message);
  }
  return inputMode;
}

function buildQueueAttachments(attachments?: MessageAttachment[]) {
  return attachments?.map((att) => ({
    type: att.type,
    ...(att.attachment_id ? { attachment_id: att.attachment_id } : { data: att.data ?? "" }),
    mime_type: att.mime_type,
    name: att.name,
    size_bytes: att.size_bytes,
    delivery_mode: att.delivery_mode,
  }));
}

export function useMessageHandler({
  resolvedSessionId,
  taskId,
  sessionModel,
  activeModel,
  planModeEnabled = false,
  hasPendingClarification = false,
  activeDocument = null,
  planComments = [],
  contextFiles = [],
  prompts = [],
}: UseMessageHandlerParams) {
  const { queue } = useQueue(resolvedSessionId);
  const storeApi = useAppStoreApi();

  const buildFinalMessage = useCallback(
    (message: string, inlineMentions?: ContextFile[], inlineTaskMentions?: TaskMentionData[]) => {
      const allContextFiles = [...contextFiles, ...(inlineMentions || [])];
      const documentContext = buildDocumentContext(activeDocument, planModeEnabled, planComments);
      const contextFilesContext = buildContextFilesContext(allContextFiles, prompts);
      const taskMentionsContext = inlineTaskMentions?.length
        ? buildTaskMentionsContext(inlineTaskMentions, storeApi.getState())
        : "";
      return {
        finalMessage: message.trim() + documentContext + contextFilesContext + taskMentionsContext,
        allContextFiles,
      };
    },
    [contextFiles, activeDocument, planModeEnabled, planComments, prompts, storeApi],
  );

  const handleSendMessage = useCallback(
    async (payload: ChatSubmitPayload) => {
      if (!taskId || !resolvedSessionId) {
        const error = new MessageSendError(
          "no-active-session",
          "No active task session. Start an agent before sending a message.",
        );
        console.error(error.message);
        throw error;
      }

      const { finalMessage, allContextFiles } = buildFinalMessage(
        payload.message,
        payload.inlineMentions,
        payload.inlineTaskMentions,
      );
      const modelToSend = activeModel && activeModel !== sessionModel ? activeModel : undefined;
      const realFiles = allContextFiles.filter(
        (f) => !f.path.startsWith("prompt:") && f.path !== "plan:context",
      );
      const contextFilesMeta =
        realFiles.length > 0
          ? realFiles.map((f) => ({
              path: f.path,
              name: f.name,
              ...(f.isDirectory !== undefined ? { is_directory: f.isDirectory } : {}),
            }))
          : undefined;

      const inputMode = requireSessionInputMode(storeApi.getState(), resolvedSessionId);
      if (hasPendingClarification || inputMode === "queue") {
        const queueAttachments = buildQueueAttachments(payload.attachments);
        await queue({
          taskId,
          content: finalMessage,
          model: modelToSend,
          planMode: planModeEnabled,
          attachments: queueAttachments,
          entityReferences: payload.entityReferences,
          ...(contextFilesMeta ? { contextFilesMeta } : {}),
        });
        return;
      }

      // Add the returned message to the store directly so the chat updates
      // even if the session.message.added broadcast is missed (subscription
      // gap, dropped frame, etc.). addMessage is idempotent on id.
      const created = await sendMessageRequest({
        taskId,
        resolvedSessionId,
        clientMessageId: generateUUID(),
        finalMessage,
        modelToSend,
        planMode: planModeEnabled,
        hasReviewComments: !!payload.reviewComments?.length,
        attachments: payload.attachments,
        contextFilesMeta,
        entityReferences: payload.entityReferences,
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
      hasPendingClarification,
      queue,
      buildFinalMessage,
      storeApi,
    ],
  );

  return { handleSendMessage };
}
