"use client";

import { memo, useCallback, useEffect, useId, useRef, useState, type RefObject } from "react";
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
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import { routePanelMouseDown } from "@/components/task/chat/route-panel-mouse-down";
import { useTranslation } from "react-i18next";

interface WorkspaceAgentChatProps {
  /** Workspace ID the conversation belongs to. */
  workspaceId: string;
  /** Stable conversation key (e.g. "coordinator"). */
  conversationKey: string;
  /** Resolved session ID for the managed conversation. */
  sessionId: string;
  /** Optional placeholder in the chat composer. */
  placeholderOverride?: string;
}

type ClarificationSectionProps = {
  pending: boolean;
  messages: readonly Message[] | null | undefined;
  onResolved: () => void;
  shortcutScopeRef: RefObject<HTMLElement | null>;
};

function ClarificationSection({
  pending,
  messages,
  onResolved,
  shortcutScopeRef,
}: ClarificationSectionProps) {
  const { t } = useTranslation();
  const [collapsed, setCollapsed] = useState(false);
  const contentId = useId();
  const { height, containerRef, resetHeight, resizeHandleProps } =
    useResizableClarificationOverlay();

  useEffect(() => {
    if (!pending) {
      setCollapsed(false);
      resetHeight();
    }
  }, [pending, resetHeight]);

  if (!pending) return null;

  const actionLabel = collapsed ? t("chat:expandClarification") : t("chat:collapseClarification");

  return (
    <div className="relative flex-shrink-0 border-t border-sky-400/30 bg-card">
      {!collapsed && <ResizeHandle {...resizeHandleProps} />}
      <div
        ref={containerRef}
        data-testid="agent-conv-clarification-container"
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
            <span className="truncate">{t("chat:clarificationNeeded")}</span>
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
            data-testid="agent-conv-clarification-toggle"
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
          data-testid="agent-conv-clarification-scroll-region"
          className={collapsed ? "hidden" : "min-h-0 flex-1 overflow-y-auto px-1"}
        >
          <ClarificationInputOverlay
            messages={messages}
            onResolved={onResolved}
            onDismiss={noop}
            shortcutScopeRef={shortcutScopeRef}
            keyboardShortcutsEnabled={!collapsed}
          />
        </div>
      </div>
    </div>
  );
}

function useAgentConversationState(sessionId: string) {
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  useSettingsData(true);
  const panelState = useChatPanelState({
    sessionId,
    onOpenFile: undefined,
    onOpenFileAtLine: undefined,
  });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, undefined);
  const { handleCancelTurn } = useChatPanelHandlers(panelState.resolvedSessionId, chatInputRef);
  return { chatInputRef, panelState, isSending, handleSubmit, handleCancelTurn };
}

/**
 * Host-owned workspace agent conversation chat component.
 *
 * Renders a managed conversation transcript with full send/queue/cancel
 * behavior, clarification overlay, streaming updates, and reconnection
 * tolerance. Plugins receive this through `host.ui.WorkspaceAgentChat`.
 *
 * The component is a hydration/error boundary around the same shared chat
 * primitives used by Quick Chat and task panels.
 */
const noop = () => {};

export const WorkspaceAgentChat = memo(function WorkspaceAgentChat({
  sessionId,
  placeholderOverride,
}: WorkspaceAgentChatProps) {
  const [clarificationKey, setClarificationKey] = useState(0);
  const shortcutScopeRef = useRef<HTMLDivElement>(null);
  const state = useAgentConversationState(sessionId);
  const { chatInputRef, panelState, isSending, handleSubmit, handleCancelTurn } = state;
  const { pendingClarification, pendingClarificationGroup } = panelState;

  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);
  const handleShortcutScopeMouseDown = useCallback(
    (event: React.MouseEvent<HTMLDivElement>) => routePanelMouseDown(event, shortcutScopeRef),
    [],
  );

  return (
    <div
      ref={shortcutScopeRef}
      data-testid="workspace-agent-chat"
      tabIndex={-1}
      onMouseDown={handleShortcutScopeMouseDown}
      className="flex flex-col h-full min-h-0 outline-none"
    >
      <div className="flex-1 min-h-0 overflow-hidden bg-popover" data-testid="agent-conv-messages">
        <MessageList
          items={panelState.groupedItems}
          messages={panelState.allMessages}
          permissionsByToolCallId={panelState.permissionsByToolCallId}
          childrenByParentToolCallId={panelState.childrenByParentToolCallId}
          taskId={panelState.taskId ?? undefined}
          sessionId={panelState.resolvedSessionId}
          messagesLoading={panelState.messagesLoading}
          isWorking={panelState.isWorking}
          sessionState={panelState.session?.state}
          worktreePath={getSessionWorkspacePath(panelState.session)}
          onOpenFile={undefined}
        />
      </div>
      <ClarificationSection
        pending={Boolean(pendingClarification)}
        messages={pendingClarificationGroup}
        onResolved={handleClarificationResolved}
        shortcutScopeRef={shortcutScopeRef}
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
        minimalToolbar={false}
        hidePlanMode={true}
        placeholderOverride={placeholderOverride}
        surfaceClassName="bg-popover"
      />
    </div>
  );
});
