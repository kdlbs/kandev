"use client";

import { memo, useCallback, useEffect, useRef, useState } from "react";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { type ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";
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

export const QuickChatContent = memo(function QuickChatContent({
  sessionId,
  minimalToolbar,
  placeholderOverride,
  initialPrompt,
  onInitialPromptSent,
}: QuickChatContentProps) {
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  const [clarificationKey, setClarificationKey] = useState(0);
  const initialPromptSent = useRef(false);

  useSettingsData(true);
  const panelState = useChatPanelState({
    sessionId,
    onOpenFile: undefined,
    onOpenFileAtLine: undefined,
  });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, undefined);
  const {
    resolvedSessionId,
    session,
    task,
    taskId,
    isWorking,
    messagesLoading,
    groupedItems,
    allMessages,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    cancelQueue,
    pendingClarification,
  } = panelState;
  const { handleCancelTurn } = useChatPanelHandlers(resolvedSessionId, cancelQueue, chatInputRef);

  // Auto-send the initial prompt once the session is ready (has taskId).
  // This is used by config chat which creates the session first, then sends
  // the user's prompt through normal WS message flow.
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
          items={groupedItems}
          messages={allMessages}
          permissionsByToolCallId={permissionsByToolCallId}
          childrenByParentToolCallId={childrenByParentToolCallId}
          taskId={taskId ?? undefined}
          sessionId={resolvedSessionId}
          messagesLoading={messagesLoading}
          isWorking={isWorking}
          sessionState={session?.state}
          taskState={task?.state}
          worktreePath={session?.worktree_path}
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
        placeholderOverride={placeholderOverride}
      />
    </div>
  );
});
