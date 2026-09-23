import { useCallback, useRef } from "react";
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
import type { ContextFile } from "@/lib/state/context-files-store";
import type {
  CustomPrompt,
  TaskPlanCommentRef,
  TaskPreviewFeedback,
  TaskPreviewFeedbackRef,
} from "@/lib/types/http";
import type { TaskMentionData } from "@/hooks/use-inline-mention";
import type { AppState } from "@/lib/state/store";
import { planCommentAdmissionConflict, toTaskPlanCommentRefs } from "@/lib/plan-comment-refs";
import {
  collectPromptReferenceExpansions,
  formatPromptReferenceExpansions,
  sanitizePromptReferenceSystemText,
} from "@/lib/prompts/expand-prompt-references";
import {
  deriveSessionInputMode,
  type SessionInputMode,
} from "./domains/session/session-input-mode";
import { t } from "@/lib/i18n";
import { getTaskPlanComments } from "@/lib/api/domains/plan-comment-api";
import { getTaskPreviewFeedback } from "@/lib/api/domains/preview-feedback-api";
import {
  previewFeedbackAdmissionConflict,
  toTaskPreviewFeedbackRefs,
} from "@/lib/preview-feedback-refs";
import { findMessageByID, sendMessageRequest } from "./message-request";

export { sendMessageRequest } from "./message-request";

