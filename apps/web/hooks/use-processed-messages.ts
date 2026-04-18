import { useMemo } from "react";
import type { Message, ClarificationRequestMetadata, MessageType } from "@/lib/types/http";
import type { ToolCallMetadata } from "@/components/task/chat/types";

const ACTIVITY_MESSAGE_TYPES: Set<MessageType> = new Set([
  "thinking",
  "tool_call",
  "tool_edit",
  "tool_read",
  "tool_execute",
  "tool_search",
]);

const VISIBLE_MESSAGE_TYPES: Set<string> = new Set([
  "message",
  "content",
  "tool_call",
  "tool_read",
  "tool_edit",
  "tool_execute",
  "tool_search",
  "progress",
  "status",
  "error",
  "thinking",
  "todo",
  "script_execution",
  "agent_plan",
]);

function isVisibleMessageType(type: MessageType | undefined): boolean {
  return !type || VISIBLE_MESSAGE_TYPES.has(type);
}

function isPermissionVisible(message: Message, toolCallIds: Set<string>): boolean {
  const metadata = message.metadata as { tool_call_id?: string; status?: string } | undefined;
  const toolCallId = metadata?.tool_call_id;
  if (toolCallId && toolCallIds.has(toolCallId)) return false;
  const status = metadata?.status;
  if (status === "approved" || status === "denied" || status === "cancelled") return false;
  return true;
}

export type TurnGroup = {
  type: "turn_group";
  id: string;
  turnId: string | null;
  messages: Message[];
};

export type PrepareProgressItem = {
  type: "prepare_progress";
  id: string;
  sessionId: string;
};

export type RenderItem = { type: "message"; message: Message } | TurnGroup | PrepareProgressItem;

