import {
  type ClarificationRequestMetadata,
  type Message,
  type MessageType,
} from "@/lib/types/http";
import type { RichMetadata, ToolCallMetadata, TodoSnapshot } from "@/components/task/chat/types";
import {
  findPendingClarification,
  isPendingClarificationMessage,
  type PendingClarificationScope,
} from "@/lib/utils/pending-clarification";

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

type AgentBootMetadata = {
  script_type?: string;
  is_resuming?: boolean;
  status?: string;
  exit_code?: number;
};

export type ScriptExecutionOutcomeMetadata = Pick<AgentBootMetadata, "status" | "exit_code">;

/** True when a script execution row reports a zero or absent exit code. */
export function isSuccessfulScriptExecutionMetadata(
  metadata: ScriptExecutionOutcomeMetadata | undefined,
): boolean {
  return (
    metadata?.status === "exited" && (metadata.exit_code === 0 || metadata.exit_code === undefined)
  );
}

/** True when a script_execution row reports an agent that finished booting
 *  successfully, whether resumed or freshly started. Mirrors the success rule
 *  the boot header renders with (script-execution-message.tsx): an "exited"
 *  status carrying a zero or absent exit code. */
export function isSuccessfulAgentBootMessage(message: Message): boolean {
  if (!isAgentBootMessage(message)) return false;
  const metadata = message.metadata as AgentBootMetadata | undefined;
  return isSuccessfulScriptExecutionMetadata(metadata);
}

/** True when a message is an agent boot row, whatever its outcome. */
function isAgentBootMessage(message: Message): boolean {
  if (message.type !== "script_execution") return false;
  const metadata = message.metadata as AgentBootMetadata | undefined;
  return metadata?.script_type === "agent_boot";
}

/** True when an agent boot after `afterCreatedAt` reported a failure — the signal that a
 *  requested recovery did not take, so its card must come back. */
export function hasFailedAgentBootAfter(
  messages: Message[] | undefined,
  afterCreatedAt: string | undefined,
): boolean {
  const failedAt = Date.parse(afterCreatedAt ?? "");
  if (Number.isNaN(failedAt) || !messages?.length) return false;
  return messages.some((message) => {
    if (!isAgentBootMessage(message) || isSuccessfulAgentBootMessage(message)) return false;
    const metadata = message.metadata as AgentBootMetadata | undefined;
    if (metadata?.status !== "failed" && metadata?.status !== "exited") return false;
    const bootedAt = Date.parse(message.created_at ?? "");
    return !Number.isNaN(bootedAt) && bootedAt > failedAt;
  });
}

/** True when the agent was successfully (re)established after `afterCreatedAt`.
 *  A recovery card is persisted against the failure that produced it, so this is
 *  the durable signal that the card is stale. Unlike an in-memory acknowledgment
 *  of the Resume click it survives a reload or task switch, and it also covers
 *  the auto-resume-on-open path where no button was ever pressed. */
export function hasSuccessfulAgentBootAfter(
  messages: Message[] | undefined,
  afterCreatedAt: string | undefined,
): boolean {
  const failedAt = Date.parse(afterCreatedAt ?? "");
  if (Number.isNaN(failedAt) || !messages?.length) return false;
  return messages.some((message) => {
    if (!isSuccessfulAgentBootMessage(message)) return false;
    const bootedAt = Date.parse(message.created_at ?? "");
    return !Number.isNaN(bootedAt) && bootedAt > failedAt;
  });
}

/** True when session metadata records a successful boot after a recovery card. */
export function hasSessionRecoveryResolutionAfter(
  metadata: Record<string, unknown> | null | undefined,
  afterCreatedAt: string | undefined,
): boolean {
  const resolvedAt = Date.parse(
    typeof metadata?.recovery_resolved_at === "string" ? metadata.recovery_resolved_at : "",
  );
  const failedAt = Date.parse(afterCreatedAt ?? "");
  return !Number.isNaN(resolvedAt) && !Number.isNaN(failedAt) && resolvedAt > failedAt;
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

function findActiveClarification(
  messages: Message[],
  scope?: PendingClarificationScope,
): Message | undefined {
  return findPendingClarification(messages, scope) ?? undefined;
}

function isClarificationVisible(
  message: Message,
  activeClarification: Message | undefined,
): boolean {
  const metadata = message.metadata as ClarificationRequestMetadata | undefined;
  if (!isPendingClarificationMessage(message)) return true;
  if (activeClarification) {
    const activeMetadata = activeClarification.metadata as ClarificationRequestMetadata | undefined;
    return activeMetadata?.pending_id
      ? metadata?.pending_id !== activeMetadata.pending_id
      : message.id !== activeClarification.id;
  }
  return true;
}

export function filterVisibleMessages(
  messages: Message[],
  toolCallIds: Set<string>,
  subagentChildIds: Set<string>,
  scope?: PendingClarificationScope,
): Message[] {
  const activeClarification = findActiveClarification(messages, scope);
  const filtered = messages.filter((message) => {
    if (subagentChildIds.has(message.id) || isSetupScriptMessage(message)) return false;
    if (message.type === "clarification_request") {
      return isClarificationVisible(message, activeClarification);
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
