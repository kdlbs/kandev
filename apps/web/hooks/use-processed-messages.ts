import { useEffect, useMemo, useState } from "react";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type ClarificationRequestMetadata,
  type Message,
  type MessageType,
} from "@/lib/types/http";
import type { RichMetadata, ToolCallMetadata, TodoSnapshot } from "@/components/task/chat/types";
import {
  findPendingClarification,
  findPendingClarificationGroup,
} from "@/lib/utils/pending-clarification";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import type { LastAgentError } from "@/lib/session-last-agent-error";

const debug = createDebugLogger("messages:process");

function countByType(messages: Message[]): Record<string, number> {
  const out: Record<string, number> = {};
  for (const m of messages) {
    const t = m.type ?? "unknown";
    out[t] = (out[t] ?? 0) + 1;
  }
  return out;
}

function useDebugProcessedPipeline(args: {
  sessionId: string | null;
  messages: Message[];
  visibleMessages: Message[];
  footerActionCount: number;
  groupedItems: RenderItem[];
}) {
  const { sessionId, messages, visibleMessages, footerActionCount, groupedItems } = args;
  useEffect(() => {
    if (!isDebug()) return;
    debug("pipeline", {
      sessionId,
      input: { count: messages.length, byType: countByType(messages) },
      afterFilter: { count: visibleMessages.length, byType: countByType(visibleMessages) },
      droppedByFilter: messages.length - visibleMessages.length,
      footerActionCount,
      groupedItemKinds: groupedItems.reduce<Record<string, number>>((acc, item) => {
        acc[item.type] = (acc[item.type] ?? 0) + 1;
        return acc;
      }, {}),
      turnGroupSizes: groupedItems
        .filter((i): i is TurnGroup => i.type === "turn_group")
        .map((g) => ({ turnId: g.turnId, size: g.messages.length })),
    });
  }, [sessionId, messages, visibleMessages, footerActionCount, groupedItems]);
}

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
  "advisor_feedback",
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

export type AgentErrorNoticeItem = {
  type: "agent_error_notice";
  id: string;
  sessionId: string;
  error: LastAgentError;
};

export type RenderItem =
  | { type: "message"; message: Message }
  | TurnGroup
  | PrepareProgressItem
  | AgentErrorNoticeItem;

export type GroupedRenderOptions = {
  canAnchorPrepareProgress?: boolean;
};

export type ProcessedMessagesOptions = {
  hasOlderMessages?: boolean;
  lastAgentError?: LastAgentError | null;
};

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

/** True when both maps hold the same keys pointing at the same Message refs. */
export function messageMapsEqualByIdentity(
  a: Map<string, Message>,
  b: Map<string, Message>,
): boolean {
  if (a.size !== b.size) return false;
  for (const [key, value] of a) {
    if (b.get(key) !== value) return false;
  }
  return true;
}

/** True when both maps hold the same keys pointing at arrays whose Message refs
 *  match positionally. */
export function messageListMapsEqual(
  a: Map<string, Message[]>,
  b: Map<string, Message[]>,
): boolean {
  if (a.size !== b.size) return false;
  for (const [key, value] of a) {
    const other = b.get(key);
    if (!other || other.length !== value.length) return false;
    for (let i = 0; i < value.length; i++) {
      if (value[i] !== other[i]) return false;
    }
  }
  return true;
}

/** Keep a prior reference when the freshly-built value is content-equal. The
 *  per-token `messages` array churns every reference downstream; holding the
 *  prior value lets `memo` on each message component survive commits where the
 *  derived map content didn't actually change. Uses the React-endorsed
 *  "adjust state during render" pattern so we never mutate a ref mid-render. */
function useStableValue<T>(next: T, equal: (a: T, b: T) => boolean): T {
  const [stable, setStable] = useState(next);
  if (!equal(stable, next)) {
    setStable(next);
    return next;
  }
  return stable;
}

function renderItemKey(item: RenderItem): string {
  return item.type === "message" ? `m:${item.message.id}` : item.id;
}

function agentErrorNoticeItemsEqual(a: AgentErrorNoticeItem, b: AgentErrorNoticeItem): boolean {
  return (
    a.id === b.id &&
    a.sessionId === b.sessionId &&
    a.error.message === b.error.message &&
    a.error.occurredAt === b.error.occurredAt &&
    a.error.agentExecutionId === b.error.agentExecutionId
  );
}

function turnGroupsEqual(a: TurnGroup, b: TurnGroup): boolean {
  if (a.id !== b.id || a.turnId !== b.turnId || a.messages.length !== b.messages.length) {
    return false;
  }
  for (let i = 0; i < a.messages.length; i++) {
    if (a.messages[i] !== b.messages[i]) return false;
  }
  return true;
}