function buildToolCallIds(messages: Message[]): Set<string> {
  const set = new Set<string>();
  for (const message of messages) {
    if (message.type === "tool_call") {
      const toolCallId = (message.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
      if (toolCallId) set.add(toolCallId);
    }
  }
  return set;
}

function buildPermissionsByToolCallId(messages: Message[]): Map<string, Message> {
  const map = new Map<string, Message>();
  for (const message of messages) {
    if (message.type === "permission_request") {
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
    if (message.type === "clarification_request") {
      const metadata = message.metadata as ClarificationRequestMetadata | undefined;
      if (!metadata?.status || metadata.status === "pending") return message;
    }
  }
  return null;
}

function isRecoveryMessage(message: Message): boolean {
  const meta = message.metadata as Record<string, unknown> | undefined;
  return meta?.recovery_actions === true;
}

/** Hide recovery messages that have been superseded by later conversation activity
 *  (user/agent messages prove the session recovered) or by a newer recovery message. */
function deduplicateRecoveryMessages(messages: Message[]): Message[] {
  let lastRecoveryIdx = -1;
  for (let i = messages.length - 1; i >= 0; i--) {
    if (isRecoveryMessage(messages[i])) {
      lastRecoveryIdx = i;
      break;
    }
  }
  if (lastRecoveryIdx === -1) return messages;

  const hasLaterActivity = messages
    .slice(lastRecoveryIdx + 1)
    .some((m) => m.type === "message" || m.type === "content");

  return messages.filter((msg, i) => {
    if (!isRecoveryMessage(msg)) return true;
    if (hasLaterActivity) return false;
    return i === lastRecoveryIdx;
  });
}

export function isAgentBootResumeMessage(message: Message): boolean {
  if (message.type !== "script_execution") return false;
  const meta = message.metadata as { script_type?: string; is_resuming?: boolean } | undefined;
  return meta?.script_type === "agent_boot" && meta?.is_resuming === true;
}

/** A resumed session may produce many "Resumed agent …" boot messages over its
 *  lifetime (every backend restart emits one). They all convey the same info;
 *  keep only the most recent and drop the rest — unconditionally, even if user
 *  messages occurred between them (unlike `deduplicateRecoveryMessages`). The
 *  underlying DB rows are untouched; this only affects the rendered chat. */
export function deduplicateAgentBootResumes(messages: Message[]): Message[] {
  let lastResumeIdx = -1;
  for (let i = messages.length - 1; i >= 0; i--) {
    if (isAgentBootResumeMessage(messages[i])) {
      lastResumeIdx = i;
      break;
    }
  }
  if (lastResumeIdx === -1) return messages;
  return messages.filter((msg, i) => !isAgentBootResumeMessage(msg) || i === lastResumeIdx);
}

function filterVisibleMessages(
  messages: Message[],
  toolCallIds: Set<string>,
  subagentChildIds: Set<string>,
): Message[] {
  const filtered = messages.filter((message) => {
    if (subagentChildIds.has(message.id)) return false;
    if (message.type === "clarification_request") {
      const metadata = message.metadata as ClarificationRequestMetadata | undefined;
      return !(!metadata?.status || metadata.status === "pending");
    }
    if (
      message.type === "status" &&
      (message.content === "New session started" || message.content === "Session resumed")
    )
      return false;
    if (isVisibleMessageType(message.type)) return true;
    if (message.type === "permission_request") return isPermissionVisible(message, toolCallIds);
    return false;
  });

  return deduplicateAgentBootResumes(deduplicateRecoveryMessages(filtered));
}

function groupActivityMessages(allMessages: Message[]): RenderItem[] {
  const items: RenderItem[] = [];
  let currentGroup: Message[] = [];
  let currentTurnId: string | null = null;

  const flushGroup = () => {
    if (currentGroup.length >= 2) {
      items.push({
        type: "turn_group",
        id: `turn-group-${currentGroup[0].id}`,
        turnId: currentGroup[0].turn_id ?? null,
        messages: currentGroup,
      });
    } else if (currentGroup.length === 1) {
      items.push({ type: "message", message: currentGroup[0] });
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
      items.push({ type: "message", message });
    }
  }
  flushGroup();
  return items;
}

function injectPrepareProgressItem(
  items: RenderItem[],
  resolvedSessionId: string | null,
): RenderItem[] {
  if (!resolvedSessionId) return items;
  const prepareItem: PrepareProgressItem = {
    type: "prepare_progress",
    id: `prepare-progress-${resolvedSessionId}`,
    sessionId: resolvedSessionId,
  };
  if (items.length === 0) return [prepareItem];
  return [items[0], prepareItem, ...items.slice(1)];
}

export function useProcessedMessages(
  messages: Message[],
  taskId: string | null,
  resolvedSessionId: string | null,
  taskDescription: string | null,
) {
  const toolCallIds = useMemo(() => buildToolCallIds(messages), [messages]);
  const permissionsByToolCallId = useMemo(() => buildPermissionsByToolCallId(messages), [messages]);
  const childrenByParentToolCallId = useMemo(
    () => buildChildrenByParentToolCallId(messages),
    [messages],
  );
  const subagentChildIds = useMemo(
    () => buildSubagentChildIds(childrenByParentToolCallId),
    [childrenByParentToolCallId],
  );
  const pendingClarification = useMemo(() => findPendingClarification(messages), [messages]);

  const visibleMessages = useMemo(
    () => filterVisibleMessages(messages, toolCallIds, subagentChildIds),
    [messages, toolCallIds, subagentChildIds],
  );

  const taskDescriptionMessage: Message | null = useMemo(() => {
    return taskDescription && visibleMessages.length === 0
      ? {
          id: "task-description",
          task_id: taskId ?? "",
          session_id: resolvedSessionId ?? "",
          author_type: "user",
          content: taskDescription,
          type: "message",
          created_at: "",
        }
      : null;
  }, [taskDescription, visibleMessages.length, taskId, resolvedSessionId]);

  const allMessages = useMemo(() => {
    return taskDescriptionMessage ? [taskDescriptionMessage, ...visibleMessages] : visibleMessages;
  }, [taskDescriptionMessage, visibleMessages]);

  const todoItems = useMemo(() => {
    const latestTodos = [...visibleMessages]
      .reverse()
      .find(
        (message) => message.type === "todo" || (message.metadata as { todos?: unknown })?.todos,
      );
    return (
      (
        latestTodos?.metadata as
          | { todos?: Array<{ text: string; done?: boolean } | string> }
          | undefined
      )?.todos
        ?.map((item) => (typeof item === "string" ? { text: item, done: false } : item))
        .filter((item) => item.text) ?? []
    );
  }, [visibleMessages]);

  const agentMessageCount = useMemo(() => {
    return visibleMessages.filter((c) => c.author_type !== "user").length;
  }, [visibleMessages]);

  // Separate footer action messages (e.g. missing branch guidance with archive/delete
  // buttons) from regular messages so they render after the env prep error status.
  const { regularMessages, footerActionMessages } = useMemo(() => {
    const regular: Message[] = [];
    const footer: Message[] = [];
    for (const msg of allMessages) {
      const meta = msg.metadata as Record<string, unknown> | undefined;
      if (Array.isArray(meta?.actions) && !meta?.recovery_actions) {
        footer.push(msg);
      } else {
        regular.push(msg);
      }
    }
    return { regularMessages: regular, footerActionMessages: footer };
  }, [allMessages]);

  const groupedItems = useMemo<RenderItem[]>(() => {
    return injectPrepareProgressItem(groupActivityMessages(regularMessages), resolvedSessionId);
  }, [regularMessages, resolvedSessionId]);

  return {
    visibleMessages,
    allMessages,
    groupedItems,
    footerActionMessages,
    toolCallIds,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    todoItems,
    agentMessageCount,
    pendingClarification,
  };
}
