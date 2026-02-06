'use client';

import { useCallback, useEffect, useRef, useState, memo, useMemo } from 'react';
import { cn } from '@/lib/utils';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore } from '@/components/state-provider';
import { getLocalStorage } from '@/lib/local-storage';
import { useKeyboardShortcut } from '@/hooks/use-keyboard-shortcut';
import { SHORTCUTS } from '@/lib/keyboard/constants';
import { useSessionMessages } from '@/hooks/domains/session/use-session-messages';
import { useSettingsData } from '@/hooks/domains/settings/use-settings-data';
import { useSessionState } from '@/hooks/domains/session/use-session-state';
import { useProcessedMessages } from '@/hooks/use-processed-messages';
import { useSessionModel } from '@/hooks/domains/session/use-session-model';
import { useMessageHandler } from '@/hooks/use-message-handler';
import { useQueue } from '@/hooks/domains/session/use-queue';
import { TodoSummary } from '@/components/task/chat/todo-summary';
import { VirtualizedMessageList } from '@/components/task/chat/virtualized-message-list';
import { ChatInputContainer, type ChatInputContainerHandle, type MessageAttachment } from '@/components/task/chat/chat-input-container';
import { QueuedMessageIndicator, type QueuedMessageIndicatorHandle } from '@/components/task/chat/queued-message-indicator';
import { formatReviewCommentsAsMarkdown } from '@/components/task/chat/messages/review-comments-attachment';
import {
  useDiffCommentsStore,
  usePendingCommentsByFile,
} from '@/lib/state/slices/diff-comments';
import type { DiffComment } from '@/lib/diff/types';

type TaskChatPanelProps = {
  onSend?: (message: string) => void;
  sessionId?: string | null;
  onOpenFile?: (path: string) => void;
  showRequestChangesTooltip?: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  /** Callback to open a file at a specific line (for comment clicks) */
  onOpenFileAtLine?: (filePath: string) => void;
};