/** Content-equal when the wrapper would render identically. Turn groups compare
 *  their messages by positional identity so a streaming token in the active turn
 *  yields a fresh wrapper while quiescent earlier turns stay equal. */
function renderItemsEqual(a: RenderItem, b: RenderItem): boolean {
  if (a.type !== b.type) return false;
  if (a.type === "message" && b.type === "message") return a.message === b.message;
  if (a.type === "prepare_progress" && b.type === "prepare_progress") return a.id === b.id;
  if (a.type === "agent_error_notice" && b.type === "agent_error_notice") {
    return agentErrorNoticeItemsEqual(a, b);
  }
  if (a.type === "turn_group" && b.type === "turn_group") {
    return turnGroupsEqual(a, b);
  }
  return false;
}

/** Reuse prior wrapper objects for items whose content didn't change so the
 *  `memo` on TurnGroupMessage/MessageRenderer survives new-message commits.
 *  Returns `prev` unchanged when every slot is positionally identical. */
export function reconcileRenderItems(prev: RenderItem[], next: RenderItem[]): RenderItem[] {
  const prevByKey = new Map<string, RenderItem>();
  for (const item of prev) prevByKey.set(renderItemKey(item), item);
  let identical = prev.length === next.length;
  const result = next.map((nextItem, i) => {
    const prevItem = prevByKey.get(renderItemKey(nextItem));
    const reused = prevItem && renderItemsEqual(prevItem, nextItem) ? prevItem : nextItem;
    if (reused !== prev[i]) identical = false;
    return reused;
  });
  return identical ? prev : result;
}

