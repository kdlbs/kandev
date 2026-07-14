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
import { ResizeHandle } from "@/components/task/chat/resize-handle";
import { useResizableClarificationOverlay } from "@/hooks/use-resizable-clarification-overlay";

type QuickChatContentProps = {
  sessionId: string;
  minimalToolbar?: boolean;
  placeholderOverride?: string;
  initialPrompt?: string;
  onInitialPromptSent?: () => void;
};

function useQuickChatState(sessionId: string) {
  const chatInputRef = useRef<ChatInputContainerHandle>(null);

  useSettingsData(true);
  const panelState = useChatPanelState({
    sessionId,
    onOpenFile: undefined,
    onOpenFileAtLine: undefined,
  });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, undefined);
  const { handleCancelTurn } = useChatPanelHandlers(panelState.resolvedSessionId, chatInputRef);

  return {
    chatInputRef,
    panelState,
    isSending,
    handleSubmit,
    handleCancelTurn,
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
  const { chatInputRef, panelState, isSending, handleSubmit, handleCancelTurn } = state;
  const { taskId, pendingClarification, pendingClarificationGroup } = panelState;

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
  const {
    height: clarificationHeight,
    containerRef: clarificationContainerRef,
    resetHeight: clarificationResetHeight,
    resizeHandleProps: clarificationResizeProps,
  } = useResizableClarificationOverlay();

  // Reset the dragged height when the overlay closes so a fresh
  // clarification starts auto-sized instead of inheriting a stale value.
  useEffect(() => {
    if (!pendingClarification) clarificationResetHeight();
  }, [pendingClarification, clarificationResetHeight]);

  return (
    <div className="flex flex-col flex-1 min-h-0">
      <div className="flex-1 min-h-0 overflow-hidden bg-popover" data-testid="quick-chat-messages">
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
          worktreePath={panelState.session?.worktree_path}
          onOpenFile={undefined}
        />
      </div>
      {pendingClarification && (
        <div className="relative flex-shrink-0 border-t border-sky-400/30 bg-card">
          <ResizeHandle {...clarificationResizeProps} />
          <div
            ref={clarificationContainerRef}
            data-testid="clarification-overlay-container"
            className="px-1 overflow-y-scroll overscroll-contain max-h-[50vh]"
            style={clarificationHeight === null ? undefined : { height: clarificationHeight }}
          >
            <ClarificationInputOverlay
              messages={pendingClarificationGroup}
              onResolved={handleClarificationResolved}
            />
          </div>
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
        surfaceClassName="bg-popover"
      />
    </div>
  );
});
