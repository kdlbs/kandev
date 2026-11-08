"use client";

import { memo, useCallback, useEffect, useRef, useState } from "react";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import {
  type ChatInputContainerHandle,
  type ChatSubmitPayload,
  type ChatSubmitResult,
} from "@/components/task/chat/chat-input-container";
import { MessageList } from "@/components/task/chat/message-list";
import { useVisibleItems } from "./startup-rows";
import { useCopilotActivity } from "@/app/coordinator/copilot/use-copilot-activity";
import { ActivityStatusSlot } from "@/app/coordinator/copilot/activity-status-line";
import { useChatPanelState } from "@/components/task/chat/use-chat-panel-state";
import {
  ChatInputArea,
  useSubmitHandler,
  useChatPanelHandlers,
} from "@/components/task/chat/chat-input-area";
import { ClarificationPanelSection } from "@/components/task/chat/clarification-panel-section";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import {
  routePanelClick,
  routePanelMouseDown,
} from "@/components/task/chat/route-panel-mouse-down";
import { useQuickChatInitialPrompt } from "./use-quick-chat-initial-prompt";
import { useQuickChatInitialDraft } from "./use-quick-chat-initial-draft";
import { useQuickChatInitialPromptRecovery } from "./use-quick-chat-initial-prompt-recovery";
import { QuickChatCancelCommands } from "./quick-chat-cancel-commands";
import { useLateClarificationMessage } from "@/hooks/use-late-clarification-message";
import type { QuickChatInitialPrompt } from "@/lib/state/slices/ui/types";

type QuickChatContentProps = {
  sessionId: string;
  loadSettings?: boolean;
  minimalToolbar?: boolean;
  placeholderOverride?: string;
  initialPrompt?: QuickChatInitialPrompt;
  onInitialPromptAttempted?: () => void;
  /** The accepted Quick Chat launch owns input until its initial startup settles. */
  startupPending?: boolean;
  /** Inserted once through `chatInputRef.insertText`, not sent. See
   *  {@link useQuickChatInitialDraft}. */
  initialDraft?: string;
  /** Applied to the composer message once per submit, before
   *  `buildSubmitMessage`. See {@link useSubmitHandler}. */
  transformOutgoing?: (message: string) => string;
  /** See {@link useVisibleItems}. */
  hideStartupRows?: boolean;
  /** Coordinator copilot: one status line while a turn runs, one chip of tool
   *  calls after it. See {@link useCopilotActivity}. */
  activityDisplay?: boolean;
};

/** Bundles the composer-priming hooks: a rejected launch prompt is restored
 *  as a manual draft, and a caller-supplied draft is inserted once the
 *  prompt (if any) clears. */
function useQuickChatComposerPriming({
  sessionId,
  taskId,
  initialPrompt,
  initialDraft,
  blocked,
  handleSubmit,
  onInitialPromptAttempted,
  chatInputRef,
}: {
  sessionId: string;
  taskId: string | null;
  initialPrompt?: QuickChatInitialPrompt;
  initialDraft?: string;
  blocked: boolean;
  handleSubmit: (payload: ChatSubmitPayload) => ChatSubmitResult;
  onInitialPromptAttempted?: () => void;
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>;
}) {
  const { restoreRejectedPrompt, clearAcceptedPrompt } = useQuickChatInitialPromptRecovery(
    sessionId,
    chatInputRef,
  );

  useQuickChatInitialPrompt({
    sessionId,
    taskId,
    prompt: initialPrompt,
    blocked,
    submit: handleSubmit,
    onAttempted: onInitialPromptAttempted,
    onAccepted: clearAcceptedPrompt,
    onRejected: restoreRejectedPrompt,
  });

  useQuickChatInitialDraft({ draft: initialDraft, initialPrompt, chatInputRef });
}

function useQuickChatState(
  sessionId: string,
  transformOutgoing?: (message: string) => string,
  loadSettings = true,
) {
  const chatInputRef = useRef<ChatInputContainerHandle>(null);

  useSettingsData(loadSettings);
  const panelState = useChatPanelState({
    sessionId,
    onOpenFile: undefined,
    onOpenFileAtLine: undefined,
  });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, undefined, {
    transformOutgoing,
  });
  const { handleCancelTurn } = useChatPanelHandlers(panelState.resolvedSessionId, chatInputRef);

  return {
    chatInputRef,
    panelState,
    isSending,
    handleSubmit,
    handleCancelTurn,
  };
}

function QuickChatInputArea({
  state,
  clarificationKey,
  onClarificationResolved,
  minimalToolbar,
  placeholderOverride,
}: {
  state: ReturnType<typeof useQuickChatState>;
  clarificationKey: number;
  onClarificationResolved: () => void;
  minimalToolbar?: boolean;
  placeholderOverride?: string;
}) {
  return (
    <ChatInputArea
      chatInputRef={state.chatInputRef}
      clarificationKey={clarificationKey}
      onClarificationResolved={onClarificationResolved}
      handleSubmit={state.handleSubmit}
      handleCancelTurn={state.handleCancelTurn}
      showRequestChangesTooltip={false}
      onRequestChangesTooltipDismiss={undefined}
      panelState={state.panelState}
      isSending={state.isSending}
      hideSessionsDropdown={true}
      minimalToolbar={minimalToolbar}
      hidePlanMode={true}
      placeholderOverride={placeholderOverride}
      surfaceClassName="bg-popover"
    />
  );
}

function useFocusComposerOnMount(chatInputRef: React.RefObject<ChatInputContainerHandle | null>) {
  useEffect(() => {
    const timer = setTimeout(() => chatInputRef.current?.focusInput(), 50);
    return () => clearTimeout(timer);
  }, [chatInputRef]);
}

