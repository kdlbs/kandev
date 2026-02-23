"use client";

import { useCallback, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { useMessageHandler } from "@/hooks/use-message-handler";
import { type ContextFile } from "@/lib/state/context-files-store";
import {
  ChatInputContainer,
  type ChatInputContainerHandle,
  type MessageAttachment,
} from "@/components/task/chat/chat-input-container";
import { type QueuedMessageIndicatorHandle } from "@/components/task/chat/queued-message-indicator";
import {
  formatReviewCommentsAsMarkdown,
  formatPRFeedbackAsMarkdown,
  formatPlanCommentsAsMarkdown,
} from "@/lib/state/slices/comments/format";
import type { DiffComment } from "@/lib/diff/types";
import type { useChatPanelState } from "./use-chat-panel-state";

const PLAN_CONTEXT_PATH = "plan:context";

function buildSubmitMessage(
  message: string,
  reviewComments: DiffComment[] | undefined,
  pendingPRFeedback: import("@/lib/state/slices/comments").PRFeedbackComment[],
  planComments: import("@/lib/state/slices/comments").PlanComment[],
): string {
  let finalMessage = message;
  if (reviewComments && reviewComments.length > 0) {
    finalMessage = formatReviewCommentsAsMarkdown(reviewComments) + (message || "");
  }
  if (pendingPRFeedback.length > 0) {
    finalMessage = formatPRFeedbackAsMarkdown(pendingPRFeedback) + finalMessage;
  }
  if (planComments.length > 0) {
    const planMarkdown = formatPlanCommentsAsMarkdown(planComments);
    const header = "### Plan Comments\n\n";
    finalMessage = finalMessage
      ? `${header}${planMarkdown}\n\n---\n\n${finalMessage}`
      : `${header}${planMarkdown}`;
  }
  return finalMessage;
}

function resolveInputPlaceholder(
  isAgentBusy: boolean,
  activeDocumentType: string | undefined,
  planModeEnabled: boolean,
  hasClarification: boolean,
): string {
  if (hasClarification) return "Answer the question above to continue...";
  if (isAgentBusy) return "Queue instructions to the agent...";
  if (activeDocumentType === "file") return "Continue working on the file...";
  if (planModeEnabled) return "Continue working on the plan...";
  return "Continue working on the task...";
}

export function useSubmitHandler(
  panelState: ReturnType<typeof useChatPanelState>,
  onSend?: (message: string) => void,
) {
  const [isSending, setIsSending] = useState(false);
  const {
    resolvedSessionId,
    sessionModel,
    activeModel,
    isAgentBusy,
    activeDocument,
    planComments,
    pendingPRFeedback,
    contextFiles,
    prompts,
    markCommentsSent,
    clearSessionPlanComments,
    handleClearPRFeedback,
    clearEphemeral,
    addContextFile,
    planModeEnabled,
  } = panelState;
  const { handleSendMessage } = useMessageHandler({
    resolvedSessionId,
    taskId: panelState.taskId,
    sessionModel,
    activeModel,
    planModeEnabled: panelState.planModeEnabled,
    isAgentBusy,
    activeDocument,
    planComments,
    contextFiles,
    prompts,
  });

  const handleSubmit = useCallback(
    async (
      message: string,
      reviewComments?: DiffComment[],
      attachments?: MessageAttachment[],
      inlineMentions?: ContextFile[],
    ) => {
      if (isSending) return;
      setIsSending(true);
      try {
        const finalMessage = buildSubmitMessage(message, reviewComments, pendingPRFeedback, planComments);
        const hasReviewComments = !!(reviewComments && reviewComments.length > 0);
        if (onSend) {
          await onSend(finalMessage);
        } else {
          await handleSendMessage(finalMessage, attachments, hasReviewComments, inlineMentions);
        }
        if (reviewComments && reviewComments.length > 0)
          markCommentsSent(reviewComments.map((c) => c.id));
        if (pendingPRFeedback.length > 0) handleClearPRFeedback();
        if (planComments.length > 0) clearSessionPlanComments();
        if (resolvedSessionId) {
          clearEphemeral(resolvedSessionId);
          // Re-add plan context if plan mode is still active (clearEphemeral removes unpinned files)
          if (planModeEnabled) {
            addContextFile(resolvedSessionId, { path: PLAN_CONTEXT_PATH, name: "Plan" });
          }
        }
      } finally {
        setIsSending(false);
      }
    },
    [
      isSending,
      onSend,
      handleSendMessage,
      markCommentsSent,
      planComments,
      clearSessionPlanComments,
      pendingPRFeedback,
      handleClearPRFeedback,
      resolvedSessionId,
      clearEphemeral,
      planModeEnabled,
      addContextFile,
    ],
  );

  return { isSending, handleSubmit };
}

export function useChatPanelHandlers(
  resolvedSessionId: string | null,
  cancelQueue: () => Promise<void>,
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>,
) {
  const handleCancelTurn = useCallback(async () => {
    if (!resolvedSessionId) return;
    const client = getWebSocketClient();
    if (!client) return;
    try {
      await client.request("agent.cancel", { session_id: resolvedSessionId }, 15000);
    } catch (error) {
      console.error("Failed to cancel agent turn:", error);
    }
  }, [resolvedSessionId]);

  const handleCancelQueue = useCallback(async () => {
    try {
      await cancelQueue();
    } catch (error) {
      console.error("Failed to cancel queued message:", error);
    }
  }, [cancelQueue]);

  const handleQueueEditComplete = useCallback(() => {
    chatInputRef.current?.focusInput();
  }, [chatInputRef]);

  useKeyboardShortcut(
    SHORTCUTS.FOCUS_INPUT,
    useCallback(
      (event: KeyboardEvent) => {
        const el = document.activeElement;
        const isTyping =
          el instanceof HTMLInputElement ||
          el instanceof HTMLTextAreaElement ||
          (el instanceof HTMLElement && el.isContentEditable);
        if (isTyping) return;
        const inputHandle = chatInputRef.current;
        if (inputHandle) {
          event.preventDefault();
          inputHandle.focusInput();
        }
      },
      [chatInputRef],
    ),
    { enabled: true, preventDefault: false },
  );

  return { handleCancelTurn, handleCancelQueue, handleQueueEditComplete };
}

function useImplementPlan(
  resolvedSessionId: string | null,
  taskId: string | null,
  handlePlanModeChange: (enabled: boolean) => void,
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>,
) {
  return useCallback(() => {
    if (!resolvedSessionId || !taskId) return;

    const client = getWebSocketClient();
    if (!client) return;

    const userText = chatInputRef.current?.getValue() ?? "";
    chatInputRef.current?.clear();

    const visibleText = userText.trim() || "Implement the plan";
    let content = `${visibleText}\n\n<kandev-system>
IMPLEMENT PLAN: The user has approved the plan and wants you to implement it now.
Read the current plan using the get_task_plan_kandev MCP tool.
Implement all changes described in the plan step by step.
After completing the implementation, provide a summary of what was done.
</kandev-system>`;

    client
      .request("message.add", {
        task_id: taskId,
        session_id: resolvedSessionId,
        content,
        plan_mode: false,
      }, 10000)
      .catch((err: unknown) => console.error("Failed to send implement plan message:", err));

    handlePlanModeChange(false);
  }, [resolvedSessionId, taskId, handlePlanModeChange, chatInputRef]);
}

type ChatInputAreaProps = {
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>;
  queuedMessageRef: React.RefObject<QueuedMessageIndicatorHandle | null>;
  clarificationKey: number;
  onClarificationResolved: () => void;
  handleSubmit: (
    message: string,
    reviewComments?: DiffComment[],
    attachments?: MessageAttachment[],
    inlineMentions?: ContextFile[],
  ) => Promise<void>;
  handleCancelTurn: () => Promise<void>;
  handleCancelQueue: () => Promise<void>;
  handleQueueEditComplete: () => void;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  isPanelFocused?: boolean;
  panelState: ReturnType<typeof useChatPanelState>;
  isSending: boolean;
};

export function ChatInputArea({
  chatInputRef,
  queuedMessageRef,
  clarificationKey,
  onClarificationResolved,
  handleSubmit,
  handleCancelTurn,
  handleCancelQueue,
  handleQueueEditComplete,
  showRequestChangesTooltip,
  onRequestChangesTooltipDismiss,
  isPanelFocused,
  panelState,
  isSending,
}: ChatInputAreaProps) {
  const {
    resolvedSessionId,
    task,
    taskId,
    taskDescription,
    isStarting,
    isAgentBusy,
    isFailed,
    planModeEnabled,
    activeDocument,
    handlePlanModeChange,
    contextFiles,
    handleToggleContextFile,
    handleAddContextFile,
    pendingCommentsByFile,
    chatSubmitKey,
    agentCommands,
    isQueued,
    queuedMessage,
    updateQueueContent,
    contextItems,
    planContextEnabled,
    todoItems,
  } = panelState;
  const handleImplementPlan = useImplementPlan(
    resolvedSessionId, taskId, handlePlanModeChange, chatInputRef,
  );

  const hasClarification = !!panelState.pendingClarification;
  const placeholder = resolveInputPlaceholder(
    isAgentBusy, activeDocument?.type, planModeEnabled, hasClarification,
  );
  return (
    <div className="bg-card flex-shrink-0 px-2 pb-2 pt-1">
      <ChatInputContainer
        ref={chatInputRef}
        key={clarificationKey}
        onSubmit={handleSubmit}
        sessionId={resolvedSessionId}
        taskId={taskId}
        taskTitle={task?.title}
        taskDescription={taskDescription ?? ""}
        planModeEnabled={planModeEnabled}
        planModeAvailable={panelState.planModeAvailable}
        mcpServers={panelState.mcpServers}
        onPlanModeChange={handlePlanModeChange}
        isAgentBusy={isAgentBusy}
        isStarting={isStarting}
        isSending={isSending}
        onCancel={handleCancelTurn}
        placeholder={placeholder}
        pendingClarification={panelState.pendingClarification}
        onClarificationResolved={onClarificationResolved}
        showRequestChangesTooltip={showRequestChangesTooltip}
        onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
        pendingCommentsByFile={pendingCommentsByFile}
        hasContextComments={panelState.planComments.length > 0 || panelState.pendingPRFeedback.length > 0}
        submitKey={chatSubmitKey}
        hasAgentCommands={!!(agentCommands && agentCommands.length > 0)}
        isFailed={isFailed}
        isQueued={isQueued}
        contextItems={contextItems}
        planContextEnabled={planContextEnabled}
        contextFiles={contextFiles}
        onToggleContextFile={handleToggleContextFile}
        onAddContextFile={handleAddContextFile}
        todoItems={todoItems}
        queuedMessage={queuedMessage?.content}
        onCancelQueue={handleCancelQueue}
        updateQueueContent={updateQueueContent}
        queuedMessageRef={queuedMessageRef}
        onQueueEditComplete={handleQueueEditComplete}
        isPanelFocused={isPanelFocused}
        onImplementPlan={handleImplementPlan}
      />
    </div>
  );
}
