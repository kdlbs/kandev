import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

// Minimal, store-independent shapes classify() needs. Deliberately narrower
// than KanbanTask/Task/Stall/Proposal so this module has no dependency on
// the store or the coordinator API client (docs/specs/coordinator/
// system-design/needs-you.md#classification).

export type AttentionActiveError = {
  preview?: string;
};

export type AttentionTaskStatusSummary = {
  last_activity_at?: string;
  primary_session?: { id: string; state?: string } | null;
  pending_action?: "clarification" | "permission";
  active_error?: AttentionActiveError | null;
  task_error?: AttentionActiveError | null;
  pull_request?: { aggregate_state?: string; state?: string } | null;
};

export type AttentionTask = {
  id: string;
  title: string;
  identifier?: string;
  state?: string;
  workflowStepId?: string;
  workflowId?: string | null;
  isArchived?: boolean;
  updatedAt?: string;
  statusSummary?: AttentionTaskStatusSummary | null;
};

export type AttentionStall = {
  task_id: string;
  stalled_for_ms: number;
  last_event_at: string;
  detected_at: string;
};

export type AttentionProposalSpec = {
  title: string;
  description: string;
  rationale: string;
  workflow_id: string;
  step_id: string;
  repository_id: string;
  source_task_id: string;
};

export type AttentionProposal = {
  id: string;
  status: string;
  task_id: string | null;
  spec: AttentionProposalSpec;
  created_at: string;
};

export type NeedsYouItemKind = "proposal" | "question" | "stall" | "error";

type NeedsYouItemBase = {
  id: string;
  referenceTimeMs: number | undefined;
  ageMs: number | undefined;
};

export type NeedsYouProposalItem = NeedsYouItemBase & {
  kind: "proposal";
  proposal: AttentionProposal;
};

export type NeedsYouQuestionItem = NeedsYouItemBase & {
  kind: "question";
  task: AttentionTask;
  pendingAction: "clarification" | "permission";
};

export type NeedsYouStallItem = NeedsYouItemBase & {
  kind: "stall";
  task: AttentionTask;
  stall: AttentionStall;
};

export type NeedsYouErrorItem = NeedsYouItemBase & {
  kind: "error";
  task: AttentionTask;
  activeError: AttentionActiveError | null;
  taskError: AttentionActiveError | null;
};

export type NeedsYouItem =
  | NeedsYouProposalItem
  | NeedsYouQuestionItem
  | NeedsYouStallItem
  | NeedsYouErrorItem;

export type QueueGroupKind = "ready_to_merge" | "in_review" | "working" | "done" | "other";

export type QueueItem = {
  group: QueueGroupKind;
  id: string;
  task: AttentionTask;
  lastActivityAtMs: number | undefined;
  ageMs: number | undefined;
  /** Set when the task's session shape could not be read (AC-004.3). Only ever set on "other". */
  sessionUnreadable?: boolean;
};

export type ClassifyResult = {
  needsYou: NeedsYouItem[];
  queue: Record<QueueGroupKind, QueueItem[]>;
};

const QUEUE_GROUP_KINDS: QueueGroupKind[] = [
  "ready_to_merge",
  "in_review",
  "working",
  "done",
  "other",
];

const NEEDS_YOU_KIND_RANK: Record<NeedsYouItemKind, number> = {
  proposal: 0,
  question: 1,
  stall: 2,
  error: 3,
};

const RUNNING_SESSION_STATES = new Set(["RUNNING", "STARTING"]);

/**
 * Proposal statuses that still need a manager's decision and therefore stay
 * on Needs you (AC-COORDINATOR-NEEDS-YOU-001.1): `pending` awaits Approve,
 * Edit or Reject; `approving` is mid-decision with edits locked; `failed`
 * kept all three actions after a create failure. `approved`/`rejected` are
 * settled and never classify.
 */
const OPEN_PROPOSAL_STATUSES = new Set(["pending", "approving", "failed"]);

/** Whether a primary session state counts as "an agent is running" (AC-COORDINATOR-NEEDS-YOU-002.6). */
export function isRunningSessionState(state: string | undefined): boolean {
  return state !== undefined && RUNNING_SESSION_STATES.has(state);
}

const NANOS_PER_MS = BigInt(1_000_000);

