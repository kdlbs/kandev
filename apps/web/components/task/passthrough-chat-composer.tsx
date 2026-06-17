"use client";

import { useCallback, type RefObject } from "react";
import { useToast } from "@/components/toast-provider";
import { useAppStoreApi } from "@/components/state-provider";
import { useCommentsStore } from "@/lib/state/slices/comments/comments-store";
import { formatReviewCommentsAsMarkdown } from "@/lib/state/slices/comments/format";
import { buildSubmitMessage } from "./chat/chat-input-area";
import {
  ChatInputContainer,
  type ChatInputContainerHandle,
  type MessageAttachment,
} from "./chat/chat-input-container";
import type { useChatPanelState } from "./chat/use-chat-panel-state";
import type { DiffComment } from "@/lib/diff/types";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { TaskMentionData } from "@/hooks/use-inline-mention";
import { buildContextFilesContext, buildTaskMentionsContext } from "@/hooks/use-message-handler";
import { getWebSocketClient } from "@/lib/ws/connection";

export type PassthroughSubmitHandler = (
  content: string,
  reviewComments?: DiffComment[],
  attachments?: MessageAttachment[],
  inlineMentions?: ContextFile[],
  inlineTaskMentions?: TaskMentionData[],
) => Promise<void>;

export function PassthroughComposerPanel({
  refHandle,
  onSubmit,
  onCancel,
  panelState,
  taskId,
  isMoving,
  isSending,
}: {
  refHandle: RefObject<ChatInputContainerHandle | null>;
  onSubmit: PassthroughSubmitHandler;
  onCancel: () => void;
  panelState: ReturnType<typeof useChatPanelState>;
  taskId: string | null;
  isMoving: boolean;
  isSending: boolean;
}) {
  const hasContextComments =
    panelState.planComments.length > 0 || panelState.pendingPRFeedback.length > 0;
  return (
    <div
      data-testid="passthrough-composer"
      onKeyDownCapture={(event) => {
        if (event.key === "Escape") onCancel();
      }}
    >
      <ChatInputContainer
        ref={refHandle}
        onSubmit={onSubmit}
        sessionId={panelState.resolvedSessionId}
        taskId={taskId}
        taskTitle={panelState.task?.title}
        taskDescription={panelState.taskDescription ?? ""}
        planModeEnabled={panelState.planModeEnabled}
        planModeAvailable={panelState.planModeAvailable}
        mcpServers={panelState.mcpServers}
        onPlanModeChange={panelState.handlePlanModeChange}
        isAgentBusy={false}
        isStarting={panelState.isStarting}
        isPreparingEnvironment={panelState.isPreparingEnvironment}
        isMoving={isMoving}
        isSending={isSending}
        onCancel={onCancel}
        placeholder="Type a message, @mention files or prompts, Shift+Enter for newline"
        pendingCommentsByFile={panelState.pendingCommentsByFile}
        hasContextComments={hasContextComments}
        submitKey={panelState.chatSubmitKey}
        hasAgentCommands={false}
        contextItems={panelState.contextItems}
        planContextEnabled={panelState.planContextEnabled}
        contextFiles={panelState.contextFiles}
        onToggleContextFile={panelState.handleToggleContextFile}
        onAddContextFile={panelState.handleAddContextFile}
        hideSessionsDropdown
      />
    </div>
  );
}

type PassthroughFinalMessage = {
  content: string;
  commentsToSend: DiffComment[];
  contextFilesMeta?: Array<{ path: string; name: string }>;
};

export function formatPassthroughBaseMessage(
  content: string,
  reviewComments: DiffComment[] | undefined,
  pendingComments: DiffComment[],
  panelState: ReturnType<typeof useChatPanelState>,
) {
  const commentsToSend = reviewComments ?? pendingComments;
  const hasStructuredComments =
    !!reviewComments ||
    panelState.pendingPRFeedback.length > 0 ||
    panelState.planComments.length > 0;
  if (hasStructuredComments) {
    return {
      formatted: buildSubmitMessage(
        content,
        commentsToSend.length > 0 ? commentsToSend : undefined,
        panelState.pendingPRFeedback,
        panelState.planComments,
      ),
      commentsToSend,
    };
  }
  if (pendingComments.length > 0) {
    return {
      formatted: formatReviewCommentsAsMarkdown(pendingComments) + content,
      commentsToSend,
    };
  }
  return { formatted: content, commentsToSend };
}

