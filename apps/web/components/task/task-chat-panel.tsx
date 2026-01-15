'use client';

import { FormEvent, useCallback, useEffect, useRef, useState } from 'react';
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn } from '@/lib/utils';
import { useAppStore } from '@/components/state-provider';
import type { Message } from '@/lib/types/http';
import { SHORTCUTS } from '@/lib/keyboard/constants';
import { KeyboardShortcutTooltip } from '@/components/keyboard-shortcut-tooltip';
import { TaskChatInput } from '@/components/task/task-chat-input';
import { useLazyLoadMessages } from '@/hooks/use-lazy-load-messages';
import { MessageRenderer } from '@/components/task/chat/message-renderer';
import { TypingIndicator } from '@/components/task/chat/messages/typing-indicator';
import { TodoSummary } from '@/components/task/chat/todo-summary';

type AgentOption = {
  id: string;
  label: string;
};

type TaskChatPanelProps = {
  agents: AgentOption[];
  onSend: (message: string) => void;
  isLoading?: boolean;
  isAgentWorking?: boolean;
  taskId?: string;
  taskDescription?: string;
};

export function TaskChatPanel({
  agents,
  onSend,
  isLoading,
  isAgentWorking,
  taskId,
  taskDescription,
}: TaskChatPanelProps) {
  const [messageInput, setMessageInput] = useState('');
  const [selectedAgent, setSelectedAgent] = useState(agents[0]?.id ?? '');
  const [planModeEnabled, setPlanModeEnabled] = useState(false);
  const [isSending, setIsSending] = useState(false);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const lastAgentMessageCountRef = useRef(0);
  const wasAtBottomRef = useRef(true);

  const messagesState = useAppStore((state) => state.messages);
  const messages = messagesState?.items ?? [];
  const messagesLoading = messagesState?.isLoading ?? false;
  const isInitialLoading = messagesLoading && messages.length === 0;
  const showLoadingState = (isLoading || isInitialLoading) && !isAgentWorking;
  const { loadMore, hasMore, isLoading: isLoadingMore } = useLazyLoadMessages(
    messagesState?.sessionId ?? null
  );

  // Filter to only show chat-relevant types.
  const visibleMessages = messages.filter((message) =>
    !message.type ||
    message.type === 'message' ||
    message.type === 'content' ||
    message.type === 'tool_call' ||
    message.type === 'progress' ||
    message.type === 'status' ||
    message.type === 'error' ||
    message.type === 'thinking' ||
    message.type === 'todo'
  );

  // Create a synthetic "user" message for the task description
  const taskDescriptionMessage: Message | null =
    taskDescription && visibleMessages.length === 0
      ? {
          id: 'task-description',
          task_id: taskId ?? '',
          agent_session_id: messagesState?.sessionId ?? '',
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

  const lastVisibleMessage = visibleMessages[visibleMessages.length - 1];
  const lastMessageRequestsInput = Boolean(lastVisibleMessage?.requests_input);
  const shouldShowTypingIndicator = Boolean(isAgentWorking) && !lastMessageRequestsInput;
  const typingIndicatorLabel = visibleMessages.length === 0 ? 'Agent is starting...' : 'Agent is thinking...';

  const itemCount = allMessages.length + (shouldShowTypingIndicator ? 1 : 0);

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

  const handleSubmit = async (event?: FormEvent) => {
    event?.preventDefault();
    const trimmed = messageInput.trim();
    if (!trimmed || isSending) return;
    setIsSending(true);
    flushSync(() => {
      setMessageInput('');
    });
    try {
      await onSend(trimmed);
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
        {showLoadingState && (
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            <IconLoader2 className="h-5 w-5 animate-spin mr-2" />
            <span>Loading messages...</span>
          </div>
        )}
        {!isLoading && !isInitialLoading && visibleMessages.length === 0 && !isAgentWorking && (
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            <span>No messages yet. Start the conversation!</span>
          </div>
        )}
        {!isLoading && !isInitialLoading && itemCount > 0 && (
          <div className="relative w-full" style={{ height: `${virtualizer.getTotalSize()}px` }}>
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const isTypingRow = shouldShowTypingIndicator && virtualRow.index === allMessages.length;
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
                    {isTypingRow ? (
                      <TypingIndicator label={typingIndicatorLabel} />
                    ) : (
                      <MessageRenderer
                        comment={message}
                        isTaskDescription={message.id === 'task-description'}
                      />
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
      <form onSubmit={handleSubmit} className="mt-3 flex flex-col gap-2">
        {todoItems.length > 0 && (
          <TodoSummary todos={todoItems} />
        )}
        <TaskChatInput
          value={messageInput}
          onChange={setMessageInput}
          onSubmit={() => handleSubmit()}
          placeholder="Write to submit work to the agent..."
          planModeEnabled={planModeEnabled}
        />
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Select value={selectedAgent} onValueChange={setSelectedAgent}>
              <SelectTrigger className="w-[160px] cursor-pointer">
                <SelectValue placeholder="Select agent" />
              </SelectTrigger>
              <SelectContent>
                {agents.map((agent) => (
                  <SelectItem key={agent.id} value={agent.id}>
                    {agent.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <DropdownMenu>
              <Tooltip>
                <TooltipTrigger asChild>
                  <DropdownMenuTrigger asChild>
                    <Button type="button" variant="outline" size="icon" className="h-9 w-9 cursor-pointer">
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
                      'h-9 w-9 cursor-pointer',
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
          </div>
          <div className="flex items-center gap-2">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button type="button" variant="outline" size="icon" className="h-9 w-9 cursor-pointer">
                  <IconPaperclip className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Add attachments</TooltipContent>
            </Tooltip>
            <KeyboardShortcutTooltip shortcut={SHORTCUTS.SUBMIT}>
              <Button type="submit">Submit</Button>
            </KeyboardShortcutTooltip>
          </div>
        </div>
      </form>
    </>
  );
}