export const TaskChatPanel = memo(function TaskChatPanel({
  onSend,
  sessionId = null,
  onOpenFile,
  showRequestChangesTooltip = false,
  onRequestChangesTooltipDismiss,
  onOpenFileAtLine,
}: TaskChatPanelProps) {
  const [isSending, setIsSending] = useState(false);
  const lastAgentMessageCountRef = useRef(0);
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  const queuedMessageRef = useRef<QueuedMessageIndicatorHandle>(null);

  // Ensure agent profile data is loaded (may not be hydrated from SSR in all navigation paths)
  useSettingsData(true);

  // Session state management
  const {
    resolvedSessionId,
    session,
    task,
    taskId,
    taskDescription,
    isStarting,
    isWorking,
    isAgentBusy,
    isFailed,
  } = useSessionState(sessionId);

  // Plan mode state from store (persisted per session)
  const planModeEnabled = useAppStore((state) =>
    resolvedSessionId ? (state.chatInput.planModeBySessionId[resolvedSessionId] ?? false) : false
  );
  const setPlanMode = useAppStore((state) => state.setPlanMode);
  const handlePlanModeChange = useCallback(
    (enabled: boolean) => {
      if (resolvedSessionId) {
        setPlanMode(resolvedSessionId, enabled);
      }
    },
    [resolvedSessionId, setPlanMode]
  );

  // Initialize plan mode from localStorage on mount
  useEffect(() => {
    if (resolvedSessionId) {
      const stored = getLocalStorage(`plan-mode-${resolvedSessionId}`, false);
      if (stored) {
        setPlanMode(resolvedSessionId, stored);
      }
    }
  }, [resolvedSessionId, setPlanMode]);

  // Fetch messages for this session
  const { messages, isLoading: messagesLoading } = useSessionMessages(resolvedSessionId);

  // Process messages (filtering, todos, etc.)
  const { allMessages, groupedItems, permissionsByToolCallId, childrenByParentToolCallId, todoItems, agentMessageCount, pendingClarification } = useProcessedMessages(
    messages,
    taskId,
    resolvedSessionId,
    taskDescription
  );

  // Extract user message history for up/down arrow navigation
  const userMessageHistory = useMemo(() => {
    return allMessages
      .filter(msg => msg.author_type === 'user')
      .map(msg => msg.content)
      .filter(content => content && content.trim().length > 0);
  }, [allMessages]);

  // Track clarification resolved state to trigger re-render after submit
  const [clarificationKey, setClarificationKey] = useState(0);
  const handleClarificationResolved = useCallback(() => {
    // Force re-render which will update messages from backend
    setClarificationKey((k) => k + 1);
  }, []);

  // Model management
  const { sessionModel, activeModel } = useSessionModel(
    resolvedSessionId,
    session?.agent_profile_id
  );

  // User settings
  const chatSubmitKey = useAppStore((state) => state.userSettings.chatSubmitKey);
  const agentCommands = useAppStore((state) =>
    resolvedSessionId ? state.availableCommands.bySessionId[resolvedSessionId] : undefined
  );
  const hasAgentCommands = agentCommands && agentCommands.length > 0;

  // Message queue
  const { queuedMessage, isQueued, cancel: cancelQueue, updateContent: updateQueueContent } = useQueue(resolvedSessionId);

  // Message sending
  const { handleSendMessage } = useMessageHandler(
    resolvedSessionId,
    taskId,
    sessionModel,
    activeModel,
    planModeEnabled,
    isAgentBusy
  );

  // Diff comments management
  const pendingCommentsByFile = usePendingCommentsByFile();
  const markCommentsSent = useDiffCommentsStore((state) => state.markCommentsSent);
  const removeComment = useDiffCommentsStore((state) => state.removeComment);

  // Handle removing all comments for a file
  const handleRemoveCommentFile = useCallback(
    (filePath: string) => {
      if (!resolvedSessionId) return;
      const comments = pendingCommentsByFile[filePath] || [];
      for (const comment of comments) {
        removeComment(resolvedSessionId, filePath, comment.id);
      }
    },
    [resolvedSessionId, pendingCommentsByFile, removeComment]
  );

  // Handle removing a single comment
  const handleRemoveComment = useCallback(
    (sessionId: string, filePath: string, commentId: string) => {
      removeComment(sessionId, filePath, commentId);
    },
    [removeComment]
  );

  // Clear awaiting state when a new agent message arrives
  useEffect(() => {
    lastAgentMessageCountRef.current = agentMessageCount;
  }, [agentMessageCount]);

  // Cancels the current agent turn without terminating the agent process,
  // allowing the user to interrupt and send a new prompt.
  const handleCancelTurn = useCallback(async () => {
    if (!resolvedSessionId) return;
    const client = getWebSocketClient();
    if (!client) return;

    try {
      await client.request('agent.cancel', { session_id: resolvedSessionId }, 15000);
    } catch (error) {
      console.error('Failed to cancel agent turn:', error);
    }
  }, [resolvedSessionId]);

  // Handle canceling queued message
  const handleCancelQueue = useCallback(async () => {
    try {
      await cancelQueue();
    } catch (error) {
      console.error('Failed to cancel queued message:', error);
    }
  }, [cancelQueue]);

  // Handle starting edit mode on queued message (from keyboard navigation)
  const handleStartQueueEdit = useCallback(() => {
    queuedMessageRef.current?.startEdit();
  }, []);

  // Handle edit complete - return focus to chat input
  const handleQueueEditComplete = useCallback(() => {
    chatInputRef.current?.focusInput();
  }, []);

  const handleSubmit = useCallback(async (message: string, reviewComments?: DiffComment[], attachments?: MessageAttachment[]) => {
    if (isSending) return;
    setIsSending(true);
    try {
      // Build final message with review comments
      let finalMessage = message;
      if (reviewComments && reviewComments.length > 0) {
        const reviewMarkdown = formatReviewCommentsAsMarkdown(reviewComments);
        finalMessage = reviewMarkdown + (message ? message : '');
      }

      if (onSend) {
        await onSend(finalMessage);
      } else {
        await handleSendMessage(finalMessage, attachments);
      }

      // Mark comments as sent and clear pending
      if (reviewComments && reviewComments.length > 0) {
        markCommentsSent(reviewComments.map((c) => c.id));
      }
    } finally {
      setIsSending(false);
    }
  }, [isSending, onSend, handleSendMessage, markCommentsSent]);

  // Focus input with / shortcut (only when input is NOT focused)
  useKeyboardShortcut(
    SHORTCUTS.FOCUS_INPUT,
    useCallback((event: KeyboardEvent) => {
      // Only handle if we're not already in an input/textarea
      const activeElement = document.activeElement;
      const isTyping =
        activeElement instanceof HTMLInputElement ||
        activeElement instanceof HTMLTextAreaElement;

      if (isTyping) {
        // Don't intercept - let the "/" be typed normally for slash commands
        return;
      }

      // Not typing anywhere, so focus the chat input
      const inputHandle = chatInputRef.current;
      if (inputHandle) {
        event.preventDefault(); // Prevent the "/" from being typed when we focus
        inputHandle.focusInput();
      }
    }, []),
    { enabled: true, preventDefault: false }
  );

  return (
    <div className="flex flex-col h-full min-h-0">
      {/* Scrollable messages area */}
      <div className="flex-1 min-h-0 overflow-auto">
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
      </div>

      {/* Sticky input at bottom */}
      <div className="flex-shrink-0 flex flex-col gap-2 mt-2">
        {/* Status section: todos, etc. */}
        {todoItems.length > 0 && <TodoSummary todos={todoItems} />}

        {/* Chat input with optional queued message on top */}
        <div className="flex flex-col">
          {/* Queued message indicator - appears above chat input */}
          {isQueued && queuedMessage && (
            <div className={cn(
              'border border-b-0 border-border bg-background shadow-md',
              'rounded-t-2xl overflow-hidden',
              'border-blue-500/30'
            )}>
              <div className="px-3 py-3">
                <QueuedMessageIndicator
                  ref={queuedMessageRef}
                  content={queuedMessage.content}
                  onCancel={handleCancelQueue}
                  onUpdate={updateQueueContent}
                  isVisible={true}
                  onEditComplete={handleQueueEditComplete}
                />
              </div>
            </div>
          )}

          <ChatInputContainer
          ref={chatInputRef}
          key={clarificationKey}
          onSubmit={handleSubmit}
          sessionId={resolvedSessionId}
          taskId={taskId}
          taskTitle={task?.title}
          taskDescription={taskDescription ?? ''}
          planModeEnabled={planModeEnabled}
          onPlanModeChange={handlePlanModeChange}
          isAgentBusy={isAgentBusy}
          isStarting={isStarting}
          isSending={isSending}
          onCancel={handleCancelTurn}
          placeholder={
            agentMessageCount > 0
              ? 'Continue working on this task...'
              : 'Write to submit work to the agent...'
          }
          pendingClarification={pendingClarification}
          onClarificationResolved={handleClarificationResolved}
          showRequestChangesTooltip={showRequestChangesTooltip}
          onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
          pendingCommentsByFile={pendingCommentsByFile}
          onRemoveCommentFile={handleRemoveCommentFile}
          onRemoveComment={handleRemoveComment}
          onCommentClick={onOpenFileAtLine ? (comment) => onOpenFileAtLine(comment.filePath) : undefined}
          submitKey={chatSubmitKey}
          hasAgentCommands={hasAgentCommands}
          isFailed={isFailed}
          isQueued={isQueued}
          onStartQueueEdit={handleStartQueueEdit}
          userMessageHistory={userMessageHistory}
          hasQueuedMessageAbove={isQueued}
        />
        </div>
      </div>
    </div>
  );
});
