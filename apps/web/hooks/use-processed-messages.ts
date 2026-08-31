import { useEffect, useMemo, useRef, useState } from "react";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type MessageType,
  type TaskPendingAction,
} from "@/lib/types/http";
import { isRichOutputMessage } from "@/components/task/chat/types";
import type { ToolCallMetadata } from "@/components/task/chat/types";
import {
  findPendingClarification,
  findPendingClarificationGroup,
  type PendingClarificationScope,
} from "@/lib/utils/pending-clarification";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import type { LastAgentError } from "@/lib/session-last-agent-error";
import {
  buildChildrenByParentToolCallId,
  buildPermissionsByToolCallId,
  buildSubagentChildIds,
  buildToolCallIds,
  filterVisibleMessages,
  isSetupScriptMessage,
} from "./processed-message-filtering";

export {
  collapseTodoSnapshotsPerTurn,
  deduplicateAgentBootResumes,
  isAgentBootResumeMessage,
  isSetupScriptMessage,
} from "./processed-message-filtering";

const debug = createDebugLogger("messages:process");
const PROCESSED_PIPELINE_DEBUG_INTERVAL_MS = 250;

function countByType(messages: Message[]): Record<string, number> {
  const out: Record<string, number> = {};
  for (const m of messages) {
    const t = m.type ?? "unknown";
    out[t] = (out[t] ?? 0) + 1;
  }
  return out;
}

type ProcessedPipelineDebugArgs = {
  sessionId: string | null;
  messages: Message[];
  visibleMessages: Message[];
  footerActionCount: number;
  groupedItems: RenderItem[];
};

function useDebugProcessedPipeline(args: ProcessedPipelineDebugArgs) {
  const latestArgsRef = useRef(args);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const emitLatestDebugSample = () => {
    const latest = latestArgsRef.current;
    debug("pipeline", {
      sessionId: latest.sessionId,
      input: { count: latest.messages.length, byType: countByType(latest.messages) },
      afterFilter: {
        count: latest.visibleMessages.length,
        byType: countByType(latest.visibleMessages),
      },
      droppedByFilter: latest.messages.length - latest.visibleMessages.length,
      footerActionCount: latest.footerActionCount,
      groupedItemKinds: latest.groupedItems.reduce<Record<string, number>>((acc, item) => {
        acc[item.type] = (acc[item.type] ?? 0) + 1;
        return acc;
      }, {}),
      turnGroupSizes: latest.groupedItems
        .filter((i): i is TurnGroup => i.type === "turn_group")
        .map((g) => ({ turnId: g.turnId, size: g.messages.length })),
    });
  };

  const flushPendingDebugSample = () => {
    if (timerRef.current === null) return;
    clearTimeout(timerRef.current);
    timerRef.current = null;
    emitLatestDebugSample();
  };

  useEffect(() => {
    if (latestArgsRef.current.sessionId !== args.sessionId) {
      flushPendingDebugSample();
    }
    latestArgsRef.current = args;
    if (!isDebug()) return;
    if (timerRef.current !== null) return;
    timerRef.current = setTimeout(() => {
      timerRef.current = null;
      emitLatestDebugSample();
    }, PROCESSED_PIPELINE_DEBUG_INTERVAL_MS);
  }, [args]);

  useEffect(
    () => () => {
      flushPendingDebugSample();
    },
    [],
  );
}

const ACTIVITY_MESSAGE_TYPES: Set<MessageType> = new Set([
  "thinking",
  "tool_call",
  "tool_edit",
  "tool_read",
  "tool_execute",
  "tool_search",
]);

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
  historyInitialized?: boolean;
  hasOlderMessages?: boolean;
  lastAgentError?: LastAgentError | null;
  currentTurnId?: string | null;
  pendingAction?: TaskPendingAction | null;
};

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

/** A subagent is another agent's turn, not a tool the agent used, so it never
 *  collapses into a turn group. Grouping one hid whole review waves behind a
 *  "4 tool calls, 2 subagents" row far up the transcript; hoisting it keeps the
 *  wave legible at rest while the surrounding tool calls still collapse. */