function useStableGroupedItems(next: RenderItem[]): RenderItem[] {
  const [stable, setStable] = useState(next);
  const reconciled = reconcileRenderItems(stable, next);
  if (reconciled !== stable) {
    setStable(reconciled);
    return reconciled;
  }
  return stable;
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

/** Per-repo setup scripts (`Repository.setup_script`) run during worktree
 *  creation and are persisted as `script_execution` rows. They belong
 *  conceptually to environment preparation, not the chat thread — `PrepareProgress`
 *  surfaces them as steps inside the env-prep panel. Hiding them from the chat
 *  prevents the script from rendering above the env-prep panel (which happens
 *  when the user prompt hasn't been recorded yet, e.g. MCP-auto-started subtasks). */
export function isSetupScriptMessage(message: Message): boolean {
  if (message.type !== "script_execution") return false;
  const meta = message.metadata as { script_type?: string } | undefined;
  return meta?.script_type === "setup";
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
    if (isSetupScriptMessage(message)) return false;
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

  return collapseTodoSnapshotsPerTurn(
    deduplicateAgentBootResumes(deduplicateRecoveryMessages(filtered)),
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

/** Some agents (Claude Opus/Sonnet) emit a fresh `todo` message every time
 *  they update their plan, producing a long stack of "Updated Todos" rows in
 *  the chat. Each todo message is a full snapshot of the list, so all but the
 *  latest in a turn are strictly stale. Collapse them: keep only the latest
 *  per `turn_id` and attach the earlier snapshots to its metadata as
 *  `previous_todo_snapshots` so the UI can show the progression on expand. */
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
  options: GroupedRenderOptions = {},
): RenderItem[] {
  if (!resolvedSessionId) return items;
  if (options.canAnchorPrepareProgress === false) return items;
  const prepareItem: PrepareProgressItem = {
    type: "prepare_progress",
    id: `prepare-progress-${resolvedSessionId}`,
    sessionId: resolvedSessionId,
  };
  if (items.length === 0) return [prepareItem];
  return [items[0], prepareItem, ...items.slice(1)];
}

export function buildGroupedRenderItems(
  regularMessages: Message[],
  resolvedSessionId: string | null,
  options: GroupedRenderOptions = {},
): RenderItem[] {
  return injectPrepareProgressItem(
    groupActivityMessages(regularMessages),
    resolvedSessionId,
    options,
  );
}

function canAnchorPrepareProgress(messages: Message[], hasOlderMessages: boolean): boolean {
  if (!hasOlderMessages) return true;
  return messages.some(isSetupScriptMessage);
}

function buildGroupedItemsForHook(args: {
  regularMessages: Message[];
  allSessionMessages: Message[];
  resolvedSessionId: string | null;
  hasOlderMessages: boolean;
  lastAgentError?: LastAgentError | null;
}): RenderItem[] {
  return insertLastAgentErrorItem(
    buildGroupedRenderItems(args.regularMessages, args.resolvedSessionId, {
      canAnchorPrepareProgress: canAnchorPrepareProgress(
        args.allSessionMessages,
        args.hasOlderMessages,
      ),
    }),
    args.resolvedSessionId,
    args.lastAgentError,
  );
}

function itemCreatedAt(item: RenderItem): string | undefined {
  if (item.type === "message") return item.message.created_at;
  if (item.type === "turn_group") return item.messages[item.messages.length - 1]?.created_at;
  return undefined;
}

function insertionIndexForAgentError(items: RenderItem[], occurredAt?: string): number {
  if (items.length === 0) return 0;
  const errorTime = occurredAt ? Date.parse(occurredAt) : Number.NaN;
  if (Number.isNaN(errorTime)) return items.length;
  let insertAt = 0;
  let sawTimestamp = false;
  for (let i = 0; i < items.length; i++) {
    const createdAt = itemCreatedAt(items[i]);
    if (!createdAt) continue;
    const itemTime = Date.parse(createdAt);
    if (Number.isNaN(itemTime)) continue;
    sawTimestamp = true;
    if (itemTime <= errorTime) insertAt = i + 1;
  }
  return sawTimestamp ? insertAt : items.length;
}

export function insertLastAgentErrorItem(
  items: RenderItem[],
  resolvedSessionId: string | null,
  error?: LastAgentError | null,
): RenderItem[] {
  if (!resolvedSessionId || !error) return items;
  const notice: AgentErrorNoticeItem = {
    type: "agent_error_notice",
    id: `last-agent-error-${resolvedSessionId}-${error.occurredAt ?? "unknown"}`,
    sessionId: resolvedSessionId,
    error,
  };
  const insertAt = insertionIndexForAgentError(items, error.occurredAt);
  return [...items.slice(0, insertAt), notice, ...items.slice(insertAt)];
}

function buildTodoItems(visibleMessages: Message[]) {
  const latestTodos = [...visibleMessages]
    .reverse()
    .find((message) => message.type === "todo" || (message.metadata as { todos?: unknown })?.todos);
  return (
    (
      latestTodos?.metadata as
        | { todos?: Array<{ text: string; done?: boolean } | string> }
        | undefined
    )?.todos
      ?.map((item) => (typeof item === "string" ? { text: item, done: false } : item))
      .filter((item) => item.text) ?? []
  );
}

export function useProcessedMessages(
  messages: Message[],
  taskId: string | null,
  resolvedSessionId: string | null,
  taskDescription: string | null,
  options: ProcessedMessagesOptions = {},
) {
  const toolCallIds = useMemo(() => buildToolCallIds(messages), [messages]);
  const permissionsByToolCallId = useStableValue(
    useMemo(() => buildPermissionsByToolCallId(messages), [messages]),
    messageMapsEqualByIdentity,
  );
  const childrenByParentToolCallId = useStableValue(
    useMemo(() => buildChildrenByParentToolCallId(messages), [messages]),
    messageListMapsEqual,
  );
  const subagentChildIds = useMemo(
    () => buildSubagentChildIds(childrenByParentToolCallId),
    [childrenByParentToolCallId],
  );
  const pendingClarification = useMemo(() => findPendingClarification(messages), [messages]);
  const pendingClarificationGroup = useMemo(
    () => findPendingClarificationGroup(messages),
    [messages],
  );

  const visibleMessages = useMemo(
    () => filterVisibleMessages(messages, toolCallIds, subagentChildIds),
    [messages, toolCallIds, subagentChildIds],
  );

  const taskDescriptionMessage: Message | null = useMemo(() => {
    return taskDescription && visibleMessages.length === 0
      ? {
          id: "task-description",
          task_id: toTaskId(taskId ?? ""),
          session_id: toSessionId(resolvedSessionId ?? ""),
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

  const todoItems = useMemo(() => buildTodoItems(visibleMessages), [visibleMessages]);

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

  const rawGroupedItems = useMemo<RenderItem[]>(() => {
    return buildGroupedItemsForHook({
      regularMessages,
      allSessionMessages: messages,
      resolvedSessionId,
      hasOlderMessages: options.hasOlderMessages ?? false,
      lastAgentError: options.lastAgentError,
    });
  }, [
    regularMessages,
    resolvedSessionId,
    messages,
    options.hasOlderMessages,
    options.lastAgentError,
  ]);
  const groupedItems = useStableGroupedItems(rawGroupedItems);

  useDebugProcessedPipeline({
    sessionId: resolvedSessionId,
    messages,
    visibleMessages,
    footerActionCount: footerActionMessages.length,
    groupedItems,
  });

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
    pendingClarificationGroup,
  };
}
