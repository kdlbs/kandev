import type { Message, TaskSessionState } from "@/lib/types/http";
import type { RenderItem, TurnGroup } from "@/hooks/use-processed-messages";
import {
  isRichOutputMessage,
  isSubagentMessage,
  kandevToolStemOf,
  type ToolCallMetadata,
} from "@/components/task/chat/types";
import { extractMcpResult, pickString } from "@/components/task/chat/messages/kandev/parse";
import { isTerminalToolCallStatus, normalizeToolCallStatus } from "@/lib/utils/tool-call-status";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";

const ACTIVITY_TYPES = new Set([
  "thinking",
  "tool_call",
  "tool_edit",
  "tool_read",
  "tool_execute",
  "tool_search",
]);

const PROPOSAL_TOOL_STEMS = new Set([
  "propose_task",
  "propose_resume",
  "propose_message",
  "propose_move",
]);

export type ActivityChipInfo = {
  count: number;
  failed: number;
  /** Whole seconds, at least 1; null when no message carries a valid timestamp. */
  durationSeconds: number | null;
};

export type RunningTurn = {
  /** Id of the turn the session is working on, or null when none is known. */
  turnId: string | null;
  running: boolean;
  /** True when the session is only running because a permission awaits a decision. */
  waiting: boolean;
  /** `tool_call_id`s whose permission request is awaiting a decision in the running turn. */
  awaiting: Set<string>;
};

function metaOf(message: Message): (ToolCallMetadata & { tool_call_id?: string }) | undefined {
  return message.metadata as (ToolCallMetadata & { tool_call_id?: string }) | undefined;
}

function isActivity(message: Message): boolean {
  return ACTIVITY_TYPES.has(message.type);
}

function isPendingPermission(message: Message): boolean {
  if (message.type !== "permission_request") return false;
  const status = (message.metadata as { status?: string } | undefined)?.status;
  return status === undefined || status === "pending";
}

/** The running turn id: the store's active id, else the turn of the latest
 *  loaded message that has one. Derived on every call, never stored. */
function resolveTurnId(
  messages: Message[],
  activeTurnId: string | null | undefined,
): string | null {
  if (activeTurnId) return activeTurnId;
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].turn_id) return messages[i].turn_id ?? null;
  }
  return null;
}

/** `messages` must be the full session list: the visible list drops the
 *  permission requests that merge into a tool row. */
export function deriveRunningTurn(args: {
  messages: Message[];
  sessionState: TaskSessionState | undefined;
  activeTurnId: string | null | undefined;
}): RunningTurn {
  const { messages, sessionState } = args;
  const live = sessionState === "RUNNING" || sessionState === "WAITING_FOR_INPUT";
  const turnId = live ? resolveTurnId(messages, args.activeTurnId) : null;
  const awaiting = new Set<string>();
  let pending = 0;
  if (live) {
    for (const message of messages) {
      if (!isPendingPermission(message)) continue;
      if (message.turn_id && message.turn_id !== turnId) continue;
      pending++;
      const id = metaOf(message)?.tool_call_id;
      if (id) awaiting.add(id);
    }
  }
  const waiting = sessionState === "WAITING_FOR_INPUT" && pending > 0;
  return { turnId, running: sessionState === "RUNNING" || waiting, waiting, awaiting };
}

function isProposal(message: Message): boolean {
  return message.type === "tool_call" && PROPOSAL_TOOL_STEMS.has(kandevToolStemOf(message) ?? "");
}

/** The condition `ProposeTaskRenderer` uses to show the card. */
function proposalCardReady(message: Message): boolean {
  const meta = metaOf(message);
  if (normalizeToolCallStatus(meta?.status) === "error") return false;
  const raw =
    meta?.normalized?.generic?.output ?? (meta as { result?: unknown } | undefined)?.result;
  return pickString(extractMcpResult(raw), "proposal_id") !== undefined;
}

function isChippable(message: Message): boolean {
  return (
    isActivity(message) &&
    Boolean(message.turn_id) &&
    !isSubagentMessage(message) &&
    !isRichOutputMessage(message) &&
    !isProposal(message)
  );
}

type Segment = { turnId: string; ordinal: number; messages: Message[] };

export function formatActivityDuration(totalSeconds: number): string {
  const s = Math.max(1, Math.floor(totalSeconds));
  if (s < 60) return `${s}s`; // i18n-exempt: unit abbreviation
  if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`; // i18n-exempt: unit abbreviation
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`; // i18n-exempt: unit abbreviation
}

function segmentDuration(messages: Message[]): number | null {
  let first: bigint | null = null;
  let last: bigint | null = null;
  for (const message of messages) {
    const ts = parseTurnTimestamp(message.created_at);
    if (ts === null) continue;
    if (first === null) first = ts;
    last = ts;
  }
  if (first === null || last === null) return null;
  const seconds = Number((last - first) / BigInt(1_000_000_000));
  return Math.max(1, seconds);
}

