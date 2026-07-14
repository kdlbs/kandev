"use client";

import { memo, useCallback, useEffect, useId, useRef, useState } from "react";
import { IconChevronDown, IconChevronUp, IconMessageQuestion } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
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
import type { Message } from "@/lib/types/http";

type QuickChatContentProps = {
  sessionId: string;
  minimalToolbar?: boolean;
  placeholderOverride?: string;
  initialPrompt?: string;
  onInitialPromptSent?: () => void;
};

type QuickChatClarificationSectionProps = {
  pending: boolean;
  messages: readonly Message[] | null | undefined;
  onResolved: () => void;
};

function QuickChatClarificationSection({
  pending,
  messages,
  onResolved,
}: QuickChatClarificationSectionProps) {
  const [collapsed, setCollapsed] = useState(false);
  const contentId = useId();
  const { height, containerRef, resetHeight, resizeHandleProps } =
    useResizableClarificationOverlay();

  // A newly opened clarification starts expanded and auto-sized. Collapsing
  // an active one leaves its form state and user-selected height intact.
  useEffect(() => {
    if (!pending) {
      setCollapsed(false);
      resetHeight();
    }
  }, [pending, resetHeight]);

  if (!pending) return null;

  const actionLabel = collapsed ? "Expand clarification" : "Collapse clarification";

  return (
    <div className="relative flex-shrink-0 border-t border-sky-400/30 bg-card">
      {!collapsed && <ResizeHandle {...resizeHandleProps} />}
      <div
        ref={containerRef}
        data-testid="clarification-overlay-container"
        className={
          collapsed
            ? "h-11"
            : "flex min-h-[7.5rem] max-h-[35vh] flex-col overflow-hidden overscroll-contain"
        }
        style={!collapsed && height !== null ? { height } : undefined}
      >
        <div className="flex h-11 flex-shrink-0 items-center justify-between gap-2 pl-4">
          <div className="flex min-w-0 items-center gap-2 text-sm font-medium">
            <IconMessageQuestion className="h-4 w-4 flex-shrink-0 text-blue-500" />
            <span className="truncate">Clarification needed</span>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-11 w-11 flex-shrink-0 cursor-pointer rounded-none"
            aria-label={actionLabel}
            aria-expanded={!collapsed}
            aria-controls={contentId}
            title={actionLabel}
            data-testid="clarification-collapse-toggle"
            onClick={() => setCollapsed((current) => !current)}
          >
            {collapsed ? (
              <IconChevronUp className="h-4 w-4" />
            ) : (
              <IconChevronDown className="h-4 w-4" />
            )}
          </Button>
        </div>
        <div
          id={contentId}
          className={collapsed ? "hidden" : "min-h-0 flex-1 overflow-y-auto px-1"}
        >
          <ClarificationInputOverlay
            messages={messages}
            onResolved={onResolved}
            keyboardShortcutsEnabled={!collapsed}
          />
        </div>
      </div>
    </div>
  );
}

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
    handleSubmit(initialPrompt);
    onInitialPromptSent?.();
  }, [initialPrompt, taskId, handleSubmit, onInitialPromptSent, sessionId]);

  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);

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
      <QuickChatClarificationSection
        pending={Boolean(pendingClarification)}
        messages={pendingClarificationGroup}
        onResolved={handleClarificationResolved}
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