export function buildPassthroughFinalMessage({
  content,
  reviewComments,
  pendingComments,
  panelState,
  inlineMentions,
  inlineTaskMentions,
  getState,
}: {
  content: string;
  reviewComments?: DiffComment[];
  pendingComments: DiffComment[];
  panelState: ReturnType<typeof useChatPanelState>;
  inlineMentions?: ContextFile[];
  inlineTaskMentions?: TaskMentionData[];
  getState: ReturnType<typeof useAppStoreApi>["getState"];
}): PassthroughFinalMessage {
  const { formatted, commentsToSend } = formatPassthroughBaseMessage(
    content,
    reviewComments,
    pendingComments,
    panelState,
  );
  const allContextFiles = [...panelState.contextFiles, ...(inlineMentions ?? [])];
  const contextFilesContext = buildContextFilesContext(allContextFiles, panelState.prompts);
  const taskMentionsContext =
    inlineTaskMentions && inlineTaskMentions.length > 0
      ? buildTaskMentionsContext(inlineTaskMentions, getState())
      : "";
  return {
    content: formatted + contextFilesContext + taskMentionsContext,
    commentsToSend,
    contextFilesMeta: buildContextFilesMeta(allContextFiles),
  };
}

export function buildContextFilesMeta(files: ContextFile[]) {
  const realContextFiles = files.filter(
    (f) => !f.path.startsWith("prompt:") && f.path !== "plan:context",
  );
  if (realContextFiles.length === 0) return undefined;
  return realContextFiles.map((f) => ({ path: f.path, name: f.name }));
}

async function requestPassthroughMessage({
  taskId,
  sessionId,
  message,
  attachments,
}: {
  taskId: string;
  sessionId: string;
  message: PassthroughFinalMessage;
  attachments?: MessageAttachment[];
}) {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket client not available");
  const hasAttachments = !!(attachments && attachments.length > 0);
  await client.request(
    "message.add",
    {
      task_id: taskId,
      session_id: sessionId,
      content: message.content,
      ...(hasAttachments && { attachments }),
      ...(message.contextFilesMeta && { context_files: message.contextFilesMeta }),
    },
    hasAttachments ? 30_000 : 10_000,
  );
}

export function clearPassthroughComposerContext(panelState: ReturnType<typeof useChatPanelState>) {
  if (panelState.pendingPRFeedback.length > 0) {
    panelState.handleClearPRFeedback();
  }
  if (panelState.planComments.length > 0) {
    panelState.clearSessionPlanComments();
  }
  if (!panelState.resolvedSessionId) return;
  panelState.clearEphemeral(panelState.resolvedSessionId);
  if (panelState.planModeEnabled) {
    panelState.addContextFile(panelState.resolvedSessionId, {
      path: "plan:context",
      name: "Plan",
    });
  }
}

export function useSendPassthroughMessage({
  taskId,
  sessionId,
  pendingComments,
  panelState,
  onSent,
}: {
  taskId: string | null;
  sessionId: string | null | undefined;
  pendingComments: DiffComment[];
  panelState: ReturnType<typeof useChatPanelState>;
  onSent: () => void;
}) {
  const { toast } = useToast();
  const markCommentsSent = useCommentsStore((s) => s.markCommentsSent);
  const storeApi = useAppStoreApi();

  return useCallback(
    async (
      content: string,
      reviewComments?: DiffComment[],
      attachments?: MessageAttachment[],
      inlineMentions?: ContextFile[],
      inlineTaskMentions?: TaskMentionData[],
    ) => {
      if (!taskId || !sessionId) {
        toast({ title: "Session not ready", variant: "error" });
        throw new Error("Session not ready");
      }
      const message = buildPassthroughFinalMessage({
        content,
        reviewComments,
        pendingComments,
        panelState,
        inlineMentions,
        inlineTaskMentions,
        getState: storeApi.getState,
      });
      try {
        await requestPassthroughMessage({ taskId, sessionId, message, attachments });
        if (message.commentsToSend.length > 0) {
          markCommentsSent(message.commentsToSend.map((c) => c.id));
        }
        clearPassthroughComposerContext(panelState);
        onSent();
      } catch (error) {
        console.error("Failed to send passthrough message:", error);
        toast({ title: "Failed to send message", variant: "error" });
        throw error;
      }
    },
    [taskId, sessionId, toast, pendingComments, panelState, storeApi, markCommentsSent, onSent],
  );
}
