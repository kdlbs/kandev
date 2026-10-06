"use client";

import { memo, useCallback, useEffect, useRef, useState } from "react";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { type ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";
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
  minimalToolbar?: boolean;
  placeholderOverride?: string;
  initialPrompt?: QuickChatInitialPrompt;
  onInitialPromptAttempted?: () => void;
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

function useQuickChatState(sessionId: string, transformOutgoing?: (message: string) => string) {
  const chatInputRef = useRef<ChatInputContainerHandle>(null);

  useSettingsData(true);
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

function useQuickChatComposerPriming({
  sessionId,
  initialPrompt,
  initialDraft,
  onInitialPromptAttempted,
  state,
}: Pick<
  QuickChatContentProps,
  "sessionId" | "initialPrompt" | "initialDraft" | "onInitialPromptAttempted"
> & {
  state: ReturnType<typeof useQuickChatState>;
}) {
  const { chatInputRef, panelState, handleSubmit } = state;
  const { taskId } = panelState;
  const { restoreRejectedPrompt, clearAcceptedPrompt } = useQuickChatInitialPromptRecovery(
    sessionId,
    chatInputRef,
  );

  useEffect(() => {
    const timer = setTimeout(() => chatInputRef.current?.focusInput(), 50);
    return () => clearTimeout(timer);
  }, [chatInputRef]);

  useQuickChatInitialPrompt({
    sessionId,
    taskId,
    prompt: initialPrompt,
    blocked:
      !panelState.session ||
      panelState.inputMode === "unavailable" ||
      (panelState.inputMode === "queue" && (!panelState.isQueueReady || panelState.isLoading)) ||
      (panelState.planCommentMigration?.isBlocking ?? false),
    submit: handleSubmit,
    onAttempted: onInitialPromptAttempted,
    onAccepted: clearAcceptedPrompt,
    onRejected: restoreRejectedPrompt,
  });

  useQuickChatInitialDraft({ draft: initialDraft, initialPrompt, chatInputRef });
}

export const QuickChatContent = memo(function QuickChatContent({
  sessionId,
  minimalToolbar,
  placeholderOverride,
  initialPrompt,
  onInitialPromptAttempted,
  initialDraft,
  transformOutgoing,
  hideStartupRows,
  activityDisplay = false,
}: QuickChatContentProps) {
  const [clarificationKey, setClarificationKey] = useState(0);
  const shortcutScopeRef = useRef<HTMLDivElement>(null);
  const state = useQuickChatState(sessionId, transformOutgoing);
  const { chatInputRef, panelState, isSending, handleSubmit, handleCancelTurn } = state;
  const { taskId, pendingClarification, pendingClarificationGroup } = panelState;
  const lateAnswer = useLateClarificationMessage(pendingClarificationGroup?.[0]);
  const activity = useCopilotActivity(activityDisplay, panelState);
  const items = useVisibleItems(activity.items, hideStartupRows);
  useQuickChatComposerPriming({
    sessionId,
    initialPrompt,
    initialDraft,
    onInitialPromptAttempted,
    state,
  });

  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);
  const handleShortcutScopeMouseDown = useCallback(
    (event: React.MouseEvent<HTMLDivElement>) => routePanelMouseDown(event, shortcutScopeRef),
    [],
  );
  return (
    <div
      ref={shortcutScopeRef}
      data-testid="quick-chat-content"
      tabIndex={-1}
      onMouseDown={handleShortcutScopeMouseDown}
      onClick={(event) => routePanelClick(event, shortcutScopeRef)}
      className="flex flex-col flex-1 min-h-0 outline-none"
    >
      <QuickChatCancelCommands
        sessionId={sessionId}
        isWorking={panelState.isWorking}
        pendingClarification={pendingClarification}
        onCancel={handleCancelTurn}
      />
      <div className="flex-1 min-h-0 overflow-hidden bg-popover" data-testid="quick-chat-messages">
        <MessageList
          items={items}
          messages={panelState.allMessages}
          permissionsByToolCallId={panelState.permissionsByToolCallId}
          childrenByParentToolCallId={panelState.childrenByParentToolCallId}
          taskId={taskId ?? undefined}
          sessionId={panelState.resolvedSessionId}
          messagesLoading={panelState.messagesLoading}
          isWorking={panelState.isWorking}
          sessionState={panelState.session?.state}
          worktreePath={getSessionWorkspacePath(panelState.session)}
          onOpenFile={undefined}
        />
      </div>
      <ClarificationPanelSection
        key={sessionId}
        pending={Boolean(pendingClarification)}
        messages={pendingClarificationGroup}
        agentDisconnected={panelState.session?.pending_action === null}
        onResolved={handleClarificationResolved}
        onLateAnswer={lateAnswer.send}
        lateAnswerState={lateAnswer.state}
        shortcutScopeRef={shortcutScopeRef}
        maxHeightVh={35}
      />
      <ActivityStatusSlot line={activity.statusLine} />
      <ChatInputArea
        chatInputRef={chatInputRef}
        clarificationKey={clarificationKey}
        onClarificationResolved={handleClarificationResolved}
        handleSubmit={handleSubmit}
        handleCancelTurn={handleCancelTurn}
        showRequestChangesTooltip={false}
        onRequestChangesTooltipDismiss={undefined}
        panelState={panelState}
        isSending={isSending}
        hideSessionsDropdown={true}
        minimalToolbar={minimalToolbar}
        hidePlanMode={true}
        placeholderOverride={placeholderOverride}
        surfaceClassName="bg-popover"
      />
    </div>
  );
});
