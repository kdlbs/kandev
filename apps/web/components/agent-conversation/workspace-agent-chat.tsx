"use client";

import { memo, useCallback, useEffect, useId, useRef, useState, type RefObject } from "react";
import { IconChevronDown, IconChevronUp, IconMessageQuestion } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
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
import type { WorkspaceAgentChatProps, WorkspaceAgentChatStatus } from "@kandev/plugin-sdk";
import { useTranslation } from "react-i18next";

type ClarificationSectionProps = {
  pending: boolean;
  messages: readonly Message[] | null | undefined;
  onResolved: () => void;
  shortcutScopeRef: RefObject<HTMLElement | null>;
};

const noop = () => {};

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
    <div className="relative shrink-0 border-t border-sky-400/30 bg-card">
      {!collapsed && <ResizeHandle {...resizeHandleProps} />}
      <div
        ref={containerRef}
        className={
          collapsed
            ? "h-11"
            : "flex min-h-[7.5rem] max-h-[35vh] flex-col overflow-hidden overscroll-contain"
        }
        style={!collapsed && height !== null ? { height } : undefined}
      >
        <div className="flex h-11 shrink-0 items-center justify-between gap-2 pl-4">
          <div className="flex min-w-0 items-center gap-2 text-sm font-medium">
            <IconMessageQuestion className="h-4 w-4 shrink-0 text-blue-500" />
            <span className="truncate">{t("chat:clarificationNeeded")}</span>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-7 w-7 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11 cursor-pointer rounded-none"
            aria-label={actionLabel}
            aria-expanded={!collapsed}
            aria-controls={contentId}
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
            onDismiss={noop}
            shortcutScopeRef={shortcutScopeRef}
            keyboardShortcutsEnabled={!collapsed}
          />
        </div>
      </div>
    </div>
  );
}

function useManagedConversationLifecycle(
  taskWorkspaceId: string | undefined,
  workspaceId: string,
  hasSession: boolean,
  onStatus: WorkspaceAgentChatProps["onStatus"],
) {
  const hasResolvedConversation = useRef(false);
  let lifecycleStatus: WorkspaceAgentChatStatus;
  if (taskWorkspaceId === undefined) {
    lifecycleStatus = "loading";
  } else if (taskWorkspaceId !== workspaceId) {
    lifecycleStatus = "permission-denied";
  } else if (hasSession) {
    lifecycleStatus = "ready";
  } else {
    lifecycleStatus = hasResolvedConversation.current ? "deleted" : "unavailable";
  }

  useEffect(() => {
    if (hasSession) hasResolvedConversation.current = true;
    onStatus?.(lifecycleStatus);
  }, [hasSession, lifecycleStatus, onStatus]);

  return lifecycleStatus;
}

function ManagedConversationChat({
  workspaceId,
  conversationId,
  readOnly = false,
  placeholderOverride,
  onStatus,
}: Omit<WorkspaceAgentChatProps, "resourceVersion">) {
  const { t } = useTranslation();
  const [clarificationKey, setClarificationKey] = useState(0);
  const shortcutScopeRef = useRef<HTMLDivElement>(null);
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  useSettingsData(true);
  const panelState = useChatPanelState({
    sessionId: conversationId,
    onOpenFile: undefined,
    onOpenFileAtLine: undefined,
    disableWorkbenchEffects: true,
  });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, undefined);
  const { handleCancelTurn } = useChatPanelHandlers(panelState.resolvedSessionId, chatInputRef);
  const lifecycleStatus = useManagedConversationLifecycle(
    panelState.task?.workspaceId,
    workspaceId,
    Boolean(panelState.session),
    onStatus,
  );

  const handleClarificationResolved = useCallback(() => setClarificationKey((key) => key + 1), []);
  const handleShortcutScopeMouseDown = useCallback(
    (event: React.MouseEvent<HTMLDivElement>) => routePanelMouseDown(event, shortcutScopeRef),
    [],
  );

  if (lifecycleStatus !== "ready") {
    return (
      <div
        data-testid="workspace-agent-chat-status"
        data-status={lifecycleStatus}
        className="flex h-full min-h-0 items-center justify-center p-4"
      >
        <Spinner aria-label={t("plugins:loadingWorkspaceAgentChat")} />
      </div>
    );
  }

  return (
    <div
      ref={shortcutScopeRef}
      data-testid="workspace-agent-chat"
      data-workspace-id={workspaceId}
      tabIndex={-1}
      onMouseDown={handleShortcutScopeMouseDown}
      className="flex h-full min-h-0 flex-col outline-none"
    >
      <div className="min-h-0 flex-1 overflow-hidden bg-popover">
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
        pending={Boolean(panelState.pendingClarification)}
        messages={panelState.pendingClarificationGroup}
        onResolved={handleClarificationResolved}
        shortcutScopeRef={shortcutScopeRef}
      />
      {!readOnly && (
        <ChatInputArea
          chatInputRef={chatInputRef}
          clarificationKey={clarificationKey}
          onClarificationResolved={handleClarificationResolved}
          handleSubmit={handleSubmit}
          handleCancelTurn={handleCancelTurn}
          showRequestChangesTooltip={false}
          panelState={panelState}
          isSending={isSending}
          hideSessionsDropdown
          hideAgentControls
          hidePlanMode
          placeholderOverride={placeholderOverride}
          surfaceClassName="bg-popover"
        />
      )}
    </div>
  );
}

export const WorkspaceAgentChat = memo(function WorkspaceAgentChat({
  resourceVersion,
  ...props
}: WorkspaceAgentChatProps) {
  return <ManagedConversationChat key={resourceVersion} {...props} />;
});
