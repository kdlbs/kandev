'use client';

import { useCallback, useEffect, useRef, useState, memo } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useSessionMessages } from '@/hooks/domains/session/use-session-messages';
import { useSettingsData } from '@/hooks/domains/settings/use-settings-data';
import { useSessionState } from '@/hooks/domains/session/use-session-state';
import { useProcessedMessages } from '@/hooks/use-processed-messages';
import { useSessionModel } from '@/hooks/domains/session/use-session-model';
import { useMessageHandler } from '@/hooks/use-message-handler';
import { TodoSummary } from '@/components/task/chat/todo-summary';
import { VirtualizedMessageList } from '@/components/task/chat/virtualized-message-list';
import { ChatInputContainer } from '@/components/task/chat/chat-input-container';

type TaskChatPanelProps = {
  onSend?: (message: string) => void;
  sessionId?: string | null;
  onOpenFile?: (path: string) => void;
  showApproveButton?: boolean;
  onApprove?: () => void;
};

export const TaskChatPanel = memo(function TaskChatPanel({
  onSend,
  sessionId = null,
  onOpenFile,
  showApproveButton,
  onApprove,
}: TaskChatPanelProps) {
  const [planModeEnabled, setPlanModeEnabled] = useState(false);
  const [isSending, setIsSending] = useState(false);
  const lastAgentMessageCountRef = useRef(0);

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
  } = useSessionState(sessionId);

  // Fetch messages for this session
  const { messages, isLoading: messagesLoading } = useSessionMessages(resolvedSessionId);

  // Process messages (filtering, todos, etc.)
  const { allMessages, groupedItems, permissionsByToolCallId, childrenByParentToolCallId, todoItems, agentMessageCount, pendingClarification } = useProcessedMessages(
    messages,
    taskId,
    resolvedSessionId,
    taskDescription
  );

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

  // Message sending
  const { handleSendMessage } = useMessageHandler(
    resolvedSessionId,
    taskId,
    sessionModel,
    activeModel,
    planModeEnabled
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

  const handleSubmit = useCallback(async (message: string) => {
    if (isSending) return;
    setIsSending(true);
    try {
      if (onSend) {
        await onSend(message);
      } else {
        await handleSendMessage(message);
      }
      // Reset plan mode after sending - it's a per-message instruction
      if (planModeEnabled) {
        setPlanModeEnabled(false);
      }
    } finally {
      setIsSending(false);
    }
  }, [isSending, planModeEnabled, onSend, handleSendMessage]);


  return (
    <>
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

      <div className="flex flex-col gap-2 mt-2">
        {todoItems.length > 0 && <TodoSummary todos={todoItems} />}
        <ChatInputContainer
          key={clarificationKey}
          onSubmit={handleSubmit}
          sessionId={resolvedSessionId}
          taskId={taskId}
          taskTitle={task?.title}
          taskDescription={taskDescription ?? ''}
          planModeEnabled={planModeEnabled}
          onPlanModeChange={setPlanModeEnabled}
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
        />
      </div>
    </>
  );
});
