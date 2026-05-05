"use client";

import { memo, useCallback, useEffect, useRef, useState } from "react";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { type ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";
import {
  QueuedMessageIndicator,
  type QueuedMessageIndicatorHandle,
} from "@/components/task/chat/queued-message-indicator";
import { MessageList } from "@/components/task/chat/message-list";
import { useChatPanelState } from "@/components/task/chat/use-chat-panel-state";
import {
  ChatInputArea,
  useSubmitHandler,
  useChatPanelHandlers,
} from "@/components/task/chat/chat-input-area";
import { ClarificationInputOverlay } from "@/components/task/chat/clarification-input-overlay";

type QuickChatContentProps = {
  sessionId: string;
  minimalToolbar?: boolean;
  placeholderOverride?: string;
  initialPrompt?: string;
  onInitialPromptSent?: () => void;
};

function useQuickChatState(sessionId: string) {
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  const queuedMessageRef = useRef<QueuedMessageIndicatorHandle>(null);

  useSettingsData(true);
  const panelState = useChatPanelState({
    sessionId,
    onOpenFile: undefined,
    onOpenFileAtLine: undefined,
  });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, undefined);
  const { cancelQueue } = panelState;
  const { handleCancelTurn, handleCancelQueue, handleQueueEditComplete } = useChatPanelHandlers(
    panelState.resolvedSessionId,
    cancelQueue,
    chatInputRef,
  );

  return {
    chatInputRef,
    queuedMessageRef,
    panelState,
    isSending,
    handleSubmit,
    handleCancelTurn,
    handleCancelQueue,
    handleQueueEditComplete,
  };
}

export const QuickChatContent = memo(function QuickChatContent({
  sessionId,
  minimalToolbar,
  placeholderOverride,
  initialPrompt,
  onInitialPromptSent,
}: QuickChatContentProps) {
  const [clarificationKey, setClarificationKey] = useState(0);
  const initialPromptSent = useRef(false);
  const state = useQuickChatState(sessionId);
  const {
    chatInputRef,
    queuedMessageRef,
    panelState,
    isSending,
    handleSubmit,
    handleCancelTurn,
    handleCancelQueue,
    handleQueueEditComplete,
  } = state;
  const { taskId, pendingClarification, isQueued, queuedMessage, updateQueueContent } = panelState;

  useEffect(() => {
    const timer = setTimeout(() => chatInputRef.current?.focusInput(), 50);
    return () => clearTimeout(timer);
  }, [chatInputRef]);

  useEffect(() => {
    if (!initialPrompt || !taskId || initialPromptSent.current) return;
    initialPromptSent.current = true;
    handleSubmit(initialPrompt);
    onInitialPromptSent?.();
  }, [initialPrompt, taskId, handleSubmit, onInitialPromptSent]);

  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);

  return (
    <div className="flex flex-col flex-1 min-h-0">
      <div className="flex-1 min-h-0 overflow-hidden">
        <MessageList
          items={panelState.groupedItems}
          messages={panelState.allMessages}
          permissionsByToolCallId={panelState.permissionsByToolCallId}
          childrenByParentToolCallId={panelState.childrenByParentToolCallId}
          taskId={taskId ?? undefined}
          sessionId={panelState.resolvedSessionId}
          messagesLoading={panelState.messagesLoading}
          isWorking={panelState.isWorking}
          sessionState={panelState.session?.state}
          taskState={panelState.task?.state}
          worktreePath={panelState.session?.worktree_path}
          onOpenFile={undefined}
        />
      </div>
      {pendingClarification && (
        <div className="flex-shrink-0 border-t border-sky-400/30 bg-card px-1">
          <ClarificationInputOverlay
            message={pendingClarification}
            onResolved={handleClarificationResolved}
          />
        </div>
      )}
      {isQueued && queuedMessage && (
        <div className="flex-shrink-0 bg-card px-3 pt-1.5">
          <QueuedMessageIndicator
            ref={queuedMessageRef}
            content={queuedMessage.content}
            onCancel={handleCancelQueue}
            onUpdate={updateQueueContent}
            isVisible={true}
            onEditComplete={handleQueueEditComplete}
          />
        </div>
      )}
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
      />
    </div>
  );
});
