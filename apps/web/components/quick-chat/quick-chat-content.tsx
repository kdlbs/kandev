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
import { ClarificationPanelSection } from "@/components/task/chat/clarification-panel-section";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import { routePanelMouseDown } from "@/components/task/chat/route-panel-mouse-down";

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
  const initialPromptSentFor = useRef<string | null>(null);
  const shortcutScopeRef = useRef<HTMLDivElement>(null);
  const state = useQuickChatState(sessionId);
  const { chatInputRef, panelState, isSending, handleSubmit, handleCancelTurn } = state;
  const { taskId, pendingClarification, pendingClarificationGroup } = panelState;

  useEffect(() => {
    const timer = setTimeout(() => chatInputRef.current?.focusInput(), 50);
    return () => clearTimeout(timer);
  }, [chatInputRef]);

  useEffect(() => {
    if (!initialPrompt || !taskId || initialPromptSentFor.current === sessionId) return;
    initialPromptSentFor.current = sessionId;
    handleSubmit({ message: initialPrompt });
    onInitialPromptSent?.();
  }, [initialPrompt, taskId, handleSubmit, onInitialPromptSent, sessionId]);

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
      className="flex flex-col flex-1 min-h-0 outline-none"
    >
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
          worktreePath={getSessionWorkspacePath(panelState.session)}
          onOpenFile={undefined}
        />
      </div>
      <ClarificationPanelSection
        key={sessionId}
        pending={Boolean(pendingClarification)}
        messages={pendingClarificationGroup}
        onResolved={handleClarificationResolved}
        shortcutScopeRef={shortcutScopeRef}
        maxHeightVh={35}
      />
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
