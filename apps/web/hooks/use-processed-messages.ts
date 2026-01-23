import { useMemo } from 'react';
import type { Message } from '@/lib/types/http';

export function useProcessedMessages(
  messages: Message[],
  taskId: string | null,
  resolvedSessionId: string | null,
  taskDescription: string | null
) {
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
  const visibleMessages = useMemo(() => {
    return messages.filter((message) => {
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
        message.type === 'todo' ||
        message.type === 'script_execution'
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
  }, [messages, toolCallIds]);

  // Create a synthetic "user" message for the task description
  const taskDescriptionMessage: Message | null = useMemo(() => {
    return taskDescription && visibleMessages.length === 0
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
  }, [taskDescription, visibleMessages.length, taskId, resolvedSessionId]);

  // Combine task description with visible messages
  const allMessages = useMemo(() => {
    return taskDescriptionMessage ? [taskDescriptionMessage, ...visibleMessages] : visibleMessages;
  }, [taskDescriptionMessage, visibleMessages]);

  // Extract todo items from latest message
  const todoItems = useMemo(() => {
    const latestTodos = [...visibleMessages]
      .reverse()
      .find((message) => message.type === 'todo' || (message.metadata as { todos?: unknown })?.todos);

    return (
      (latestTodos?.metadata as { todos?: Array<{ text: string; done?: boolean } | string> } | undefined)?.todos
        ?.map((item) => (typeof item === 'string' ? { text: item, done: false } : item))
        .filter((item) => item.text) ?? []
    );
  }, [visibleMessages]);

  // Count agent messages to detect new responses
  const agentMessageCount = useMemo(() => {
    return visibleMessages.filter((c) => c.author_type !== 'user').length;
  }, [visibleMessages]);

  return {
    visibleMessages,
    allMessages,
    toolCallIds,
    permissionsByToolCallId,
    todoItems,
    agentMessageCount,
  };
}
