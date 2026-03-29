"use client";

import { useCallback, useState } from "react";
import { IconArrowRight, IconGitMerge, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { TodoIndicator } from "./todo-indicator";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { useMessageHandler } from "@/hooks/use-message-handler";
import { useAppStore } from "@/components/state-provider";
import { type ContextFile } from "@/lib/state/context-files-store";
import {
  ChatInputContainer,
  type ChatInputContainerHandle,
  type MessageAttachment,
} from "@/components/task/chat/chat-input-container";
import {
  formatReviewCommentsAsMarkdown,
  formatPRFeedbackAsMarkdown,
  formatPlanCommentsAsMarkdown,
} from "@/lib/state/slices/comments/format";
import { usePlanActions } from "@/hooks/domains/kanban/use-plan-actions";
import { useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useToast } from "@/components/toast-provider";
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
  needsRecovery: boolean,
): string {
  if (needsRecovery) return "Choose a recovery option above to continue...";
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
        const finalMessage = buildSubmitMessage(
          message,
          reviewComments,
          pendingPRFeedback,
          planComments,
        );
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

function PRMergedBanner({ taskId }: { taskId: string }) {
  const taskPR = useAppStore((state) => state.taskPRs.byTaskId[taskId]);
  const [dismissed, setDismissed] = useState(false);
  const archiveAndSwitch = useArchiveAndSwitchTask();
  const { toast } = useToast();

  const handleArchive = useCallback(async () => {
    try {
      await archiveAndSwitch(taskId);
      toast({ description: "Task archived" });
    } catch {
      toast({ description: "Failed to archive task", variant: "error" });
    }
  }, [taskId, archiveAndSwitch, toast]);

  if (taskPR?.state !== "merged" || dismissed) return null;

  return (
    <div
      data-testid="pr-merged-banner"
      className="flex flex-1 items-center gap-2 rounded-md bg-purple-500/10 px-2 py-1 text-purple-600 dark:text-purple-400"
    >
      <IconGitMerge className="h-3.5 w-3.5 shrink-0" />
      <span className="flex-1">
        PR #{taskPR.pr_number} has been merged. You can archive this task.
      </span>
      <button
        type="button"
        data-testid="pr-merged-archive-button"
        onClick={handleArchive}
        className="underline underline-offset-2 hover:text-purple-700 dark:hover:text-purple-300 cursor-pointer"
      >
        Archive
      </button>
      <button
        type="button"
        aria-label="Dismiss"
        onClick={() => setDismissed(true)}
        className="p-0.5 hover:bg-purple-500/10 rounded cursor-pointer"
      >
        <IconX className="h-3 w-3" />
      </button>
    </div>
  );
}

type TodoDisplayItem = {
  text: string;
  done?: boolean;
  status?: "pending" | "in_progress" | "completed" | "failed";
};

function ChatStatusBar({
  todoItems,
  taskId,
  nextStepName,
  onProceed,
  isAgentBusy,
  isMoving,
}: {
  todoItems: TodoDisplayItem[];
  taskId: string | null;
  nextStepName: string | null;
  onProceed: () => void;
  isAgentBusy: boolean;
  isMoving: boolean;
}) {
  const showTodos = todoItems.length > 0;
  const showProceed = !!nextStepName && !isAgentBusy;
  // PRMergedBanner returns null internally when not applicable
  return (
    <div
      data-testid="chat-status-bar"
      className="flex items-center gap-1.5 py-1 text-xs text-muted-foreground"
    >
      {showTodos && <TodoIndicator todos={todoItems} />}
      {taskId && <PRMergedBanner key={taskId} taskId={taskId} />}
      {showProceed && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="ml-auto h-6 gap-1 px-2 text-xs cursor-pointer hover:bg-muted/40"
          onClick={onProceed}
          disabled={isMoving}
          data-testid="proceed-next-step"
        >
          <span className="text-muted-foreground">{nextStepName}</span>
          <IconArrowRight className="h-3.5 w-3.5" />
        </Button>
      )}
    </div>
  );
}

