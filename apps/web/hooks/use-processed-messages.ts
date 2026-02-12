import { useMemo } from 'react';
import type { Message, ClarificationRequestMetadata, MessageType } from '@/lib/types/http';
import type { ToolCallMetadata } from '@/components/task/chat/types';

// Activity types that get grouped together
const ACTIVITY_MESSAGE_TYPES: Set<MessageType> = new Set([
  'thinking',
  'tool_call',
  'tool_edit',
  'tool_read',
  'tool_execute',
  'tool_search',
]);

export type TurnGroup = {
  type: 'turn_group';
  id: string;
  turnId: string | null;
  messages: Message[];
};

export type RenderItem =
  | { type: 'message'; message: Message }
  | TurnGroup;

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

  // Build a map of parent_tool_call_id → child messages for subagent nesting.
  // When an agent sends parent_tool_use_id, child tools should be nested under the parent Task.
  const childrenByParentToolCallId = useMemo(() => {
    const map = new Map<string, Message[]>();
    for (const message of messages) {
      const metadata = message.metadata as ToolCallMetadata | undefined;
      const parentId = metadata?.parent_tool_call_id;
      if (parentId) {
        const children = map.get(parentId) || [];
        children.push(message);
        map.set(parentId, children);
      }
    }
    return map;
  }, [messages]);

  // Build a set of message IDs that are children of a subagent (should not render at top level)
  const subagentChildIds = useMemo(() => {
    const set = new Set<string>();
    for (const children of childrenByParentToolCallId.values()) {
      for (const child of children) {
        set.add(child.id);
      }
    }
    return set;
  }, [childrenByParentToolCallId]);

  // Find pending clarification request (the most recent one that is still pending)
  // This will be rendered in the input area, not in the message list
  const pendingClarification = useMemo(() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      const message = messages[i];
      if (message.type === 'clarification_request') {
        const metadata = message.metadata as ClarificationRequestMetadata | undefined;
        // A clarification is pending if it has no status or status is 'pending'
        if (!metadata?.status || metadata.status === 'pending') {
          return message;
        }
      }
    }
    return null;
  }, [messages]);

  // Filter to only show chat-relevant types.
  // Permission requests are only hidden if they have a matching tool_call in the messages
  // (so they can be displayed inline). Standalone permissions are shown as separate messages.
  // Pending clarification requests are also hidden (shown in input area instead).
  // Child messages of subagents are hidden at top level (rendered nested under parent).
  const visibleMessages = useMemo(() => {
    return messages.filter((message) => {
      // Hide child messages of subagents (they render nested under parent Task tool)
      if (subagentChildIds.has(message.id)) {
        return false;
      }

      // Hide pending clarification request (it's shown in input area)
      if (message.type === 'clarification_request') {
        const metadata = message.metadata as ClarificationRequestMetadata | undefined;
        const isPending = !metadata?.status || metadata.status === 'pending';
        // Only show clarification requests that are resolved (not pending)
        return !isPending;
      }

      // Hide session status messages — redundant with agent boot message
      if (message.type === 'status' && (message.content === 'New session started' || message.content === 'Session resumed')) {
        return false;
      }

      // Standard visible types
      if (
        !message.type ||
        message.type === 'message' ||
        message.type === 'content' ||
        message.type === 'tool_call' ||
        message.type === 'tool_read' ||
        message.type === 'tool_edit' ||
        message.type === 'tool_execute' ||
        message.type === 'tool_search' ||
        message.type === 'progress' ||
        message.type === 'status' ||
        message.type === 'error' ||
        message.type === 'thinking' ||
        message.type === 'todo' ||
        message.type === 'script_execution'
      ) {
        return true;
      }

      // Permission requests: hide if merged with a matching tool_call message,
      // or if already resolved (approved/denied) — no longer needs user attention.
      if (message.type === 'permission_request') {
        const metadata = message.metadata as { tool_call_id?: string; status?: string } | undefined;
        const toolCallId = metadata?.tool_call_id;
        // Hide if merged with an existing tool_call message (shown inline there)
        if (toolCallId && toolCallIds.has(toolCallId)) return false;
        // Hide if resolved — the user already acted on it
        const status = metadata?.status;
        if (status === 'approved' || status === 'denied' || status === 'cancelled') return false;
        // Show as standalone pending permission
        return true;
      }

      return false;
    });
  }, [messages, toolCallIds, subagentChildIds]);

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

  // Group consecutive activity messages (thinking + tool types) with same turn_id
  // Only create groups if 2+ activity messages exist consecutively with the same turn_id
  // Messages without turn_id are NOT grouped
  const groupedItems = useMemo<RenderItem[]>(() => {
    const items: RenderItem[] = [];
    let currentGroup: Message[] = [];
    let currentTurnId: string | null = null;

    const flushGroup = () => {
      if (currentGroup.length >= 2) {
        // Create a turn group - use first message ID for unique ID
        const groupTurnId = currentGroup[0].turn_id;
        const firstMsgId = currentGroup[0].id;
        items.push({
          type: 'turn_group',
          // Use first message ID to ensure unique group IDs even within same turn
          id: `turn-group-${firstMsgId}`,
          turnId: groupTurnId ?? null,
          messages: currentGroup,
        });
      } else if (currentGroup.length === 1) {
        // Single message, don't group
        items.push({ type: 'message', message: currentGroup[0] });
      }
      currentGroup = [];
      currentTurnId = null;
    };

    for (const message of allMessages) {
      const isActivity = message.type && ACTIVITY_MESSAGE_TYPES.has(message.type);
      const messageTurnId = message.turn_id ?? null;

      // Only group activity messages that have an actual turn_id
      if (isActivity && messageTurnId) {
        // Check if this continues the current group (same turn_id)
        if (currentGroup.length > 0 && currentTurnId === messageTurnId) {
          currentGroup.push(message);
        } else {
          // Flush previous group and start new one
          flushGroup();
          currentGroup = [message];
          currentTurnId = messageTurnId;
        }
      } else {
        // Non-activity message or activity without turn_id breaks the group
        flushGroup();
        items.push({ type: 'message', message });
      }
    }

    // Flush any remaining group
    flushGroup();

    return items;
  }, [allMessages]);

  return {
    visibleMessages,
    allMessages,
    groupedItems,
    toolCallIds,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    todoItems,
    agentMessageCount,
    pendingClarification,
  };
}