function useQuickChatClarificationHost() {
  const shortcutScopeRef = useRef<HTMLDivElement>(null);
  const cancelCommandsRef = useRef<HTMLDivElement>(null);
  const activityStatusRef = useRef<HTMLDivElement>(null);
  const chatInputAreaRef = useRef<HTMLDivElement>(null);
  const clarificationReservations = [cancelCommandsRef, activityStatusRef, chatInputAreaRef];
  const handleShortcutScopeMouseDown = useCallback(
    (event: React.MouseEvent<HTMLDivElement>) => routePanelMouseDown(event, shortcutScopeRef),
    [],
  );
  return {
    shortcutScopeRef,
    cancelCommandsRef,
    activityStatusRef,
    chatInputAreaRef,
    clarificationReservations,
    handleShortcutScopeMouseDown,
  };
}

type QuickChatMessageSurfaceProps = {
  items: ReturnType<typeof useVisibleItems>;
  panelState: ReturnType<typeof useChatPanelState>;
  taskId: string | null;
};

function QuickChatMessageSurface({ items, panelState, taskId }: QuickChatMessageSurfaceProps) {
  return (
    <div className="flex-1 min-h-0 overflow-hidden bg-popover" data-testid="quick-chat-messages">
      <MessageList
        items={items}
        messages={panelState.allMessages}
        footerActionMessages={panelState.footerActionMessages}
        permissionsByToolCallId={panelState.permissionsByToolCallId}
        childrenByParentToolCallId={panelState.childrenByParentToolCallId}
        taskId={taskId ?? undefined}
        sessionId={panelState.resolvedSessionId}
        messagesLoading={panelState.messagesLoading}
        isWorking={panelState.isWorking}
        sessionState={panelState.session?.state}
        worktreePath={getSessionWorkspacePath(panelState.session)}
      />
    </div>
  );
}

function isQuickChatComposerBlocked(
  startupPending: boolean | undefined,
  panelState: ReturnType<typeof useChatPanelState>,
): boolean {
  return (
    startupPending ||
    !panelState.session ||
    panelState.inputMode === "unavailable" ||
    (panelState.inputMode === "queue" && (!panelState.isQueueReady || panelState.isLoading)) ||
    (panelState.planCommentMigration?.isBlocking ?? false)
  );
}

export const QuickChatContent = memo(function QuickChatContent({
  sessionId,
  minimalToolbar,
  placeholderOverride,
  initialPrompt,
  onInitialPromptAttempted,
  startupPending,
  initialDraft,
  transformOutgoing,
  hideStartupRows,
  activityDisplay = false,
  loadSettings,
}: QuickChatContentProps) {
  const [clarificationKey, setClarificationKey] = useState(0);
  const {
    shortcutScopeRef,
    cancelCommandsRef,
    activityStatusRef,
    chatInputAreaRef,
    clarificationReservations,
    handleShortcutScopeMouseDown,
  } = useQuickChatClarificationHost();
  const state = useQuickChatState(sessionId, transformOutgoing, loadSettings);
  const { chatInputRef, panelState, handleSubmit, handleCancelTurn } = state;
  const { taskId, pendingClarification, pendingClarificationGroup } = panelState;
  const lateAnswer = useLateClarificationMessage(pendingClarificationGroup?.[0]);
  const activity = useCopilotActivity(activityDisplay, panelState);
  const items = useVisibleItems(activity.items, hideStartupRows);
  useFocusComposerOnMount(chatInputRef);

  useQuickChatComposerPriming({
    sessionId,
    taskId,
    initialPrompt,
    initialDraft,
    blocked: isQuickChatComposerBlocked(startupPending, panelState),
    handleSubmit,
    onInitialPromptAttempted,
    chatInputRef,
  });

  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);
  return (
    <div
      ref={shortcutScopeRef}
      data-testid="quick-chat-content"
      tabIndex={-1}
      onMouseDown={handleShortcutScopeMouseDown}
      onClick={(event) => routePanelClick(event, shortcutScopeRef)}
      className="flex flex-col flex-1 min-h-0 outline-none"
    >
      <div ref={cancelCommandsRef} className="shrink-0">
        <QuickChatCancelCommands
          sessionId={sessionId}
          isWorking={panelState.isWorking}
          pendingClarification={pendingClarification}
          onCancel={handleCancelTurn}
        />
      </div>
      <QuickChatMessageSurface items={items} panelState={panelState} taskId={taskId} />
      <ClarificationPanelSection
        key={sessionId}
        pending={Boolean(pendingClarification)}
        messages={pendingClarificationGroup}
        agentDisconnected={panelState.session?.pending_action === null}
        onResolved={handleClarificationResolved}
        onLateAnswer={lateAnswer.send}
        lateAnswerState={lateAnswer.state}
        shortcutScopeRef={shortcutScopeRef}
        initialMaxHeightVh={35}
        allocationRef={shortcutScopeRef}
        anchorRef={shortcutScopeRef}
        reservedSiblingRefs={clarificationReservations}
        reservedHeightPx={80}
      />
      <div ref={activityStatusRef} className="shrink-0">
        <ActivityStatusSlot line={activity.statusLine} />
      </div>
      <div ref={chatInputAreaRef} className="shrink-0">
        <QuickChatInputArea
          state={state}
          clarificationKey={clarificationKey}
          onClarificationResolved={handleClarificationResolved}
          minimalToolbar={minimalToolbar}
          placeholderOverride={placeholderOverride}
        />
      </div>
    </div>
  );
});
