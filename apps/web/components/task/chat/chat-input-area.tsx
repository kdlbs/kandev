"use client";

import { useCallback, useState, type ReactNode } from "react";
import { IconArrowRight, IconGitMerge, IconGitPullRequestClosed, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { TodoIndicator } from "./todo-indicator";
import { PRStatusChip } from "@/components/github/pr-status-chip";
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
} from "@/lib/state/slices/comments/format";
import { usePlanActions } from "@/hooks/domains/kanban/use-plan-actions";
import { useExecutorEnvironmentAvailability } from "@/hooks/domains/session/use-executor-environment-availability";
import { useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useToast } from "@/components/toast-provider";
import {
  markPRClosedBannerDismissed,
  markPRMergedBannerDismissed,
  wasPRClosedBannerDismissed,
  wasPRMergedBannerDismissed,
} from "@/lib/local-storage";
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
    markCommentsSent,
    clearSessionPlanComments,
    handleClearPRFeedback,
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

// Shared archive-task action for the terminal-state banners: archives the task,
// switches to the next one, and only toasts on failure.
function useArchiveTaskAction(taskId: string) {
  const archiveAndSwitch = useArchiveAndSwitchTask();
  const { toast } = useToast();
  return useCallback(async () => {
    try {
      await archiveAndSwitch(taskId);
    } catch {
      toast({ description: "Failed to archive task", variant: "error" });
    }
  }, [taskId, archiveAndSwitch, toast]);
}

// Presentational banner shared by PRMergedBanner / PRClosedBanner — an icon, a
// message, and Archive + Dismiss controls. Colors/icon/testIds are supplied by
// the caller so the two variants stay visually distinct.
function ArchiveDismissBanner({
  testIdPrefix,
  icon,
  text,
  containerClass,
  archiveClass,
  dismissClass,
  onArchive,
  onDismiss,
}: {
  testIdPrefix: string;
  icon: ReactNode;
  text: string;
  containerClass: string;
  archiveClass: string;
  dismissClass: string;
  onArchive: () => void;
  onDismiss: () => void;
}) {
  return (
    <div data-testid={`${testIdPrefix}-banner`} className={containerClass}>
      {icon}
      <span className="flex-1">{text}</span>
      <button
        type="button"
        data-testid={`${testIdPrefix}-archive-button`}
        onClick={onArchive}
        className={archiveClass}
      >
        Archive
      </button>
      <button
        type="button"
        aria-label="Dismiss"
        data-testid={`${testIdPrefix}-dismiss-button`}
        onClick={onDismiss}
        className={dismissClass}
      >
        <IconX className="h-3 w-3" />
      </button>
    </div>
  );
}

export function PRMergedBanner({ taskId }: { taskId: string }) {
  const taskPRs = useAppStore((state) => state.taskPRs.byTaskId[taskId]);
  const [dismissed, setDismissed] = useState(() => wasPRMergedBannerDismissed(taskId));
  const handleArchive = useArchiveTaskAction(taskId);

  const handleDismiss = useCallback(() => {
    markPRMergedBannerDismissed(taskId);
    setDismissed(true);
  }, [taskId]);

  // Multi-repo: only show "ready to archive" once every PR is merged. A
  // single merged repo with others still open means the task isn't done yet.
  const allMerged = !!taskPRs && taskPRs.length > 0 && taskPRs.every((pr) => pr.state === "merged");
  if (!allMerged || dismissed) return null;

  const bannerText =
    taskPRs.length === 1
      ? `PR #${taskPRs[0].pr_number} has been merged. You can archive this task.`
      : `All ${taskPRs.length} PRs have been merged. You can archive this task.`;

  return (
    <ArchiveDismissBanner
      testIdPrefix="pr-merged"
      icon={<IconGitMerge className="h-3.5 w-3.5 shrink-0" />}
      text={bannerText}
      containerClass="flex flex-1 items-center gap-2 rounded-md bg-purple-500/10 px-2 py-1 text-purple-600 dark:text-purple-400"
      archiveClass="underline underline-offset-2 hover:text-purple-700 dark:hover:text-purple-300 cursor-pointer"
      dismissClass="p-0.5 hover:bg-purple-500/10 rounded cursor-pointer"
      onArchive={handleArchive}
      onDismiss={handleDismiss}
    />
  );
}

export function PRClosedBanner({ taskId }: { taskId: string }) {
  const taskPRs = useAppStore((state) => state.taskPRs.byTaskId[taskId]);
  const [dismissed, setDismissed] = useState(() => wasPRClosedBannerDismissed(taskId));
  const handleArchive = useArchiveTaskAction(taskId);

  const handleDismiss = useCallback(() => {
    markPRClosedBannerDismissed(taskId);
    setDismissed(true);
  }, [taskId]);

  // Mirror the merged banner's all-or-nothing rule: show only once every PR is
  // closed-without-merging. A mix of merged + closed shows neither banner.
  const allClosed = !!taskPRs && taskPRs.length > 0 && taskPRs.every((pr) => pr.state === "closed");
  if (!allClosed || dismissed) return null;

  const bannerText =
    taskPRs.length === 1
      ? `PR #${taskPRs[0].pr_number} was closed without merging. You can archive this task.`
      : `All ${taskPRs.length} PRs were closed without merging. You can archive this task.`;

  return (
    <ArchiveDismissBanner
      testIdPrefix="pr-closed"
      icon={<IconGitPullRequestClosed className="h-3.5 w-3.5 shrink-0" />}
      text={bannerText}
      containerClass="flex flex-1 items-center gap-2 rounded-md bg-red-500/10 px-2 py-1 text-red-600 dark:text-red-400"
      archiveClass="underline underline-offset-2 hover:text-red-700 dark:hover:text-red-300 cursor-pointer"
      dismissClass="p-0.5 hover:bg-red-500/10 rounded cursor-pointer"
      onArchive={handleArchive}
      onDismiss={handleDismiss}
    />
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
      {queueChip}
      {/* Distinct per-banner keys: the key remounts the banner on task switch
          so its dismissed state re-initialises, and keeping the two suffixes
          different avoids a duplicate-sibling-key collision. */}
      {taskId && <PRMergedBanner key={`${taskId}-merged`} taskId={taskId} />}
      {taskId && <PRClosedBanner key={`${taskId}-closed`} taskId={taskId} />}
      {canShare && taskId && sessionId && (
        <div className="ml-auto">
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
    <div className="bg-card flex-shrink-0 px-2 pb-2 pt-1">
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
            panelState.planComments.length > 0 || panelState.pendingPRFeedback.length > 0
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