function toEpochMs(value: string | undefined): number | undefined {
  const ns = parseStrictRfc3339Timestamp(value);
  if (ns === null) return undefined;
  return Number(ns / NANOS_PER_MS);
}

function compareIds(a: string, b: string): number {
  if (a < b) return -1;
  if (a > b) return 1;
  return 0;
}

function toMs(now: Date | number): number {
  return typeof now === "number" ? now : now.getTime();
}

function ageFrom(referenceTimeMs: number | undefined, nowMs: number): number | undefined {
  return referenceTimeMs === undefined ? undefined : nowMs - referenceTimeMs;
}

/**
 * A session is unreadable when statusSummary itself is absent, or when it is
 * present but primary_session is a non-null object with no state
 * (needs-you.md#classification). Question/error/PR/Working rules never match
 * an unreadable session; Done and the stall rule are unaffected by it.
 */
function isSessionUnreadable(summary: AttentionTaskStatusSummary | null | undefined): boolean {
  if (!summary) return true;
  const session = summary.primary_session;
  return session != null && !session.state;
}

function matchingStall(task: AttentionTask, stallsByTaskId: Map<string, AttentionStall>) {
  const stall = stallsByTaskId.get(task.id);
  if (!stall) return undefined;
  const lastActivityAtMs = toEpochMs(task.statusSummary?.last_activity_at);
  const detectedAtMs = toEpochMs(stall.detected_at);
  if (lastActivityAtMs !== undefined && detectedAtMs !== undefined) {
    if (lastActivityAtMs > detectedAtMs) return undefined;
  }
  return stall;
}

function questionItem(
  task: AttentionTask,
  summary: AttentionTaskStatusSummary,
  nowMs: number,
): NeedsYouQuestionItem | undefined {
  if (!summary.pending_action) return undefined;
  const referenceTimeMs = toEpochMs(summary.last_activity_at) ?? toEpochMs(task.updatedAt);
  return {
    kind: "question",
    id: task.id,
    task,
    pendingAction: summary.pending_action,
    referenceTimeMs,
    ageMs: ageFrom(referenceTimeMs, nowMs),
  };
}

function stallItem(
  task: AttentionTask,
  stallsByTaskId: Map<string, AttentionStall>,
  nowMs: number,
): NeedsYouStallItem | undefined {
  const stall = matchingStall(task, stallsByTaskId);
  if (!stall) return undefined;
  const referenceTimeMs = toEpochMs(stall.last_event_at);
  return {
    kind: "stall",
    id: task.id,
    task,
    stall,
    referenceTimeMs,
    ageMs: ageFrom(referenceTimeMs, nowMs),
  };
}

function errorItem(
  task: AttentionTask,
  summary: AttentionTaskStatusSummary,
  nowMs: number,
): NeedsYouErrorItem | undefined {
  if (!summary.active_error && !summary.task_error) return undefined;
  const referenceTimeMs = toEpochMs(summary.last_activity_at) ?? toEpochMs(task.updatedAt);
  return {
    kind: "error",
    id: task.id,
    task,
    activeError: summary.active_error ?? null,
    taskError: summary.task_error ?? null,
    referenceTimeMs,
    ageMs: ageFrom(referenceTimeMs, nowMs),
  };
}

function needsYouItemForTask(
  task: AttentionTask,
  summary: AttentionTaskStatusSummary | null | undefined,
  unreadable: boolean,
  stallsByTaskId: Map<string, AttentionStall>,
  nowMs: number,
): NeedsYouItem | undefined {
  if (!unreadable && summary) {
    const question = questionItem(task, summary, nowMs);
    if (question) return question;
  }

  const stall = stallItem(task, stallsByTaskId, nowMs);
  if (stall) return stall;

  if (!unreadable && summary) {
    const error = errorItem(task, summary, nowMs);
    if (error) return error;
  }

  return undefined;
}

function queueGroupForTask(
  task: AttentionTask,
  summary: AttentionTaskStatusSummary | null | undefined,
  unreadable: boolean,
): QueueGroupKind {
  if (!unreadable && summary?.pull_request?.aggregate_state === "ready") return "ready_to_merge";
  if (!unreadable && summary?.pull_request?.aggregate_state === "awaiting_review")
    return "in_review";
  const sessionState = summary?.primary_session?.state;
  if (!unreadable && sessionState !== undefined && RUNNING_SESSION_STATES.has(sessionState)) {
    return "working";
  }
  if (task.state === "COMPLETED") return "done";
  return "other";
}

