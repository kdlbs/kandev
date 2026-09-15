import type { CommentTurnContext } from "./turn-context";
import { groupSortKey, type SessionGroup } from "./session-groups";
import { normalizeRemediationUrl } from "@/lib/remediation-url";
import {
  lastAgentErrorStamp,
  readLastAgentError,
  readLastAgentErrorIncludingDismissed,
} from "@/lib/session-last-agent-error";
import { isMatchingTaskLaunchError } from "@/components/task/chat/types";
import type {
  RunError,
  TaskComment,
  TaskDecision,
  TaskSession,
  TimelineEvent,
} from "@/app/office/tasks/[id]/types";

/**
 * Discriminated union of every row that can appear in the task chat
 * timeline. The `task-chat.tsx` renderer narrows on `kind`.
 */
export type ChatEntry =
  | {
      kind: "comment";
      data: TaskComment;
      sortKey: string;
      turn?: CommentTurnContext;
      hasLaterAgentReply?: boolean;
    }
  | { kind: "timeline"; data: TimelineEvent; sortKey: string }
  | { kind: "session"; data: SessionGroup; sortKey: string }
  | { kind: "decision"; data: TaskDecision; sortKey: string }
  | { kind: "error"; data: RunError; sortKey: string };

/**
 * For each user comment, returns true when at least one agent comment
 * exists with a strictly later createdAt. Used to suppress the run
 * status badge once the agent reply has landed.
 */
export function buildLaterAgentReplyMap(comments: TaskComment[]): Map<string, boolean> {
  const map = new Map<string, boolean>();
  // Cheap O(n*m) loop — typical chat threads have a small handful of
  // comments. If this grows, sort once and binary-search instead.
  for (const c of comments) {
    if (c.authorType !== "user") continue;
    const hasReply = comments.some(
      (other) => other.authorType === "agent" && other.createdAt > c.createdAt,
    );
    map.set(c.id, hasReply);
  }
  return map;
}

/**
 * Pull retained session failures into the Office chat timeline. The session
 * metadata remains the source of the chronological breadcrumb after recovery;
 * only an undismissed record carries recovery actions. The remediation URL is
 * copied from the persisted metadata and passed through the browser-edge
 * allowlist, so RunError never carries an unvalidated string.
 */
export function buildRunErrorsFromSessions(sessions: TaskSession[]): RunError[] {
  const errors: RunError[] = [];
  for (const s of sessions) {
    const error = buildRunErrorFromSession(s);
    if (error) errors.push(error);
  }
  return errors;
}

function buildRunErrorFromSession(s: TaskSession): RunError | null {
  const parsedLastError = readLastAgentErrorIncludingDismissed(s.metadata);
  const isLegacy = parsedLastError === null;
  // Older Office sessions only stored error_message on the session row. Keep
  // those FAILED rows in the chronological timeline while using metadata as
  // the richer identity for resumed and dismissed failures.
  if (isLegacy && s.state !== "FAILED") return null;

  const activeLastError = readLastAgentError(s.metadata);
  const error: RunError = {
    id: `re-${s.id}`,
    sessionId: s.id,
    agentProfileId: s.agentProfileId,
    rawPayload: s.errorMessage ?? "",
    failedAt: parsedLastError?.occurredAt ?? s.completedAt ?? s.updatedAt ?? s.startedAt ?? "",
    recoveryActions: activeLastError?.recoveryActions,
    isActive: activeLastError !== null || isLegacy,
  };
  if (isLegacy) return error;

  error.failureCode = parsedLastError.code;
  error.failureDetails = parsedLastError.details;
  error.message = parsedLastError.message;
  error.taskRepositoryId = parsedLastError.taskRepositoryId;
  error.errorStamp = lastAgentErrorStamp(parsedLastError);
  const remediationUrl = normalizeRemediationUrl(parsedLastError.remediationUrl);
  if (remediationUrl) error.remediationUrl = remediationUrl;
  return error;
}

export function hasMatchingSessionLaunchError(
  summarySessionId: string | undefined,
  summaryStamp: string | undefined,
  runErrors: RunError[],
): boolean {
  if (!summarySessionId) return false;
  return runErrors.some((error) =>
    isMatchingTaskLaunchError(
      { session_id: summarySessionId, stamp: summaryStamp ?? "" },
      { sessionId: error.sessionId, errorStamp: error.errorStamp },
    ),
  );
}

export function mergeLiveSessionMetadata(
  base: Record<string, unknown> | null | undefined,
  live: Record<string, unknown> | null | undefined,
): Record<string, unknown> | null | undefined {
  // Tri-state: `live` is undefined when the store has no metadata update for
  // this session (keep the initial fetch), and explicit null is preserved so
  // a server-side metadata clear cannot resurrect stale initial metadata.
  return live === undefined ? base : live;
}

/**
 * Live metadata for one session from the taskSessions store. Returns
 * undefined both when no store row exists and when an existing row carries no
 * `metadata` field (a partial row) — in both cases the initial REST fetch
 * stays authoritative. Explicit `null` is preserved because the server uses
 * it to clear metadata.
 */
export function liveSessionMetadataFromStore(
  items: Record<string, { metadata?: Record<string, unknown> | null } | undefined>,
  sessionId: string,
): Record<string, unknown> | null | undefined {
  const row = items[sessionId];
  if (row === undefined || row.metadata === undefined) {
    return undefined;
  }
  return row.metadata;
}

export type MergeChatEntriesArgs = {
  comments: TaskComment[];
  timeline: TimelineEvent[];
  groups: SessionGroup[];
  decisions?: TaskDecision[];
  turnCtx?: Map<string, CommentTurnContext>;
  runErrors?: RunError[];
  laterAgentReplyMap?: Map<string, boolean>;
};

export function mergeChatEntries({
  comments,
  timeline,
  groups,
  decisions = [],
  turnCtx,
  runErrors = [],
  laterAgentReplyMap,
}: MergeChatEntriesArgs): ChatEntry[] {
  const entries: ChatEntry[] = [
    ...comments.map((c) => ({
      kind: "comment" as const,
      data: c,
      sortKey: c.createdAt,
      turn: turnCtx?.get(c.id),
      hasLaterAgentReply: laterAgentReplyMap?.get(c.id),
    })),
    ...timeline.map((t) => ({ kind: "timeline" as const, data: t, sortKey: t.at })),
    ...groups.map((g) => ({
      kind: "session" as const,
      data: g,
      sortKey: groupSortKey(g),
    })),
    ...decisions.map((d) => ({
      kind: "decision" as const,
      data: d,
      sortKey: d.createdAt,
    })),
    ...runErrors.map((e) => ({
      kind: "error" as const,
      data: e,
      sortKey: e.failedAt,
    })),
  ];
  return entries.sort((a, b) => a.sortKey.localeCompare(b.sortKey));
}