export function isSubagentMessage(message: Message): boolean {
  const metadata = message.metadata as ToolCallMetadata | undefined;
  return metadata?.normalized?.kind === "subagent_task";
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
    const isActivity =
      message.type &&
      ACTIVITY_MESSAGE_TYPES.has(message.type) &&
      !isSubagentMessage(message) &&
      !isRichOutputMessage(message);
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

/** Builds the todo checklist from the latest persisted `todo`-type message,
 *  accepting string and `{ text, done }` entries. Total by construction:
 *  malformed metadata yields an empty list, never throws. */
export function buildTodoItems(visibleMessages: Message[]) {
  // Scan backward from the latest message without allocating a reversed
  // copy: this runs on the todo-panel sync hot path for every message-array
  // update while the "only pin when not empty" sub-option is on.
  for (let i = visibleMessages.length - 1; i >= 0; i--) {
    const message = visibleMessages[i];
    const hasTodoMetadata =
      message.metadata !== null &&
      typeof message.metadata === "object" &&
      "todos" in message.metadata;
    if (message.type !== "todo" && !hasTodoMetadata) continue;
    const metadata = message.metadata;
    const todos =
      metadata !== null && typeof metadata === "object" && "todos" in metadata
        ? metadata.todos
        : undefined;
    // Total by construction: malformed persisted metadata (non-array `todos`,
    // null/primitive entries) must yield an empty list, never throw, so the
    // todos panel, chat processing, and the todo-panel sync hook can all
    // trust this call. Matches the spec's "malformed todo message falls back
    // to an empty state" behavior. The latest todo-carrying message wins even
    // when its payload is malformed.
    if (!Array.isArray(todos)) return [];
    return todos
      .map((item) => (typeof item === "string" ? { text: item, done: false } : item))
      .filter(
        (item): item is { text: string; done?: boolean } =>
          typeof item === "object" &&
          item !== null &&
          typeof item.text === "string" &&
          item.text.length > 0,
      );
  }
  return [];
}

function usePendingClarificationState(messages: Message[], options: ProcessedMessagesOptions) {
  const hasScopeKeys = "currentTurnId" in options || "pendingAction" in options;
  const scope = useMemo(
    () =>
      hasScopeKeys
        ? { currentTurnId: options.currentTurnId, pendingAction: options.pendingAction }
        : undefined,
    [hasScopeKeys, options.currentTurnId, options.pendingAction],
  );
  return {
    scope,
    pendingClarification: useMemo(
      () => findPendingClarification(messages, scope),
      [messages, scope],
    ),
    pendingClarificationGroup: useMemo(
      () => findPendingClarificationGroup(messages, scope),
      [messages, scope],
    ),
  };
}

function useVisibleMessages(
  messages: Message[],
  toolCallIds: Set<string>,
  subagentChildIds: Set<string>,
  scope: PendingClarificationScope | undefined,
) {
  return useMemo(
    () => filterVisibleMessages(messages, toolCallIds, subagentChildIds, scope),
    [messages, toolCallIds, subagentChildIds, scope],
  );
}

export function shouldShowTaskDescriptionFallback(
  taskDescription: string | null,
  visibleMessages: Message[],
  options: Pick<ProcessedMessagesOptions, "historyInitialized" | "hasOlderMessages"> = {},
): boolean {
  return (
    Boolean(taskDescription) &&
    options.historyInitialized === true &&
    options.hasOlderMessages === false &&
    !visibleMessages.some((message) => message.author_type === "user")
  );
}

export const TASK_DESCRIPTION_SYNTHETIC_ID = "task-description";

function buildTaskDescriptionMessage(
  showFallback: boolean,
  taskDescription: string | null,
  taskId: string | null,
  resolvedSessionId: string | null,
): Message | null {
  if (!showFallback) return null;
  return {
    id: TASK_DESCRIPTION_SYNTHETIC_ID,
    task_id: toTaskId(taskId ?? ""),
    session_id: toSessionId(resolvedSessionId ?? ""),
    author_type: "user",
    content: taskDescription ?? "",
    type: "message",
    created_at: "",
  };
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
  const { scope, pendingClarification, pendingClarificationGroup } = usePendingClarificationState(
    messages,
    options,
  );

  const visibleMessages = useVisibleMessages(messages, toolCallIds, subagentChildIds, scope);

  const showTaskDescriptionFallback = useMemo(
    () => shouldShowTaskDescriptionFallback(taskDescription, visibleMessages, options),
    [taskDescription, visibleMessages, options.historyInitialized, options.hasOlderMessages],
  );
  const taskDescriptionMessage: Message | null = useMemo(
    () =>
      buildTaskDescriptionMessage(
        showTaskDescriptionFallback,
        taskDescription,
        taskId,
        resolvedSessionId,
      ),
    [showTaskDescriptionFallback, taskDescription, taskId, resolvedSessionId],
  );

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
