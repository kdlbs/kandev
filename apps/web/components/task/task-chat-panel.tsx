'use client';

import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { flushSync } from 'react-dom';
import { useVirtualizer } from '@tanstack/react-virtual';
import { IconBrain, IconListCheck, IconLoader2, IconPaperclip } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';

import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn } from '@/lib/utils';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Message } from '@/lib/types/http';
import { SHORTCUTS } from '@/lib/keyboard/constants';
import { KeyboardShortcutTooltip } from '@/components/keyboard-shortcut-tooltip';
import { TaskChatInput } from '@/components/task/task-chat-input';
import { useLazyLoadMessages } from '@/hooks/use-lazy-load-messages';
import { useSession } from '@/hooks/use-session';
import { useTask } from '@/hooks/use-task';
import { useSessionMessages } from '@/hooks/use-session-messages';
import { useSettingsData } from '@/hooks/use-settings-data';
import { MessageRenderer } from '@/components/task/chat/message-renderer';
import { RunningIndicator } from '@/components/task/chat/messages/running-indicator';
import { TokenUsageDisplay } from '@/components/task/chat/token-usage-display';
import { TodoSummary } from '@/components/task/chat/todo-summary';
import { SessionsDropdown } from '@/components/task/sessions-dropdown';
import { useSessionContextWindow } from '@/hooks/use-session-context-window';
import { ModelSelector } from '@/components/task/model-selector';