type ChatInputAreaProps = {
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>;
  clarificationKey: number;
  onClarificationResolved: () => void;
  handleSubmit: (
    message: string,
    reviewComments?: DiffComment[],
    attachments?: MessageAttachment[],
    inlineMentions?: ContextFile[],
  ) => Promise<void>;
  handleCancelTurn: () => Promise<void>;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  panelState: ReturnType<typeof useChatPanelState>;
  isSending: boolean;
  hideSessionsDropdown?: boolean;
  minimalToolbar?: boolean;
  /** Hide the plan mode toggle button (for ephemeral/quick chat sessions) */
  hidePlanMode?: boolean;
  placeholderOverride?: string;
};

export function ChatInputArea({
  chatInputRef,
  clarificationKey,
  onClarificationResolved,
  handleSubmit,
  handleCancelTurn,
  showRequestChangesTooltip,
  onRequestChangesTooltipDismiss,
  panelState,
  isSending,
  hideSessionsDropdown,
  minimalToolbar,
  hidePlanMode,
  placeholderOverride,
}: ChatInputAreaProps) {
  const {
    resolvedSessionId,
    taskId,
    isAgentBusy,
    needsRecovery,
    planModeEnabled,
    activeDocument,
    handlePlanModeChange,
    todoItems,
  } = panelState;
  const { implementPlanHandler, proceedStepName, proceed, isMoving } = usePlanActions({
    resolvedSessionId,
    taskId,
    planModeEnabled,
    handlePlanModeChange,
    chatInputRef,
  });
  const hasClarification = !!panelState.pendingClarification;
  const placeholder =
    placeholderOverride ??
    resolveInputPlaceholder(
      isAgentBusy,
      activeDocument?.type,
      planModeEnabled,
      hasClarification,
      needsRecovery,
    );
  return (
    <div className="bg-card flex-shrink-0 px-2 pb-2 pt-1">
      <ChatStatusBar
        todoItems={todoItems}
        taskId={taskId}
        nextStepName={proceedStepName}
        onProceed={proceed}
        isAgentBusy={isAgentBusy}
        isMoving={isMoving}
      />
      <ChatInputContainer
        ref={chatInputRef}
        key={clarificationKey}
        onSubmit={handleSubmit}
        sessionId={resolvedSessionId}
        taskId={taskId}
        taskTitle={panelState.task?.title}
        taskDescription={panelState.taskDescription ?? ""}
        planModeEnabled={planModeEnabled}
        planModeAvailable={panelState.planModeAvailable}
        mcpServers={panelState.mcpServers}
        onPlanModeChange={handlePlanModeChange}
        isAgentBusy={isAgentBusy}
        isStarting={panelState.isStarting}
        isSending={isSending}
        onCancel={handleCancelTurn}
        placeholder={placeholder}
        pendingClarification={panelState.pendingClarification}
        onClarificationResolved={onClarificationResolved}
        showRequestChangesTooltip={showRequestChangesTooltip}
        onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
        pendingCommentsByFile={panelState.pendingCommentsByFile}
        hasContextComments={
          panelState.planComments.length > 0 || panelState.pendingPRFeedback.length > 0
        }
        submitKey={panelState.chatSubmitKey}
        hasAgentCommands={!!(panelState.agentCommands && panelState.agentCommands.length > 0)}
        isFailed={panelState.isFailed}
        needsRecovery={needsRecovery}
        contextItems={panelState.contextItems}
        planContextEnabled={panelState.planContextEnabled}
        contextFiles={panelState.contextFiles}
        onToggleContextFile={panelState.handleToggleContextFile}
        onAddContextFile={panelState.handleAddContextFile}
        onImplementPlan={implementPlanHandler}
        hideSessionsDropdown={hideSessionsDropdown}
        minimalToolbar={minimalToolbar}
        hidePlanMode={hidePlanMode}
      />
    </div>
  );
}
