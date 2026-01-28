'use client';

import { FormEvent, useCallback, useEffect, useRef, useState } from 'react';
import { flushSync } from 'react-dom';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore } from '@/components/state-provider';
import { useSessionMessages } from '@/hooks/domains/session/use-session-messages';
import { useSettingsData } from '@/hooks/domains/settings/use-settings-data';
import { useSessionState } from '@/hooks/domains/session/use-session-state';
import { useProcessedMessages } from '@/hooks/use-processed-messages';
import { useSessionModel } from '@/hooks/domains/session/use-session-model';
import { useMessageHandler } from '@/hooks/use-message-handler';
import { TaskChatInput } from '@/components/task/task-chat-input';
import { RunningIndicator } from '@/components/task/chat/messages/running-indicator';
import { TodoSummary } from '@/components/task/chat/todo-summary';
import { VirtualizedMessageList } from '@/components/task/chat/virtualized-message-list';
import { ChatToolbar } from '@/components/task/chat/chat-toolbar';
import { approveSessionAction } from '@/app/actions/workspaces';

type TaskChatPanelProps = {
  onSend?: (message: string) => void;
  sessionId?: string | null;
};

export function TaskChatPanel({
  onSend,
  sessionId = null,
}: TaskChatPanelProps) {
  const [messageInput, setMessageInput] = useState('');
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
  const { allMessages, permissionsByToolCallId, todoItems, agentMessageCount } = useProcessedMessages(
    messages,
    taskId,
    resolvedSessionId,
    taskDescription
  );

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

  // Get store action for updating session
  const setTaskSession = useAppStore((state) => state.setTaskSession);

  // Review panel handlers
  const handleApprove = useCallback(async () => {
    if (!resolvedSessionId || !taskId) return;
    try {
      const response = await approveSessionAction(resolvedSessionId);

      // Update the session in the store so the review panel closes
      if (response?.session) {
        setTaskSession(response.session);
      }

      // Check if the new step has auto_start_agent enabled
      // Use fire-and-forget (send) instead of request since resuming session + sending prompt
      // can take a long time. Session state updates will come via WebSocket events anyway.
      if (response?.workflow_step?.auto_start_agent) {
        const client = getWebSocketClient();
        if (client) {
          client.send({
            type: 'request',
            action: 'orchestrator.start',
            payload: {
              task_id: taskId,
              session_id: resolvedSessionId,
              workflow_step_id: response.workflow_step.id,
            },
          });
        }
      }
    } catch (error) {
      console.error('Failed to approve session:', error);
    }
  }, [resolvedSessionId, taskId, setTaskSession]);



  // Check if we should show the approve button
  // Show when session has a review_status that is not approved (meaning it's in a review step)
  // Also hide while agent is working to prevent premature approval
  const showApproveButton =
    session?.review_status != null && session.review_status !== 'approved' && !isWorking;

  const handleSubmit = async (event?: FormEvent) => {
    event?.preventDefault();
    const trimmed = messageInput.trim();
    if (!trimmed || isSending) return;
    setIsSending(true);
    flushSync(() => {
      setMessageInput('');
    });
    try {
      if (onSend) {
        await onSend(trimmed);
      } else {
        await handleSendMessage(trimmed);
      }
      // Reset plan mode after sending - it's a per-message instruction
      if (planModeEnabled) {
        setPlanModeEnabled(false);
      }
    } finally {
      setIsSending(false);
    }
  };


  return (
    <>
      <VirtualizedMessageList
        messages={allMessages}
        permissionsByToolCallId={permissionsByToolCallId}
        taskId={taskId ?? undefined}
        sessionId={resolvedSessionId}
        messagesLoading={messagesLoading}
        isWorking={isWorking}
      />

      {/* Session info - shows agent state */}
      {session?.state && (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <RunningIndicator state={session.state} sessionId={resolvedSessionId} />
        </div>
      )}

      <form onSubmit={handleSubmit} className="flex flex-col gap-2">
        {todoItems.length > 0 && <TodoSummary todos={todoItems} />}
        <TaskChatInput
          value={messageInput}
          onChange={setMessageInput}
          onSubmit={() => handleSubmit()}
          placeholder={
            agentMessageCount > 0
              ? 'Continue working on this task...'
              : 'Write to submit work to the agent...'
          }
          planModeEnabled={planModeEnabled}
          sessionId={resolvedSessionId}
        />
        <ChatToolbar
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
          showApproveButton={showApproveButton}
          onApprove={handleApprove}
        />
      </form>
    </>
  );
}