export function buildDocumentContext(
  activeDocument: ActiveDocument | null,
  planModeEnabled: boolean,
): string {
  if (!activeDocument) return "";

  if (activeDocument.type === "plan") {
    if (!planModeEnabled) return "";

    // i18n-exempt: agent-facing prompt sent verbatim to the model, never rendered.
    return `\n\n<kandev-system>\nACTIVE DOCUMENT: The user is editing the task plan side-by-side with this chat.\nRead the current plan using the get_task_plan_kandev MCP tool to understand the context before responding.\nAny plan modifications should use the update_task_plan_kandev MCP tool.\n</kandev-system>`;
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
        return `### ${sanitizePromptReferenceSystemText(prompt.name)}\n${sanitizePromptReferenceSystemText(prompt.content)}`;
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
  /** Resolves the source session's current clarification barrier at send time. */
  getHasPendingClarification?: () => boolean;
  activeDocument?: ActiveDocument | null;
  planComments?: PlanComment[];
  previewFeedback?: TaskPreviewFeedback[];
  contextFiles?: ContextFile[];
  prompts?: CustomPrompt[];
}

export type MessageAdmissionOutcome = "sent" | "queued";

const TERMINAL_SESSION_STATES = new Set(["FAILED", "CANCELLED", "COMPLETED"]);

function requireSessionInputMode(state: AppState, selectedSessionId: string): SessionInputMode {
  const selectedSession = state.taskSessions.items[selectedSessionId] ?? null;
  const queuedCount = state.queue.metaBySessionId[selectedSessionId]?.count ?? 0;
  const inputMode = deriveSessionInputMode(selectedSession, queuedCount);
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

function normalizePlanCommentSendError(
  error: unknown,
  taskId: string,
  storeApi: ReturnType<typeof useAppStoreApi>,
): unknown {
  const conflict = planCommentAdmissionConflict(error);
  if (!conflict) return error;
  if (conflict.snapshot) storeApi.getState().setTaskPlanComments(taskId, conflict.snapshot);
  return conflict.code === "plan_comments_changed"
    ? new MessageSendError("plan-comments-changed", t("task:planCommentsChangedRetry"))
    : new MessageSendError("primary-session-changed", t("task:primarySessionChangedRetry"));
}

function normalizePreviewFeedbackSendError(
  error: unknown,
  taskId: string,
  storeApi: ReturnType<typeof useAppStoreApi>,
): unknown {
  const snapshot = previewFeedbackAdmissionConflict(error);
  if (!snapshot) return error;
  storeApi.getState().setTaskPreviewFeedback(taskId, snapshot);
  return new MessageSendError("preview-feedback-changed", t("task:previewFeedbackChangedRetry"));
}

function buildContextFilesMetadata(contextFiles: ContextFile[]) {
  const realFiles = contextFiles.filter(
    (file) => !file.path.startsWith("prompt:") && file.path !== "plan:context",
  );
  if (realFiles.length === 0) return undefined;
  return realFiles.map((file) => ({
    path: file.path,
    name: file.name,
    ...(file.isDirectory !== undefined ? { is_directory: file.isDirectory } : {}),
  }));
}

async function deliverComposedMessage({
  payload,
  taskId,
  resolvedSessionId,
  finalMessage,
  modelToSend,
  planModeEnabled,
  hasPendingClarification,
  planCommentRefs,
  previewFeedbackRefs,
  contextFilesMeta,
  inputMode,
  queue,
  storeApi,
  clientAdmissionId,
}: {
  payload: ChatSubmitPayload;
  taskId: string;
  resolvedSessionId: string;
  finalMessage: string;
  modelToSend: string | undefined;
  planModeEnabled: boolean;
  hasPendingClarification: boolean;
  planCommentRefs: TaskPlanCommentRef[];
  previewFeedbackRefs: TaskPreviewFeedbackRef[];
  contextFilesMeta: ReturnType<typeof buildContextFilesMetadata>;
  inputMode: SessionInputMode;
  queue: ReturnType<typeof useQueue>["queue"];
  storeApi: ReturnType<typeof useAppStoreApi>;
  clientAdmissionId: string;
}): Promise<MessageAdmissionOutcome | false> {
  try {
    if (hasPendingClarification || inputMode === "queue") {
      const accepted = await queue({
        taskId,
        content: finalMessage,
        model: modelToSend,
        planMode: planModeEnabled,
        attachments: buildQueueAttachments(payload.attachments),
        entityReferences: payload.entityReferences,
        clientQueueId: clientAdmissionId,
        ...(planCommentRefs.length > 0 ? { planCommentRefs } : {}),
        ...(previewFeedbackRefs.length > 0 ? { previewFeedbackRefs } : {}),
        ...(contextFilesMeta ? { contextFilesMeta } : {}),
      });
      if (!accepted) {
        return false;
      }
      await refreshAcceptedPlanComments(taskId, planCommentRefs, storeApi);
      await refreshAcceptedPreviewFeedback(taskId, previewFeedbackRefs, storeApi);
      return "queued";
    }

    const created = await sendMessageRequest({
      taskId,
      resolvedSessionId,
      clientMessageId: clientAdmissionId,
      finalMessage,
      modelToSend,
      planMode: planModeEnabled,
      hasReviewComments: !!payload.reviewComments?.length,
      attachments: payload.attachments,
      contextFilesMeta,
      entityReferences: payload.entityReferences,
      planCommentRefs,
      previewFeedbackRefs,
    });
    if (created?.id && created.session_id) storeApi.getState().addMessage(created);
    await refreshAcceptedPlanComments(taskId, planCommentRefs, storeApi);
    await refreshAcceptedPreviewFeedback(taskId, previewFeedbackRefs, storeApi);
    return "sent";
  } catch (error) {
    throw normalizePreviewFeedbackSendError(
      normalizePlanCommentSendError(error, taskId, storeApi),
      taskId,
      storeApi,
    );
  }
}

async function refreshAcceptedPreviewFeedback(
  taskId: string,
  refs: TaskPreviewFeedbackRef[],
  storeApi: ReturnType<typeof useAppStoreApi>,
) {
  if (refs.length === 0) return;
  try {
    const snapshot = await getTaskPreviewFeedback(taskId);
    storeApi.getState().setTaskPreviewFeedback(taskId, snapshot);
  } catch (error) {
    // i18n-exempt: accepted delivery remains successful; foreground recovery retries this refresh.
    console.error("Failed to refresh task preview feedback after delivery:", error);
  }
}

async function refreshAcceptedPlanComments(
  taskId: string,
  refs: TaskPlanCommentRef[],
  storeApi: ReturnType<typeof useAppStoreApi>,
) {
  if (refs.length === 0) return;
  try {
    const snapshot = await getTaskPlanComments(taskId);
    storeApi.getState().setTaskPlanComments(taskId, snapshot);
  } catch (error) {
    // i18n-exempt: accepted delivery remains successful; foreground recovery retries this refresh.
    console.error("Failed to refresh task plan comments after delivery:", error);
  }
}

type PendingMessageAdmission = { key: string; id: string };

function messageAdmissionKey(parts: {
  taskId: string;
  resolvedSessionId: string;
  finalMessage: string;
  modelToSend?: string;
  planModeEnabled: boolean;
  hasReviewComments: boolean;
  planCommentRefs: TaskPlanCommentRef[];
  previewFeedbackRefs: TaskPreviewFeedbackRef[];
  contextFilesMeta: ReturnType<typeof buildContextFilesMetadata>;
  attachments: ChatSubmitPayload["attachments"];
  entityReferences: ChatSubmitPayload["entityReferences"];
}) {
  return JSON.stringify(parts);
}

type PendingMessageRecovery = {
  admission: PendingMessageAdmission | null;
  taskId: string;
  sessionId: string;
  refs: TaskPlanCommentRef[];
  previewRefs: TaskPreviewFeedbackRef[];
  storeApi: ReturnType<typeof useAppStoreApi>;
};

async function recoverPendingMessageAdmission({
  admission,
  taskId,
  sessionId,
  refs,
  previewRefs,
  storeApi,
}: PendingMessageRecovery) {
  if (!admission) return false;
  const client = getWebSocketClient();
  const committed = client
    ? await findMessageByID(client, taskId, sessionId, admission.id)
    : undefined;
  if (!committed) return false;
  storeApi.getState().addMessage(committed);
  await refreshAcceptedPlanComments(taskId, refs, storeApi);
  await refreshAcceptedPreviewFeedback(taskId, previewRefs, storeApi);
  return true;
}

function buildFinalMessageForSubmit({
  payload,
  contextFiles,
  activeDocument,
  planModeEnabled,
  prompts,
  state,
}: {
  payload: ChatSubmitPayload;
  contextFiles: ContextFile[];
  activeDocument: ActiveDocument | null;
  planModeEnabled: boolean;
  prompts: CustomPrompt[];
  state: AppState;
}) {
  const allContextFiles = [...contextFiles, ...(payload.inlineMentions || [])];
  const documentContext = buildDocumentContext(activeDocument, planModeEnabled);
  const contextFilesContext = buildContextFilesContext(allContextFiles, prompts);
  const taskMentionsContext = payload.inlineTaskMentions?.length
    ? buildTaskMentionsContext(payload.inlineTaskMentions, state)
    : "";
  return {
    finalMessage:
      payload.message.trim() + documentContext + contextFilesContext + taskMentionsContext,
    allContextFiles,
  };
}

type ComposedMessageContext = {
  resolvedSessionId: string | null;
  taskId: string | null;
  sessionModel: string | null;
  activeModel: string | null;
  planModeEnabled: boolean;
  hasPendingClarification: boolean;
  getHasPendingClarification?: () => boolean;
  activeDocument: ActiveDocument | null;
  planComments: PlanComment[];
  previewFeedback: TaskPreviewFeedback[];
  contextFiles: ContextFile[];
  prompts: CustomPrompt[];
  queue: ReturnType<typeof useQueue>["queue"];
  storeApi: ReturnType<typeof useAppStoreApi>;
  pendingAdmissionRef: { current: PendingMessageAdmission | null };
};

// eslint-disable-next-line complexity -- admission evaluates one ordered input-mode and recovery path.
async function sendComposedMessage(payload: ChatSubmitPayload, context: ComposedMessageContext) {
  const { taskId, resolvedSessionId, storeApi, pendingAdmissionRef } = context;
  if (!taskId || !resolvedSessionId) {
    const error = new MessageSendError(
      "no-active-session",
      "No active task session. Start an agent before sending a message.",
    );
    console.error(error.message);
    throw error;
  }

  const { finalMessage, allContextFiles } = buildFinalMessageForSubmit({
    payload,
    contextFiles: context.contextFiles,
    activeDocument: context.activeDocument,
    planModeEnabled: context.planModeEnabled,
    prompts: context.prompts,
    state: storeApi.getState(),
  });
  const modelToSend =
    context.activeModel && context.activeModel !== context.sessionModel
      ? context.activeModel
      : undefined;
  const planCommentRefs = payload.planCommentRefs ?? toTaskPlanCommentRefs(context.planComments);
  const previewFeedbackRefs =
    payload.previewFeedbackRefs ?? toTaskPreviewFeedbackRefs(context.previewFeedback);
  const contextFilesMeta = buildContextFilesMetadata(allContextFiles);
  const inputMode = requireSessionInputMode(storeApi.getState(), resolvedSessionId);
  const admissionKey = messageAdmissionKey({
    taskId,
    resolvedSessionId,
    finalMessage,
    modelToSend,
    planModeEnabled: context.planModeEnabled,
    hasReviewComments: !!payload.reviewComments?.length,
    planCommentRefs,
    previewFeedbackRefs,
    contextFilesMeta,
    attachments: payload.attachments,
    entityReferences: payload.entityReferences,
  });
  const previousAdmission =
    pendingAdmissionRef.current?.key === admissionKey ? pendingAdmissionRef.current : null;
  const admission =
    previousAdmission ??
    ({ key: admissionKey, id: payload.clientMessageId ?? generateUUID() } as const);
  pendingAdmissionRef.current = admission;
  const recovered = await recoverPendingMessageAdmission({
    admission: previousAdmission,
    taskId,
    sessionId: resolvedSessionId,
    refs: planCommentRefs,
    previewRefs: previewFeedbackRefs,
    storeApi,
  });
  if (recovered) {
    if (pendingAdmissionRef.current === admission) pendingAdmissionRef.current = null;
    return "sent" as const;
  }
  const delivered = await deliverComposedMessage({
    payload,
    taskId,
    resolvedSessionId,
    finalMessage,
    modelToSend,
    planModeEnabled: context.planModeEnabled,
    hasPendingClarification:
      context.getHasPendingClarification?.() ?? context.hasPendingClarification,
    planCommentRefs,
    previewFeedbackRefs,
    contextFilesMeta,
    inputMode,
    queue: context.queue,
    storeApi,
    clientAdmissionId: admission.id,
  });
  if (delivered === false) return false;
  if (pendingAdmissionRef.current === admission) pendingAdmissionRef.current = null;
  return delivered;
}

export function useMessageHandler({
  resolvedSessionId,
  taskId,
  sessionModel,
  activeModel,
  planModeEnabled = false,
  hasPendingClarification = false,
  getHasPendingClarification,
  activeDocument = null,
  planComments = [],
  previewFeedback = [],
  contextFiles = [],
  prompts = [],
}: UseMessageHandlerParams) {
  const { queue } = useQueue(resolvedSessionId);
  const storeApi = useAppStoreApi();
  const pendingAdmissionRef = useRef<PendingMessageAdmission | null>(null);

  const sendMessage = useCallback(
    (payload: ChatSubmitPayload) =>
      sendComposedMessage(payload, {
        taskId,
        resolvedSessionId,
        sessionModel,
        activeModel,
        planModeEnabled,
        hasPendingClarification,
        getHasPendingClarification,
        activeDocument,
        planComments,
        previewFeedback,
        contextFiles,
        prompts,
        queue: queue,
        storeApi,
        pendingAdmissionRef,
      }),
    [
      resolvedSessionId,
      taskId,
      activeModel,
      sessionModel,
      planModeEnabled,
      hasPendingClarification,
      getHasPendingClarification,
      queue,
      storeApi,
      planComments,
      previewFeedback,
      contextFiles,
      activeDocument,
      prompts,
    ],
  );

  const handleSendMessage = useCallback(
    async (payload: ChatSubmitPayload) => {
      const outcome = await sendMessage(payload);
      if (outcome === false) return false;
    },
    [sendMessage],
  );

  return { handleSendMessage, handleSendMessageWithOutcome: sendMessage };
}