function chipInfo(messages: Message[]): ActivityChipInfo {
  const calls = messages.filter((m) => m.type !== "thinking");
  const failed = calls.filter((m) => normalizeToolCallStatus(metaOf(m)?.status) === "error").length;
  return { count: calls.length, failed, durationSeconds: segmentDuration(messages) };
}

type Slot = { kind: "item"; item: RenderItem } | { kind: "segment"; segment: Segment };

function flatten(items: RenderItem[]): Slot[] {
  const slots: Slot[] = [];
  for (const item of items) {
    if (item.type === "turn_group") {
      for (const message of item.messages)
        slots.push({ kind: "item", item: { type: "message", message } });
    } else {
      slots.push({ kind: "item", item });
    }
  }
  return slots;
}

function hiddenWhileRunning(message: Message, running: RunningTurn): boolean {
  if (!running.running || !running.turnId || message.turn_id !== running.turnId) return false;
  const toolCallId = metaOf(message)?.tool_call_id;
  if (toolCallId && running.awaiting.has(toolCallId)) return false;
  if (isSubagentMessage(message) || isRichOutputMessage(message)) return false;
  if (isProposal(message)) return !proposalCardReady(message);
  return true;
}

/** Replaces each turn's tool activity with one chip per segment once the turn
 *  ended, and hides the running turn's activity. The default grouping is
 *  flattened first, so it is left untouched for every other chat. */
export function buildActivityItems(items: RenderItem[], running: RunningTurn): RenderItem[] {
  const slots: Slot[] = [];
  const open = new Map<string, Segment>();
  const ordinals = new Map<string, number>();
  for (const slot of flatten(items)) {
    if (slot.kind !== "item" || slot.item.type !== "message") {
      slots.push(slot);
      continue;
    }
    const message = slot.item.message;
    if (message.author_type === "user") open.clear();
    if (!isActivity(message)) {
      slots.push(slot);
      continue;
    }
    if (hiddenWhileRunning(message, running)) continue;
    const toolCallId = metaOf(message)?.tool_call_id;
    const awaiting = toolCallId !== undefined && running.awaiting.has(toolCallId);
    if (awaiting || !isChippable(message)) {
      slots.push(slot);
      continue;
    }
    const turnId = message.turn_id as string;
    let segment = open.get(turnId);
    if (!segment) {
      const ordinal = ordinals.get(turnId) ?? 0;
      ordinals.set(turnId, ordinal + 1);
      segment = { turnId, ordinal, messages: [] };
      open.set(turnId, segment);
      slots.push({ kind: "segment", segment });
    }
    segment.messages.push(message);
  }
  return slots.flatMap((slot): RenderItem[] => {
    if (slot.kind === "item") return [slot.item];
    const { segment } = slot;
    const info = chipInfo(segment.messages);
    if (info.count === 0) return segment.messages.map((message) => ({ type: "message", message }));
    const group: TurnGroup = {
      type: "turn_group",
      id: `activity-chip-${segment.turnId}-${segment.ordinal}`,
      turnId: segment.turnId,
      messages: segment.messages,
      activityChip: info,
    };
    return [group];
  });
}

const VERB_KEYS: Record<string, string> = {
  list_tasks: "coordinator:activityVerbListTasks",
  get_task_conversation: "coordinator:activityVerbTaskConversation",
  list_workflows: "coordinator:activityVerbListWorkflows",
  list_workflow_steps: "coordinator:activityVerbListWorkflowSteps",
  list_repositories: "coordinator:activityVerbListRepositories",
  get_coordinator_item: "coordinator:activityVerbCoordinatorItem",
  propose_task: "coordinator:activityVerbProposeTask",
};
const VERB_WORKING = "coordinator:activityVerbWorking";
const VERB_WAITING = "coordinator:activityVerbWaiting";

export type ActivityStatusLine = {
  turnId: string | null;
  verbKey: string;
  /** Epoch ms of the running turn's first message; null when it has no valid timestamp. */
  startedAtMs: number | null;
};

function runningToolStem(messages: Message[], turnId: string | null): string | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const message = messages[i];
    if (turnId && message.turn_id !== turnId) continue;
    if (!isActivity(message) || message.type === "thinking") continue;
    if (isTerminalToolCallStatus(metaOf(message)?.status)) continue;
    return kandevToolStemOf(message) ?? "";
  }
  return null;
}

function turnStartMs(messages: Message[], turnId: string | null): number | null {
  if (!turnId) return null;
  const first = messages.find((message) => message.turn_id === turnId);
  const ts = first ? parseTurnTimestamp(first.created_at) : null;
  return ts === null ? null : Number(ts / BigInt(1_000_000));
}

/** Null while no turn runs or the session is starting. */
export function deriveStatusLine(
  messages: Message[],
  running: RunningTurn,
): ActivityStatusLine | null {
  if (!running.running) return null;
  let verbKey = VERB_WORKING;
  if (running.waiting) {
    verbKey = VERB_WAITING;
  } else {
    const stem = runningToolStem(messages, running.turnId);
    if (stem && VERB_KEYS[stem]) verbKey = VERB_KEYS[stem];
  }
  return { turnId: running.turnId, verbKey, startedAtMs: turnStartMs(messages, running.turnId) };
}
