"use client";

import { useCallback, useEffect, useRef, useState, memo } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { type ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";
import { type QueuedMessageIndicatorHandle } from "@/components/task/chat/queued-message-indicator";
import { VirtualizedMessageList } from "@/components/task/chat/virtualized-message-list";
import { useIsTaskArchived } from "./task-archived-context";
import { useChatPanelState } from "./chat/use-chat-panel-state";
import { ChatInputArea, useSubmitHandler, useChatPanelHandlers } from "./chat/chat-input-area";
import { ClarificationInputOverlay } from "./chat/clarification-input-overlay";

type TaskChatPanelProps = {
  onSend?: (message: string) => void;
  sessionId?: string | null;
  onOpenFile?: (path: string) => void;
  showRequestChangesTooltip?: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  /** Callback to open a file at a specific line (for comment clicks) */
  onOpenFileAtLine?: (filePath: string) => void;
  /** Whether the dockview group containing this panel is focused */
  isPanelFocused?: boolean;
};

export const TaskChatPanel = memo(function TaskChatPanel({
  onSend,
  sessionId = null,
  onOpenFile,
  showRequestChangesTooltip = false,
  onRequestChangesTooltipDismiss,
  onOpenFileAtLine,
  isPanelFocused,
}: TaskChatPanelProps) {
  const isArchived = useIsTaskArchived();
  const lastAgentMessageCountRef = useRef(0);
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  const queuedMessageRef = useRef<QueuedMessageIndicatorHandle>(null);
  const [clarificationKey, setClarificationKey] = useState(0);

  useSettingsData(true);
  const panelState = useChatPanelState({ sessionId, onOpenFile, onOpenFileAtLine });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, onSend);
  const {
    resolvedSessionId,
    session,
    taskId,
    isWorking,
    messagesLoading,
    groupedItems,
    allMessages,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    agentMessageCount,
    cancelQueue,
    pendingClarification,
  } = panelState;
  const { handleCancelTurn, handleCancelQueue, handleQueueEditComplete } = useChatPanelHandlers(
    resolvedSessionId,
    cancelQueue,
    chatInputRef,
  );

  useEffect(() => {
    lastAgentMessageCountRef.current = agentMessageCount;
  }, [agentMessageCount]);

  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);

  return (
    <PanelRoot>
      <PanelBody padding={false}>
        <VirtualizedMessageList
          items={groupedItems}
          messages={allMessages}
          permissionsByToolCallId={permissionsByToolCallId}
          childrenByParentToolCallId={childrenByParentToolCallId}
          taskId={taskId ?? undefined}
          sessionId={resolvedSessionId}
          messagesLoading={messagesLoading}
          isWorking={isWorking}
          sessionState={session?.state}
          worktreePath={session?.worktree_path}
          onOpenFile={onOpenFile}
        />
      </PanelBody>
      {pendingClarification && !isArchived && (
        <div className="flex-shrink-0 border-t border-sky-400/30 bg-card px-1">
          <ClarificationInputOverlay
            message={pendingClarification}
            onResolved={handleClarificationResolved}
          />
        </div>
      )}
      {isArchived ? (
        <div className="bg-muted/50 flex-shrink-0 px-4 py-3 text-center text-sm text-muted-foreground border-t">
          This task is archived and read-only.
        </div>
      ) : (
        <ChatInputArea
          chatInputRef={chatInputRef}
          queuedMessageRef={queuedMessageRef}
          clarificationKey={clarificationKey}
          onClarificationResolved={handleClarificationResolved}
          handleSubmit={handleSubmit}
          handleCancelTurn={handleCancelTurn}
          handleCancelQueue={handleCancelQueue}
          handleQueueEditComplete={handleQueueEditComplete}
          showRequestChangesTooltip={showRequestChangesTooltip}
          onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
          isPanelFocused={isPanelFocused}
          panelState={panelState}
          isSending={isSending}
        />
      )}
    </PanelRoot>
  );
});
