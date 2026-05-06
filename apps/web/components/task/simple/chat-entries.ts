import type { CommentTurnContext } from "./turn-context";
import { groupSortKey, type SessionGroup } from "./session-groups";
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
 * Pull RunError entries out of failed sessions. One entry per office
 * session in FAILED state — the session row's error_message becomes
 * the rawPayload for the chat entry's Show details.
 */
export function buildRunErrorsFromSessions(sessions: TaskSession[]): RunError[] {
  const errors: RunError[] = [];
  for (const s of sessions) {
    if (s.state !== "FAILED") continue;
    const failedAt = s.completedAt ?? s.updatedAt ?? s.startedAt ?? "";
    errors.push({
      id: `re-${s.id}`,
      sessionId: s.id,
      agentProfileId: s.agentProfileId,
      rawPayload: s.errorMessage ?? "",
      failedAt,
    });
  }
  return errors;
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