type TaskChatPanelProps = {
  /** @deprecated No longer used - model selection is now handled by ModelSelector */
  agents?: { id: string; label: string }[];
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
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const lastAgentMessageCountRef = useRef(0);
  const wasAtBottomRef = useRef(true);

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const resolvedSessionId = sessionId ?? activeSessionId;

  // Get task session details and derived task information from session ID
  const { session } = useSession(resolvedSessionId);
  const task = useTask(session?.task_id ?? null);
  const taskId = session?.task_id ?? null;
  const taskDescription = task?.description ?? null;
  const isStarting = session?.state === 'STARTING';
  const isWorking = isStarting || session?.state === 'RUNNING';
  const isAgentBusy = session?.state === 'CREATED' || session?.state === 'RUNNING';

  // Ensure agent profile data is loaded (may not be hydrated from SSR in all navigation paths)
  useSettingsData(true);

  // Get model from agent profile using agent_profile_id
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const sessionProfile = useMemo(() => {
    if (!session?.agent_profile_id) return null;
    for (const agent of settingsAgents) {
      const profile = agent.profiles.find((p) => p.id === session.agent_profile_id);
      if (profile) return profile;
    }
    return null;
  }, [session?.agent_profile_id, settingsAgents]);

  const sessionModel = sessionProfile?.model ?? null;

  // Get pending model state for this session
  const pendingModels = useAppStore((state) => state.pendingModel.bySessionId);
  const clearPendingModel = useAppStore((state) => state.clearPendingModel);
  const setActiveModel = useAppStore((state) => state.setActiveModel);
  const pendingModel = resolvedSessionId ? pendingModels[resolvedSessionId] : null;

  // Fetch context window usage for this session
  const contextWindow = useSessionContextWindow(resolvedSessionId);

  // Fetch messages for this session
  const { messages, isLoading: messagesLoading } = useSessionMessages(resolvedSessionId);
  const isInitialLoading = messagesLoading && messages.length === 0;
  const showLoadingState = (messagesLoading || isInitialLoading) && !isWorking;
  const { loadMore, hasMore, isLoading: isLoadingMore } = useLazyLoadMessages(resolvedSessionId);

  // Build a set of tool_call_ids that exist in the messages
  const toolCallIds = useMemo(() => {
    const set = new Set<string>();
    for (const message of messages) {
      if (message.type === 'tool_call') {
        const toolCallId = (message.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
        if (toolCallId) {
          set.add(toolCallId);
        }
      }
    }
    return set;
  }, [messages]);

  // Build a lookup map of tool_call_id → permission_request message
  const permissionsByToolCallId = useMemo(() => {
    const map = new Map<string, Message>();
    for (const message of messages) {
      if (message.type === 'permission_request') {
        const toolCallId = (message.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
        if (toolCallId) {
          map.set(toolCallId, message);
        }
      }
    }
    return map;
  }, [messages]);

  // Filter to only show chat-relevant types.
  // Permission requests are only hidden if they have a matching tool_call in the messages
  // (so they can be displayed inline). Standalone permissions are shown as separate messages.
  const visibleMessages = messages.filter((message) => {
    // Standard visible types
    if (
      !message.type ||
      message.type === 'message' ||
      message.type === 'content' ||
      message.type === 'tool_call' ||
      message.type === 'progress' ||
      message.type === 'status' ||
      message.type === 'error' ||
      message.type === 'thinking' ||
      message.type === 'todo'
    ) {
      return true;
    }

    // Permission requests: show if no matching tool_call exists (standalone permission)
    if (message.type === 'permission_request') {
      const toolCallId = (message.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
      // Show as standalone if no tool_call_id or no matching tool_call message exists
      return !toolCallId || !toolCallIds.has(toolCallId);
    }

    return false;
  });

  // Create a synthetic "user" message for the task description
  const taskDescriptionMessage: Message | null =
    taskDescription && visibleMessages.length === 0
      ? {
        id: 'task-description',
        task_id: taskId ?? '',
        session_id: resolvedSessionId ?? '',
        author_type: 'user',
        content: taskDescription,
        type: 'message',
        created_at: '',
      }
      : null;

  // Combine task description with visible messages
  const allMessages = taskDescriptionMessage
    ? [taskDescriptionMessage, ...visibleMessages]
    : visibleMessages;

  const latestTodos = [...visibleMessages]
    .reverse()
    .find((message) => message.type === 'todo' || (message.metadata as { todos?: unknown })?.todos);

  const todoItems = (latestTodos?.metadata as { todos?: Array<{ text: string; done?: boolean } | string> } | undefined)
    ?.todos
    ?.map((item) => (typeof item === 'string' ? { text: item, done: false } : item))
    .filter((item) => item.text) ?? [];

  // Count agent messages to detect new responses
  const agentMessageCount = visibleMessages.filter((c) => c.author_type !== 'user').length;

  // Clear awaiting state when a new agent message arrives
  useEffect(() => {
    lastAgentMessageCountRef.current = agentMessageCount;
  }, [agentMessageCount]);

  const itemCount = allMessages.length;

  const virtualizer = useVirtualizer({
    count: itemCount,
    getScrollElement: () => messagesContainerRef.current,
    estimateSize: () => 96,
    overscan: 6,
  });

  const checkAtBottom = useCallback(() => {
    const element = messagesContainerRef.current;
    if (!element) return;
    const { scrollTop, scrollHeight, clientHeight } = element;
    wasAtBottomRef.current = scrollHeight - scrollTop - clientHeight < 48;
  }, []);

  useEffect(() => {
    const element = messagesContainerRef.current;
    if (!element) return;
    element.addEventListener('scroll', checkAtBottom);
    return () => element.removeEventListener('scroll', checkAtBottom);
  }, [checkAtBottom]);

  // Scroll to bottom when new messages arrive or when typing indicator appears
  useEffect(() => {
    if (itemCount === 0) return;
    if (wasAtBottomRef.current) {
      virtualizer.scrollToIndex(itemCount - 1, { align: 'end' });
    }
  }, [itemCount, virtualizer]);

  const virtualItems = virtualizer.getVirtualItems();

  useEffect(() => {
    const [firstItem] = virtualItems;
    if (!firstItem) return;
    const element = messagesContainerRef.current;
    if (!element) return;
    if (firstItem.index !== 0 || element.scrollTop > 40) {
      return;
    }
    if (!hasMore || isLoadingMore) {
      return;
    }
    const prevScrollHeight = element.scrollHeight;
    const prevScrollTop = element.scrollTop;
    loadMore().then((added) => {
      if (!added) return;
      requestAnimationFrame(() => {
        const nextScrollHeight = element.scrollHeight;
        element.scrollTop = prevScrollTop + (nextScrollHeight - prevScrollHeight);
      });
    });
  }, [virtualItems, hasMore, isLoadingMore, loadMore]);

  const handleSendMessage = useCallback(async (message: string) => {
    if (!taskId || !resolvedSessionId) {
      console.error('No active task session. Start an agent before sending a message.');
      return;
    }
    const client = getWebSocketClient();
    if (!client) return;

    // Include pending model in the request if one is selected
    const modelToSend = pendingModel && pendingModel !== sessionModel ? pendingModel : undefined;

    await client.request(
      'message.add',
      {
        task_id: taskId,
        session_id: resolvedSessionId,
        content: message,
        ...(modelToSend && { model: modelToSend }),
      },
      10000
    );

    // Move pending model to active model after sending (it's now the model in use)
    if (modelToSend && resolvedSessionId) {
      setActiveModel(resolvedSessionId, modelToSend);
      clearPendingModel(resolvedSessionId);
    }
  }, [resolvedSessionId, taskId, pendingModel, sessionModel, setActiveModel, clearPendingModel]);

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
    } finally {
      setIsSending(false);
    }
  };


  return (
    <>
      <div
        ref={messagesContainerRef}
        className="relative flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3"
      >
        {isLoadingMore && hasMore && (
          <div className="absolute top-2 left-1/2 -translate-x-1/2 text-xs text-muted-foreground">
            Loading older messages...
          </div>
        )}
        {/* Show loading messages spinner when initially loading */}
        {showLoadingState && (
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            <IconLoader2 className="h-5 w-5 animate-spin mr-2" />
            <span>Loading messages...</span>
          </div>
        )}
        {/* Show empty state when no messages and no loading */}
        {!messagesLoading && !isInitialLoading && visibleMessages.length === 0 && (
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            <span>No messages yet. Start the conversation!</span>
          </div>
        )}
        {/* Render messages */}
        {!isInitialLoading && itemCount > 0 && (
          <div className="relative w-full" style={{ height: `${virtualizer.getTotalSize()}px` }}>
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const message = allMessages[virtualRow.index];
              return (
                <div
                  key={virtualRow.key}
                  ref={virtualizer.measureElement}
                  data-index={virtualRow.index}
                  className="absolute left-0 top-0 w-full"
                  style={{ transform: `translateY(${virtualRow.start}px)` }}
                >
                  <div className="pb-3">
                    <MessageRenderer
                      comment={message}
                      isTaskDescription={message.id === 'task-description'}
                      taskId={taskId ?? undefined}
                      permissionsByToolCallId={permissionsByToolCallId}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Session info - shows agent state */}
      {session?.state && (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <RunningIndicator state={session.state} />
        </div>
      )}

      <form onSubmit={handleSubmit} className="mt-3 flex flex-col gap-2">
        {todoItems.length > 0 && (
          <TodoSummary todos={todoItems} />
        )}
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
        />
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <ModelSelector sessionId={resolvedSessionId} />
            <DropdownMenu>
              <Tooltip>
                <TooltipTrigger asChild>
                  <DropdownMenuTrigger asChild>
                    <Button type="button" variant="outline" size="icon" className="h-7 w-7 cursor-pointer">
                      <IconBrain className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                </TooltipTrigger>
                <TooltipContent>Thinking level</TooltipContent>
              </Tooltip>
              <DropdownMenuContent align="start" side="top">
                <DropdownMenuItem>High</DropdownMenuItem>
                <DropdownMenuItem>Medium</DropdownMenuItem>
                <DropdownMenuItem>Low</DropdownMenuItem>
                <DropdownMenuItem>Off</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    className={cn(
                      'h-7 w-7 cursor-pointer',
                      planModeEnabled &&
                      'bg-primary/15 text-primary border-primary/40 shadow-[0_0_0_1px_rgba(59,130,246,0.35)]'
                    )}
                    onClick={() => setPlanModeEnabled((value) => !value)}
                  >
                    <IconListCheck className="h-4 w-4" />
                  </Button>
                  {planModeEnabled && (
                    <span className="text-xs font-medium text-primary">Plan mode active</span>
                  )}
                </div>
              </TooltipTrigger>
              <TooltipContent>Toggle plan mode</TooltipContent>
            </Tooltip>
            {/* Context window usage indicator */}
            {contextWindow && <TokenUsageDisplay contextWindow={contextWindow} />}
          </div>
          <div className="flex items-center gap-2">
            <SessionsDropdown
              taskId={taskId}
              activeSessionId={resolvedSessionId}
              taskTitle={task?.title}
              taskDescription={taskDescription ?? ''}
            />
            <Tooltip>
              <TooltipTrigger asChild>
                <Button type="button" variant="outline" size="icon" className="h-7 w-7 cursor-pointer">
                  <IconPaperclip className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Add attachments</TooltipContent>
            </Tooltip>
            <KeyboardShortcutTooltip shortcut={SHORTCUTS.SUBMIT} enabled={!isAgentBusy && !isStarting}>
              <Button
                type={isAgentBusy ? 'button' : 'submit'}
                variant={isAgentBusy ? 'destructive' : 'default'}
                className={cn('h-7', isAgentBusy && 'gap-2')}
                disabled={isStarting || isSending}
                onClick={isAgentBusy ? handleCancelTurn : undefined}
              >
                {isAgentBusy ? (
                  <>
                    <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
                    Stop
                  </>
                ) : (
                  'Submit'
                )}
              </Button>
            </KeyboardShortcutTooltip>
          </div>
        </div>
      </form>
    </>
  );
}
