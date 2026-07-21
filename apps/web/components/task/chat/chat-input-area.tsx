"use client";

import { useCallback, useState, type ReactNode } from "react";
import { IconArrowRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { TodoIndicator } from "./todo-indicator";
import { PRMergedBanner, PRClosedBanner } from "./pr-archive-banners";
import { PRStatusChip } from "@/components/github/pr-status-chip";
import { AzureDevOpsTaskPullRequestChip } from "@/components/azure-devops/azure-devops-task-pull-request-chip";
import { ShareButton, shareableSessionStateClient } from "@/components/task/share/share-button";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { useMessageHandler, buildTaskMentionsContext } from "@/hooks/use-message-handler";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { type ContextFile } from "@/lib/state/context-files-store";
import type { TaskMentionData } from "@/hooks/use-inline-mention";
import {
  ChatInputContainer,
  type ChatSubmitResult,
  type ChatInputContainerHandle,
  type MessageAttachment,
} from "@/components/task/chat/chat-input-container";
import { QueueAffordance } from "@/components/task/chat/queued-ghost-list";
import {
  formatReviewCommentsAsMarkdown,
  formatPRFeedbackAsMarkdown,
  formatPlanCommentsAsMarkdown,
  formatWalkthroughCommentsAsMarkdown,
} from "@/lib/state/slices/comments/format";
import { usePlanActions } from "@/hooks/domains/kanban/use-plan-actions";
import { useExecutorEnvironmentAvailability } from "@/hooks/domains/session/use-executor-environment-availability";
import { useToast } from "@/components/toast-provider";
import type { DiffComment } from "@/lib/diff/types";
import type { useChatPanelState } from "./use-chat-panel-state";
import { cn } from "@/lib/utils";

const PLAN_CONTEXT_PATH = "plan:context";

export function buildSubmitMessage(
  message: string,
  reviewComments: DiffComment[] | undefined,
  pendingPRFeedback: import("@/lib/state/slices/comments").PRFeedbackComment[],
  planComments: import("@/lib/state/slices/comments").PlanComment[],
  walkthroughComments: import("@/lib/state/slices/comments").WalkthroughComment[] = [],
): string {
  let finalMessage = message;
  if (reviewComments && reviewComments.length > 0) {
    finalMessage = formatReviewCommentsAsMarkdown(reviewComments) + (message || "");
  }
  if (walkthroughComments.length > 0) {
    finalMessage = formatWalkthroughCommentsAsMarkdown(walkthroughComments) + finalMessage;
  }
  if (pendingPRFeedback.length > 0) {
    finalMessage = formatPRFeedbackAsMarkdown(pendingPRFeedback) + finalMessage;
  }
  if (planComments.length > 0) {
    const planMarkdown = formatPlanCommentsAsMarkdown(planComments);
    finalMessage = finalMessage ? `${planMarkdown}${finalMessage}` : planMarkdown;
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

type PlaceholderArgs = {
  override: string | undefined;
  isMoving: boolean;
  isAgentBusy: boolean;
  activeDocumentType: string | undefined;
  planModeEnabled: boolean;
  hasClarification: boolean;
  needsRecovery: boolean;
};

function pickInputPlaceholder(a: PlaceholderArgs): string {
  if (a.isMoving) return "Switching agent...";
  // Preserve the prior `??` semantics: an explicit "" override (caller wants
  // no placeholder text) must NOT fall through to the resolver default.
  if (a.override !== undefined) return a.override;
  return resolveInputPlaceholder(
    a.isAgentBusy,
    a.activeDocumentType,
    a.planModeEnabled,
    a.hasClarification,
    a.needsRecovery,
  );
}

function showUnknownMessageSendToast(error: unknown, toast: ReturnType<typeof useToast>["toast"]) {
  console.error("Failed to send message:", error);
  toast({
    title: "Message send status unknown",
    description:
      "The connection dropped or timed out. Refresh the task to confirm whether it went through.",
    variant: "error",
  });
}

function usePanelMessageHandler(panelState: ReturnType<typeof useChatPanelState>) {
  const {
    resolvedSessionId,
    sessionModel,
    activeModel,
    isAgentBusy,
    activeDocument,
    planComments,
    contextFiles,
    prompts,
  } = panelState;
  return useMessageHandler({
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
}

export function useSubmitHandler(
  panelState: ReturnType<typeof useChatPanelState>,
  onSend?: (message: string) => void,
) {
  const [isSending, setIsSending] = useState(false);
  const storeApi = useAppStoreApi();
  const { toast } = useToast();
  const {
    resolvedSessionId,
    planComments,
    pendingPRFeedback,
    walkthroughComments,
    markCommentsSent,
    clearSessionPlanComments,
    handleClearPRFeedback,
    handleClearWalkthroughComments,
    clearEphemeral,
    addContextFile,
    planModeEnabled,
  } = panelState;
  const { handleSendMessage } = usePanelMessageHandler(panelState);

  const handleSubmit = useCallback(
    async (
      message: string,
      reviewComments?: DiffComment[],
      attachments?: MessageAttachment[],
      inlineMentions?: ContextFile[],
      inlineTaskMentions?: TaskMentionData[],
    ) => {
      if (isSending) return;
      setIsSending(true);
      try {
        const finalMessage = buildSubmitMessage(
          message,
          reviewComments,
          pendingPRFeedback,
          planComments,
          walkthroughComments,
        );
        const hasReviewComments = !!(reviewComments && reviewComments.length > 0);
        if (onSend) {
          // Expand task mentions because onSend bypasses useMessageHandler.buildFinalMessage.
          const taskCtx = inlineTaskMentions?.length
            ? buildTaskMentionsContext(inlineTaskMentions, storeApi.getState())
            : "";
          await onSend(finalMessage + taskCtx);
        } else {
          await handleSendMessage(
            finalMessage,
            attachments,
            hasReviewComments,
            inlineMentions,
            inlineTaskMentions,
          );
        }
        if (reviewComments && reviewComments.length > 0)
          markCommentsSent(reviewComments.map((c) => c.id));
        if (pendingPRFeedback.length > 0) handleClearPRFeedback();
        if (walkthroughComments.length > 0) handleClearWalkthroughComments();
        if (planComments.length > 0) clearSessionPlanComments();
        if (resolvedSessionId) {
          clearEphemeral(resolvedSessionId);
          // Re-add plan context if plan mode is still active (clearEphemeral removes unpinned files)
          if (planModeEnabled) {
            addContextFile(resolvedSessionId, { path: PLAN_CONTEXT_PATH, name: "Plan" });
          }
        }
      } catch (error) {
        showUnknownMessageSendToast(error, toast);
        return false;
      } finally {
        setIsSending(false);
      }
    },
    [
      isSending,
      onSend,
      storeApi,
      handleSendMessage,
      markCommentsSent,
      planComments,
      clearSessionPlanComments,
      walkthroughComments,
      handleClearWalkthroughComments,
      pendingPRFeedback,
      handleClearPRFeedback,
      resolvedSessionId,
      clearEphemeral,
      planModeEnabled,
      addContextFile,
      toast,
    ],
  );

  return { isSending, handleSubmit };
}

export function useChatPanelHandlers(
  resolvedSessionId: string | null,
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

  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  useKeyboardShortcut(
    getShortcut("FOCUS_INPUT", keyboardShortcuts),
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

  return { handleCancelTurn };
}

type TodoDisplayItem = {
  text: string;
  done?: boolean;
  status?: "pending" | "in_progress" | "completed" | "failed";
};

function ChatStatusBar({
  todoItems,
  taskId,
  sessionId,
  sessionState,
  nextStepName,
  onProceed,
  isAgentBusy,
  isMoving,
  queueChip,
}: {
  todoItems: TodoDisplayItem[];
  taskId: string | null;
  sessionId: string | null;
  sessionState: string | null;
  nextStepName: string | null;
  onProceed: () => void;
  isAgentBusy: boolean;
  isMoving: boolean;
  queueChip?: ReactNode;
}) {
  const showTodos = todoItems.length > 0;
  const showProceed = !!nextStepName && !isAgentBusy;
  const canShare = !!taskId && !!sessionId && shareableSessionStateClient(sessionState);
  // PRMergedBanner returns null internally when not applicable
  return (
    <div
      data-testid="chat-status-bar"
      className="flex items-center gap-1.5 py-1 text-xs text-muted-foreground"
    >
      {showTodos && <TodoIndicator todos={todoItems} />}
      <PRStatusChip taskId={taskId} />
      <AzureDevOpsTaskPullRequestChip taskId={taskId} />
      {queueChip}
      {/* Distinct per-banner keys: the key remounts the banner on task switch
          so its dismissed state re-initialises, and keeping the two suffixes
          different avoids a duplicate-sibling-key collision. */}
      {taskId && <PRMergedBanner key={`${taskId}-merged`} taskId={taskId} />}
      {taskId && <PRClosedBanner key={`${taskId}-closed`} taskId={taskId} />}
      {canShare && taskId && sessionId && (
        <div className="ml-auto shrink-0">
          <ShareButton taskId={taskId} sessionId={sessionId} iconOnly />
        </div>
      )}
      {showProceed && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className={`${canShare ? "" : "ml-auto "}h-6 gap-1 px-2.5 text-xs cursor-pointer text-primary`}
              onClick={onProceed}
              disabled={isMoving}
              data-testid="proceed-next-step"
            >
              {nextStepName}
              <IconArrowRight className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Move task to the next workflow step</TooltipContent>
        </Tooltip>
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
    inlineTaskMentions?: TaskMentionData[],
  ) => ChatSubmitResult;
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
  surfaceClassName?: string;
};

function useExecutorUnavailable(taskId: string | null, sessionId: string | null) {
  const availability = useExecutorEnvironmentAvailability(taskId, Boolean(sessionId && taskId));
  return {
    unavailable: availability.unavailable,
    reason: availability.status?.label,
  };
}

function useChatInputDerived(
  panelState: ReturnType<typeof useChatPanelState>,
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>,
  placeholderOverride: string | undefined,
) {
  const { resolvedSessionId, taskId, isAgentBusy, needsRecovery, planModeEnabled, activeDocument } =
    panelState;
  const planActions = usePlanActions({
    resolvedSessionId,
    taskId,
    planModeEnabled,
    handlePlanModeChange: panelState.handlePlanModeChange,
    chatInputRef,
  });
  const hasClarification = !!panelState.pendingClarification;
  const executor = useExecutorUnavailable(taskId, resolvedSessionId);
  const placeholder = pickInputPlaceholder({
    override: placeholderOverride,
    isMoving: planActions.isMoving,
    isAgentBusy,
    activeDocumentType: activeDocument?.type,
    planModeEnabled,
    hasClarification,
    needsRecovery,
  });
  return { planActions, executor, placeholder };
}

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
  surfaceClassName,
}: ChatInputAreaProps) {
  const { resolvedSessionId, taskId, isAgentBusy, needsRecovery, planModeEnabled, todoItems } =
    panelState;
  const sessionState = panelState.session?.state ?? null;
  const canDrainQueue = sessionState === "WAITING_FOR_INPUT" || sessionState === "IDLE";
  const { planActions, executor, placeholder } = useChatInputDerived(
    panelState,
    chatInputRef,
    placeholderOverride,
  );
  const { implementPlanHandler, proceedStepName, proceed, isMoving } = planActions;
  return (
    <div
      data-testid="chat-input-area"
      className={cn("bg-card flex-shrink-0 px-2 pb-2 pt-1", surfaceClassName)}
    >
      <QueueAffordance
        sessionId={resolvedSessionId}
        canDrain={canDrainQueue}
        renderStatusBar={(queueChip) => (
          <ChatStatusBar
            todoItems={todoItems}
            taskId={taskId}
            sessionId={resolvedSessionId}
            sessionState={sessionState}
            nextStepName={proceedStepName}
            onProceed={proceed}
            isAgentBusy={isAgentBusy}
            isMoving={isMoving}
            queueChip={queueChip}
          />
        )}
      >
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
          onPlanModeChange={panelState.handlePlanModeChange}
          isAgentBusy={isAgentBusy}
          isStarting={panelState.isStarting}
          isPreparingEnvironment={panelState.isPreparingEnvironment}
          isMoving={isMoving}
          isSending={isSending}
          onCancel={handleCancelTurn}
          placeholder={placeholder}
          pendingClarification={panelState.pendingClarification}
          onClarificationResolved={onClarificationResolved}
          showRequestChangesTooltip={showRequestChangesTooltip}
          onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
          pendingCommentsByFile={panelState.pendingCommentsByFile}
          hasContextComments={
            panelState.planComments.length > 0 ||
            panelState.pendingPRFeedback.length > 0 ||
            panelState.walkthroughComments.length > 0
          }
          submitKey={panelState.chatSubmitKey}
          hasAgentCommands={!!(panelState.agentCommands && panelState.agentCommands.length > 0)}
          isFailed={panelState.isFailed}
          needsRecovery={needsRecovery}
          executorUnavailable={executor.unavailable}
          executorUnavailableReason={executor.reason}
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
      </QueueAffordance>
    </div>
  );
}