function queueItemForTask(
  task: AttentionTask,
  summary: AttentionTaskStatusSummary | null | undefined,
  unreadable: boolean,
  nowMs: number,
): QueueItem {
  const group = queueGroupForTask(task, summary, unreadable);
  const lastActivityAtMs = toEpochMs(summary?.last_activity_at);
  return {
    group,
    id: task.id,
    task,
    lastActivityAtMs,
    ageMs: ageFrom(lastActivityAtMs, nowMs),
    ...(group === "other" ? { sessionUnreadable: unreadable } : {}),
  };
}

function classifyTask(
  task: AttentionTask,
  stallsByTaskId: Map<string, AttentionStall>,
  nowMs: number,
): NeedsYouItem | QueueItem {
  const summary = task.statusSummary;
  const unreadable = isSessionUnreadable(summary);

  const needsYouItem = needsYouItemForTask(task, summary, unreadable, stallsByTaskId, nowMs);
  if (needsYouItem) return needsYouItem;

  return queueItemForTask(task, summary, unreadable, nowMs);
}

function proposalItem(proposal: AttentionProposal, nowMs: number): NeedsYouProposalItem {
  const referenceTimeMs = toEpochMs(proposal.created_at);
  return {
    kind: "proposal",
    id: proposal.id,
    proposal,
    referenceTimeMs,
    ageMs: ageFrom(referenceTimeMs, nowMs),
  };
}

function sortNeedsYou(items: NeedsYouItem[]): NeedsYouItem[] {
  return [...items].sort((a, b) => {
    if (a.referenceTimeMs !== b.referenceTimeMs) {
      if (a.referenceTimeMs === undefined) return 1;
      if (b.referenceTimeMs === undefined) return -1;
      return a.referenceTimeMs - b.referenceTimeMs;
    }
    const rankDiff = NEEDS_YOU_KIND_RANK[a.kind] - NEEDS_YOU_KIND_RANK[b.kind];
    if (rankDiff !== 0) return rankDiff;
    return compareIds(a.id, b.id);
  });
}

function sortQueueGroup(items: QueueItem[]): QueueItem[] {
  return [...items].sort((a, b) => {
    if (a.lastActivityAtMs !== b.lastActivityAtMs) {
      if (a.lastActivityAtMs === undefined) return 1;
      if (b.lastActivityAtMs === undefined) return -1;
      return b.lastActivityAtMs - a.lastActivityAtMs;
    }
    return compareIds(a.id, b.id);
  });
}

/**
 * Classifies open tasks and pending proposals into the Needs you list and
 * Queue groups (docs/specs/coordinator/system-design/needs-you.md#classification,
 * docs/specs/coordinator/requirements/needs-you.md REQ-COORDINATOR-NEEDS-YOU-001).
 * Pure and store-independent: callers own reading tasks/stalls/proposals and
 * re-invoking this on a 30s timer to refresh ages.
 */
export function classify(
  tasks: AttentionTask[],
  stalls: AttentionStall[],
  proposals: AttentionProposal[],
  now: Date | number,
): ClassifyResult {
  const nowMs = toMs(now);
  const stallsByTaskId = new Map(stalls.map((stall) => [stall.task_id, stall]));

  const needsYou: NeedsYouItem[] = proposals
    .filter((proposal) => OPEN_PROPOSAL_STATUSES.has(proposal.status))
    .map((proposal) => proposalItem(proposal, nowMs));

  const queue: Record<QueueGroupKind, QueueItem[]> = {
    ready_to_merge: [],
    in_review: [],
    working: [],
    done: [],
    other: [],
  };

  for (const task of tasks) {
    if (task.isArchived) continue;
    const item = classifyTask(task, stallsByTaskId, nowMs);
    if ("kind" in item) {
      needsYou.push(item);
    } else {
      queue[item.group].push(item);
    }
  }

  for (const group of QUEUE_GROUP_KINDS) {
    queue[group] = sortQueueGroup(queue[group]);
  }

  return { needsYou: sortNeedsYou(needsYou), queue };
}
