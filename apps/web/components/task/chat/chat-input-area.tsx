"use client";

import { useCallback, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { useMessageHandler, buildTaskMentionsContext } from "@/hooks/use-message-handler";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import {
  ChatInputContainer,
  type ChatSubmitPayload,
  type ChatSubmitResult,
  type ChatInputContainerHandle,
} from "@/components/task/chat/chat-input-container";
import { QueueAffordance } from "@/components/task/chat/queued-ghost-list";
import { ComposerAgentStartHint } from "./composer-agent-start-hint";
import {
  formatReviewCommentsAsMarkdown,
  formatPRFeedbackAsMarkdown,
  formatPlanCommentsAsMarkdown,
  formatWalkthroughCommentsAsMarkdown,
  formatAgentMessageCommentsAsMarkdown,
} from "@/lib/state/slices/comments/format";
import { usePlanActions } from "@/hooks/domains/kanban/use-plan-actions";
import { useExecutorEnvironmentAvailability } from "@/hooks/domains/session/use-executor-environment-availability";
import { useToast } from "@/components/toast-provider";
import { isMessageSendError } from "@/lib/chat/message-send-error";
import type { DiffComment } from "@/lib/diff/types";
import type { AgentMessageComment } from "@/lib/state/slices/comments";
import type { ChatPanelState } from "./use-chat-panel-state";
import { useComposerProps } from "./use-composer-props";
import { cn } from "@/lib/utils";
import { resolveComposerWorkspaceId } from "./composer-workspace";
import { t } from "@/lib/i18n";
import { ChatStatusBar, resolveStatusRowTaskId } from "./chat-status-bar";
import { DynamicRouteRecovery } from "./dynamic-route-recovery";

const PLAN_CONTEXT_PATH = "plan:context";

/**
 * Prepends any pending review/walkthrough/PR-feedback/plan/message comments
 * to the composer text as Markdown, in a fixed stacking order, before send.
 */
export function buildSubmitMessage({
  message,
  reviewComments,
  pendingPRFeedback,
  planComments,
  walkthroughComments = [],
  messageComments = [],
}: {
  message: string;
  reviewComments?: DiffComment[];
  pendingPRFeedback: import("@/lib/state/slices/comments").PRFeedbackComment[];
  planComments: import("@/lib/state/slices/comments").PlanComment[];
  walkthroughComments?: import("@/lib/state/slices/comments").WalkthroughComment[];
  messageComments?: AgentMessageComment[];
}): string {
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
  if (messageComments.length > 0) {
    const messageCommentsMarkdown = formatAgentMessageCommentsAsMarkdown(messageComments);
    finalMessage = finalMessage
      ? `${messageCommentsMarkdown}${finalMessage}`
      : messageCommentsMarkdown;
  }
  return finalMessage;
}

/** Resolves the composer placeholder text for the current agent/document/plan state. */
export function resolveInputPlaceholder(
  isAgentBusy: boolean,
  activeDocumentType: string | undefined,
  planModeEnabled: boolean,
  hasClarification: boolean,
  needsRecovery: boolean,
): string {
  if (needsRecovery) return t("task:composerPlaceholderRecovery");
  if (hasClarification) return t("task:composerPlaceholderClarification");
  if (isAgentBusy) return t("task:composerPlaceholderBusy");
  if (activeDocumentType === "file") return t("task:composerPlaceholderFile");
  if (planModeEnabled) return t("task:composerPlaceholderPlan");
  return t("task:composerPlaceholderTask");
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

/** Picks the composer placeholder: an explicit override wins, then the
 *  "switching agent" state, then {@link resolveInputPlaceholder}. */
function pickInputPlaceholder(a: PlaceholderArgs): string {
  if (a.isMoving) return t("task:composerPlaceholderSwitchingAgent");
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

/** Shows an error toast for a failed message send, distinguishing a known
 *  send error from an ambiguous connection drop/timeout. */
function showMessageSendToast(error: unknown, toast: ReturnType<typeof useToast>["toast"]) {
  console.error("Failed to send message:", error);
  if (isMessageSendError(error)) {
    toast({
      title: t("task:messageNotSent"),
      description: error.message,
      variant: "error",
    });
    return;
  }
  toast({
    title: t("task:messageSendStatusUnknown"),
    description: t("task:theConnectionDroppedOrTimedOut"),
    variant: "error",
  });
}

/** Adapts {@link useChatPanelState}'s fields into {@link useMessageHandler}'s params. */
function usePanelMessageHandler(panelState: ChatPanelState) {
  const {
    resolvedSessionId,
    sessionModel,
    activeModel,
    pendingClarification,
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
    hasPendingClarification: !!pendingClarification,
    activeDocument,
    planComments,
    contextFiles,
    prompts,
  });
}
/** Builds the composer's submit handler, tracking in-flight sends and
 *  routing errors to a toast. */
export function useSubmitHandler(
  panelState: ChatPanelState,
  onSend?: (payload: ChatSubmitPayload) => ChatSubmitResult,
) {
  const [isSending, setIsSending] = useState(false);
  const storeApi = useAppStoreApi();
  const { toast } = useToast();
  const {
    resolvedSessionId,
    planComments,
    pendingPRFeedback,
    walkthroughComments,
    messageComments,
    markCommentsSent,
    clearSessionPlanComments,
    handleClearPRFeedback,
    handleClearWalkthroughComments,
    clearEphemeral,
    addContextFile,
    planModeEnabled,
    pendingClarification,
  } = panelState;
  const { handleSendMessage } = usePanelMessageHandler(panelState);

  const handleSubmit = useCallback(
    async (payload: ChatSubmitPayload) => {
      if (isSending) return;
      setIsSending(true);
      try {
        const finalMessage = buildSubmitMessage({
          message: payload.message,
          reviewComments: payload.reviewComments,
          pendingPRFeedback,
          planComments,
          walkthroughComments,
          messageComments,
        });
        const outbound = { ...payload, message: finalMessage };
        if (onSend && !pendingClarification) {
          // Expand task mentions because onSend bypasses useMessageHandler.buildFinalMessage.
          const taskCtx = payload.inlineTaskMentions?.length
            ? buildTaskMentionsContext(payload.inlineTaskMentions, storeApi.getState())
            : "";
          await onSend({ ...outbound, message: finalMessage + taskCtx });
        } else {
          await handleSendMessage(outbound);
        }
        if (payload.reviewComments && payload.reviewComments.length > 0)
          markCommentsSent(payload.reviewComments.map((c) => c.id));
        if (messageComments.length > 0) markCommentsSent(messageComments.map((c) => c.id));
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
        showMessageSendToast(error, toast);
        return false;
      } finally {
        setIsSending(false);
      }
    },
    [
      isSending,
      onSend,
      pendingClarification,
      storeApi,
      handleSendMessage,
      markCommentsSent,
      planComments,
      clearSessionPlanComments,
      walkthroughComments,
      messageComments,
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

/** Builds the cancel-turn handler and registers the global focus-input keyboard shortcut. */
export function useChatPanelHandlers(
  resolvedSessionId: string | null,
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>,
  options: { enableFocusShortcut?: boolean } = {},
) {
  const enableFocusShortcut = options.enableFocusShortcut ?? true;
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
    { enabled: enableFocusShortcut, preventDefault: false },
  );

  return { handleCancelTurn };
}

type ChatInputAreaProps = {
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>;
  clarificationKey: number;
  onClarificationResolved: () => void;
  handleSubmit: (payload: ChatSubmitPayload) => ChatSubmitResult;
  handleCancelTurn: () => Promise<void>;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  panelState: ChatPanelState;
  isSending: boolean;
  hideSessionsDropdown?: boolean;
  minimalToolbar?: boolean;
  /** Hide ACP/session-specific controls (model picker, mode, MCP, reset context,
   *  sessions, enhance, plugin actions) while keeping Plan, attachment and
   *  context controls. Surfaces whose agent only exists for the length of a turn
   *  — an automation run — flip this off the moment the turn ends, so a control
   *  that talks to a live ACP session is never left on screen without one. */
  hideAgentControls?: boolean;
  /** Hide the plan mode toggle button (for ephemeral/quick chat sessions) */
  hidePlanMode?: boolean;
  placeholderOverride?: string;
  surfaceClassName?: string;
  /** Always-on affordance: scrolls the transcript to the top of the last
   * user prompt. Omitted callers (e.g. quick chat) render no button. */
  showScrollToLastPrompt?: boolean;
  onScrollToLastPrompt?: () => void;
  /** Direction the last prompt actually sits in, driving the scroll
   * button's icon. Ignored while `showScrollToLastPrompt` is falsy. */
  lastPromptScrollDirection?: "up" | "down";
  /** Shown once the first message scrolls out of view: jumps back to the
   * start of the transcript. */
  showScrollToStart?: boolean;
  onScrollToStart?: () => void;
  /**
   * Task this composer belongs to, for the status row only. Hosts that mount a
   * task's chat before any session exists pass it so the dependency / autopilot
   * chips still render; without it the row is hidden on exactly the tasks a
   * dependency chip is about.
   */
  statusTaskId?: string | null;
  /** Recovered-idle (resume-skipped) sessions render the "a message will
   * auto-start the agent" hint above the composer while the agent is
   * stopped. */
  showAgentStartHint?: boolean;
};

/** Resolves whether this session's executor environment is unavailable, and why. */
function useExecutorUnavailable(taskId: string | null, sessionId: string | null) {
  const availability = useExecutorEnvironmentAvailability(taskId, Boolean(sessionId && taskId));
  return {
    unavailable: availability.unavailable,
    reason: availability.status?.label,
  };
}

/** Resolves the workspace ID the composer should attribute a new message to. */
function useComposerWorkspaceId(sessionId: string | null, taskId: string | null) {
  return useAppStore((state) =>
    resolveComposerWorkspaceId({
      sessionId,
      taskId,
      quickChatSessions: state.quickChat.sessions,
      activeWorkflowId: state.kanban.workflowId,
      activeTasks: state.kanban.tasks,
      snapshots: Object.values(state.kanbanMulti.snapshots),
      workflows: state.workflows.items,
    }),
  );
}

/** Derives the composer's plan actions, executor-availability state, and placeholder text. */
function useChatInputDerived(
  panelState: ChatPanelState,
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

/**
 * The chat composer: input box, submit/cancel handling, plan-mode toggle,
 * clarification banner, and the {@link ChatStatusBar} above it.
 */
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
  hideAgentControls,
  hidePlanMode,
  placeholderOverride,
  surfaceClassName,
  showScrollToLastPrompt,
  onScrollToLastPrompt,
  lastPromptScrollDirection,
  showScrollToStart,
  onScrollToStart,
  statusTaskId = null,
  showAgentStartHint = false,
}: ChatInputAreaProps) {
  const { resolvedSessionId, taskId, isAgentBusy } = panelState;
  const statusRowTaskId = resolveStatusRowTaskId(taskId, statusTaskId);
  const composerWorkspaceId = useComposerWorkspaceId(resolvedSessionId, taskId);
  const sessionState = panelState.session?.state ?? null;
  const { planActions, executor, placeholder } = useChatInputDerived(
    panelState,
    chatInputRef,
    placeholderOverride,
  );
  const { implementPlanHandler, proceedStepName, proceed, isMoving } = planActions;
  const composerProps = useComposerProps({
    panelState,
    composerWorkspaceId,
    isMoving,
    implementPlanHandler,
    executor,
    placeholder,
    handleSubmit,
    handleCancelTurn,
    isSending,
    showRequestChangesTooltip,
    onRequestChangesTooltipDismiss,
    onClarificationResolved,
    hideSessionsDropdown,
    minimalToolbar,
    hideAgentControls,
    hidePlanMode,
  });
  return (
    <div
      data-testid="chat-input-area"
      className={cn("bg-card flex-shrink-0 px-2 pb-2 pt-1", surfaceClassName)}
    >
      <DynamicRouteRecovery session={panelState.session} />
      <ComposerAgentStartHint
        show={showAgentStartHint}
        needsRecovery={panelState.needsRecovery}
        executorUnavailable={executor.unavailable}
        hasPendingClarification={Boolean(panelState.pendingClarification)}
      />
      <QueueAffordance
        sessionId={resolvedSessionId}
        renderStatusBar={(queueChip) => (
          <ChatStatusBar
            todoItems={panelState.todoItems}
            taskId={statusRowTaskId}
            sessionId={resolvedSessionId}
            sessionState={sessionState}
            nextStepName={proceedStepName}
            onProceed={proceed}
            isAgentBusy={isAgentBusy}
            isMoving={isMoving}
            queueChip={queueChip}
            showScrollToLastPrompt={showScrollToLastPrompt}
            onScrollToLastPrompt={onScrollToLastPrompt}
            lastPromptScrollDirection={lastPromptScrollDirection}
            showScrollToStart={showScrollToStart}
            onScrollToStart={onScrollToStart}
          />
        )}
      >
        <ChatInputContainer ref={chatInputRef} key={clarificationKey} {...composerProps} />
      </QueueAffordance>
    </div>
  );
}
