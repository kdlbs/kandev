import {
  type ClarificationRequestMetadata,
  type Message,
  type MessageType,
  type TaskPendingAction,
} from "@/lib/types/http";
import type { RichMetadata, ToolCallMetadata, TodoSnapshot } from "@/components/task/chat/types";

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

export function buildToolCallIds(messages: Message[]): Set<string> {
  const set = new Set<string>();
  for (const message of messages) {
    if (message.type === "tool_call") {
      const toolCallId = (message.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
      if (toolCallId) set.add(toolCallId);
    }
  }
  return set;
}

export function buildPermissionsByToolCallId(messages: Message[]): Map<string, Message> {
  const map = new Map<string, Message>();
  for (const message of messages) {
    if (message.type === "permission_request") {
      const toolCallId = (message.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
      if (toolCallId) map.set(toolCallId, message);
    }
  }
  return map;
}

export function buildChildrenByParentToolCallId(messages: Message[]): Map<string, Message[]> {
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

export function buildSubagentChildIds(
  childrenByParentToolCallId: Map<string, Message[]>,
): Set<string> {
  const set = new Set<string>();
  for (const children of childrenByParentToolCallId.values()) {
    for (const child of children) set.add(child.id);
  }
  return set;
}

function isRecoveryMessage(message: Message): boolean {
  const metadata = message.metadata as Record<string, unknown> | undefined;
  return metadata?.recovery_actions === true;
}

function deduplicateRecoveryMessages(messages: Message[]): Message[] {
  let lastRecoveryIndex = -1;
  for (let index = messages.length - 1; index >= 0; index--) {
    if (isRecoveryMessage(messages[index])) {
      lastRecoveryIndex = index;
      break;
    }
  }
  if (lastRecoveryIndex === -1) return messages;
  const hasLaterActivity = messages
    .slice(lastRecoveryIndex + 1)
    .some((message) => message.type === "message" || message.type === "content");
  return messages.filter((message, index) => {
    if (!isRecoveryMessage(message)) return true;
    if (hasLaterActivity) return false;
    return index === lastRecoveryIndex;
  });
}

export function isAgentBootResumeMessage(message: Message): boolean {
  if (message.type !== "script_execution") return false;
  const metadata = message.metadata as { script_type?: string; is_resuming?: boolean } | undefined;
  return metadata?.script_type === "agent_boot" && metadata?.is_resuming === true;
}

export function isSetupScriptMessage(message: Message): boolean {
  if (message.type !== "script_execution") return false;
  const metadata = message.metadata as { script_type?: string } | undefined;
  return metadata?.script_type === "setup";
}

export function deduplicateAgentBootResumes(messages: Message[]): Message[] {
  let lastResumeIndex = -1;
  for (let index = messages.length - 1; index >= 0; index--) {
    if (isAgentBootResumeMessage(messages[index])) {
      lastResumeIndex = index;
      break;
    }
  }
  if (lastResumeIndex === -1) return messages;
  return messages.filter(
    (message, index) => !isAgentBootResumeMessage(message) || index === lastResumeIndex,
  );
}

function findLatestTodoIdsByTurn(messages: Message[]): Map<string, string> {
  const latest = new Map<string, string>();
  for (const message of messages) {
    if (message.type === "todo" && message.turn_id) {
      latest.set(message.turn_id, message.id);
    }
  }
  return latest;
}

function collectPriorSnapshotsByLatestId(
  messages: Message[],
  latestTodoIdByTurn: Map<string, string>,
): Map<string, TodoSnapshot[]> {
  const previousByLatestId = new Map<string, TodoSnapshot[]>();
  for (const message of messages) {
    if (message.type !== "todo" || !message.turn_id) continue;
    const latestId = latestTodoIdByTurn.get(message.turn_id);
    if (!latestId || latestId === message.id) continue;
    const snapshot: TodoSnapshot = {
      todos: (message.metadata as RichMetadata | undefined)?.todos ?? [],
      created_at: message.created_at,
    };
    if (!previousByLatestId.has(latestId)) previousByLatestId.set(latestId, []);
    previousByLatestId.get(latestId)!.push(snapshot);
  }
  return previousByLatestId;
}

export function collapseTodoSnapshotsPerTurn(messages: Message[]): Message[] {
  const latestTodoIdByTurn = findLatestTodoIdsByTurn(messages);
  if (latestTodoIdByTurn.size === 0) return messages;
  const previousByLatestId = collectPriorSnapshotsByLatestId(messages, latestTodoIdByTurn);
  return messages.flatMap((message) => {
    if (message.type !== "todo" || !message.turn_id) return [message];
    if (latestTodoIdByTurn.get(message.turn_id) !== message.id) return [];
    const previous = previousByLatestId.get(message.id);
    if (!previous || previous.length === 0) return [message];
    return [{ ...message, metadata: { ...message.metadata, previous_todo_snapshots: previous } }];
  });
}

function findUnhydratedActiveClarification(
  messages: Message[],
  currentTurnId?: string | null,
  pendingAction?: TaskPendingAction | null,
): Message | undefined {
  if (currentTurnId !== undefined || pendingAction !== "clarification") return undefined;
  for (let index = messages.length - 1; index >= 0; index--) {
    const candidate = messages[index];
    const metadata = candidate.metadata as ClarificationRequestMetadata | undefined;
    if (
      candidate.type === "clarification_request" &&
      (!metadata?.status || metadata.status === "pending")
    ) {
      return candidate;
    }
  }
  return undefined;
}

function isClarificationVisible(
  message: Message,
  currentTurnId: string | null | undefined,
  pendingAction: TaskPendingAction | null | undefined,
  unhydratedActive: Message | undefined,
): boolean {
  const metadata = message.metadata as ClarificationRequestMetadata | undefined;
  if (metadata?.status && metadata.status !== "pending") return true;
  if (currentTurnId !== undefined) {
    return typeof currentTurnId === "string" && message.turn_id !== currentTurnId;
  }
  if (unhydratedActive) {
    const activeMetadata = unhydratedActive.metadata as ClarificationRequestMetadata | undefined;
    return activeMetadata?.pending_id
      ? metadata?.pending_id !== activeMetadata.pending_id
      : message.id !== unhydratedActive.id;
  }
  return pendingAction !== undefined && pendingAction !== "clarification";
}

export function filterVisibleMessages(
  messages: Message[],
  toolCallIds: Set<string>,
  subagentChildIds: Set<string>,
  currentTurnId?: string | null,
  pendingAction?: TaskPendingAction | null,
): Message[] {
  const unhydratedActive = findUnhydratedActiveClarification(
    messages,
    currentTurnId,
    pendingAction,
  );
  const filtered = messages.filter((message) => {
    if (subagentChildIds.has(message.id) || isSetupScriptMessage(message)) return false;
    if (message.type === "clarification_request") {
      return isClarificationVisible(message, currentTurnId, pendingAction, unhydratedActive);
    }
    if (
      message.type === "status" &&
      (message.content === "New session started" || message.content === "Session resumed")
    ) {
      return false;
    }
    if (isVisibleMessageType(message.type)) return true;
    if (message.type === "permission_request") return isPermissionVisible(message, toolCallIds);
    return false;
  });
  return collapseTodoSnapshotsPerTurn(
    deduplicateAgentBootResumes(deduplicateRecoveryMessages(filtered)),
  );
}
