import { useMemo } from 'react';
import type { Message, ClarificationRequestMetadata, MessageType } from '@/lib/types/http';
import type { ToolCallMetadata } from '@/components/task/chat/types';

const ACTIVITY_MESSAGE_TYPES: Set<MessageType> = new Set([
  'thinking', 'tool_call', 'tool_edit', 'tool_read', 'tool_execute', 'tool_search',
]);

const VISIBLE_MESSAGE_TYPES: Set<string> = new Set([
  'message', 'content', 'tool_call', 'tool_read', 'tool_edit', 'tool_execute',
  'tool_search', 'progress', 'status', 'error', 'thinking', 'todo', 'script_execution',
]);

function isVisibleMessageType(type: MessageType | undefined): boolean {
  return !type || VISIBLE_MESSAGE_TYPES.has(type);
}

function isPermissionVisible(message: Message, toolCallIds: Set<string>): boolean {
  const metadata = message.metadata as { tool_call_id?: string; status?: string } | undefined;
  const toolCallId = metadata?.tool_call_id;
  if (toolCallId && toolCallIds.has(toolCallId)) return false;
  const status = metadata?.status;
  if (status === 'approved' || status === 'denied' || status === 'cancelled') return false;
  return true;
}

export type TurnGroup = {
  type: 'turn_group';
  id: string;
  turnId: string | null;
  messages: Message[];
};

export type RenderItem =
  | { type: 'message'; message: Message }
  | TurnGroup;

function buildToolCallIds(messages: Message[]): Set<string> {
  const set = new Set<string>();
  for (const message of messages) {
    if (message.type === 'tool_call') {
      const toolCallId = (message.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
      if (toolCallId) set.add(toolCallId);
    }
  }
  return set;
}

function buildPermissionsByToolCallId(messages: Message[]): Map<string, Message> {
  const map = new Map<string, Message>();
  for (const message of messages) {
    if (message.type === 'permission_request') {
      const toolCallId = (message.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
      if (toolCallId) map.set(toolCallId, message);
    }
  }
  return map;
}

function buildChildrenByParentToolCallId(messages: Message[]): Map<string, Message[]> {
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
}

function buildSubagentChildIds(childrenByParentToolCallId: Map<string, Message[]>): Set<string> {
  const set = new Set<string>();
  for (const children of childrenByParentToolCallId.values()) {
    for (const child of children) set.add(child.id);
  }
  return set;
}

function findPendingClarification(messages: Message[]): Message | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const message = messages[i];
    if (message.type === 'clarification_request') {
      const metadata = message.metadata as ClarificationRequestMetadata | undefined;
      if (!metadata?.status || metadata.status === 'pending') return message;
    }
  }
  return null;
}

function filterVisibleMessages(
  messages: Message[],
  toolCallIds: Set<string>,
  subagentChildIds: Set<string>,
): Message[] {
  return messages.filter((message) => {
    if (subagentChildIds.has(message.id)) return false;
    if (message.type === 'clarification_request') {
      const metadata = message.metadata as ClarificationRequestMetadata | undefined;
      return !(!metadata?.status || metadata.status === 'pending');
    }
    if (message.type === 'status' && (message.content === 'New session started' || message.content === 'Session resumed')) return false;
    if (isVisibleMessageType(message.type)) return true;
    if (message.type === 'permission_request') return isPermissionVisible(message, toolCallIds);
    return false;
  });
}

function groupActivityMessages(allMessages: Message[]): RenderItem[] {
  const items: RenderItem[] = [];
  let currentGroup: Message[] = [];
  let currentTurnId: string | null = null;

  const flushGroup = () => {
    if (currentGroup.length >= 2) {
      items.push({ type: 'turn_group', id: `turn-group-${currentGroup[0].id}`, turnId: currentGroup[0].turn_id ?? null, messages: currentGroup });
    } else if (currentGroup.length === 1) {
      items.push({ type: 'message', message: currentGroup[0] });
    }
    currentGroup = [];
    currentTurnId = null;
  };

  for (const message of allMessages) {
    const isActivity = message.type && ACTIVITY_MESSAGE_TYPES.has(message.type);
    const messageTurnId = message.turn_id ?? null;
    if (isActivity && messageTurnId) {
      if (currentGroup.length > 0 && currentTurnId === messageTurnId) {
        currentGroup.push(message);
      } else {
        flushGroup();
        currentGroup = [message];
        currentTurnId = messageTurnId;
      }
    } else {
      flushGroup();
      items.push({ type: 'message', message });
    }
  }
  flushGroup();
  return items;
}

export function useProcessedMessages(
  messages: Message[],
  taskId: string | null,
  resolvedSessionId: string | null,
  taskDescription: string | null
) {
  const toolCallIds = useMemo(() => buildToolCallIds(messages), [messages]);
  const permissionsByToolCallId = useMemo(() => buildPermissionsByToolCallId(messages), [messages]);
  const childrenByParentToolCallId = useMemo(() => buildChildrenByParentToolCallId(messages), [messages]);
  const subagentChildIds = useMemo(() => buildSubagentChildIds(childrenByParentToolCallId), [childrenByParentToolCallId]);
  const pendingClarification = useMemo(() => findPendingClarification(messages), [messages]);

  const visibleMessages = useMemo(
    () => filterVisibleMessages(messages, toolCallIds, subagentChildIds),
    [messages, toolCallIds, subagentChildIds]
  );

  const taskDescriptionMessage: Message | null = useMemo(() => {
    return taskDescription && visibleMessages.length === 0
      ? { id: 'task-description', task_id: taskId ?? '', session_id: resolvedSessionId ?? '', author_type: 'user', content: taskDescription, type: 'message', created_at: '' }
      : null;
  }, [taskDescription, visibleMessages.length, taskId, resolvedSessionId]);

  const allMessages = useMemo(() => {
    return taskDescriptionMessage ? [taskDescriptionMessage, ...visibleMessages] : visibleMessages;
  }, [taskDescriptionMessage, visibleMessages]);

  const todoItems = useMemo(() => {
    const latestTodos = [...visibleMessages].reverse()
      .find((message) => message.type === 'todo' || (message.metadata as { todos?: unknown })?.todos);
    return (
      (latestTodos?.metadata as { todos?: Array<{ text: string; done?: boolean } | string> } | undefined)?.todos
        ?.map((item) => (typeof item === 'string' ? { text: item, done: false } : item))
        .filter((item) => item.text) ?? []
    );
  }, [visibleMessages]);

  const agentMessageCount = useMemo(() => {
    return visibleMessages.filter((c) => c.author_type !== 'user').length;
  }, [visibleMessages]);

  const groupedItems = useMemo<RenderItem[]>(() => groupActivityMessages(allMessages), [allMessages]);

  return {
    visibleMessages, allMessages, groupedItems, toolCallIds,
    permissionsByToolCallId, childrenByParentToolCallId, todoItems,
    agentMessageCount, pendingClarification,
  };
}
