import type { TaskComment, TaskSession } from "@/app/office/tasks/[id]/types";

/**
 * Per-comment turn context. The chat layer attaches a collapsible panel
 * under each comment scoped to the messages produced *between* the
 * previous comment and this one — i.e. the work the agent did to lead
 * up to this reply (for agent comments) or the conversation that
 * preceded the user's message (rare for office tasks where the agent
 * usually replies after the user).
 *
 * - `fromExclusive` is the previous comment's `createdAt` for any task,
 *   falling back to the session's `startedAt`. `null` means "from the
 *   beginning of the session".
 * - `toInclusive` is this comment's `createdAt`.
 */
export type CommentTurnContext = {
  sessionId: string;
  fromExclusive: string | null;
  toInclusive: string;
};

/**
 * Build one CommentTurnContext per comment, windowed to the messages
 * between the previous comment and this one. All comments (user + agent)
 * get a panel because every reply is the visible boundary of an agent
 * turn — the messages above it are the work that produced or preceded it.
 *
 * The session is the most-recent office session on the task; office tasks
 * always have a single (task, agent) session row that survives across
 * runs, so a single sessionId scopes every comment's window.
 */
export function buildCommentTurnContext(
  comments: TaskComment[],
  sessions: TaskSession[],
): Map<string, CommentTurnContext> {
  const ctx = new Map<string, CommentTurnContext>();
  if (comments.length === 0) return ctx;

  const officeSessions = sessions.filter((s) => s.agentProfileId);
  const candidates = officeSessions.length > 0 ? officeSessions : sessions;
  if (candidates.length === 0) return ctx;
  const session = [...candidates].sort((a, b) =>
    (b.startedAt ?? "").localeCompare(a.startedAt ?? ""),
  )[0];

  const sorted = [...comments].sort((a, b) => a.createdAt.localeCompare(b.createdAt));
  let prevAt: string | null = session.startedAt ?? null;
  for (const c of sorted) {
    ctx.set(c.id, {
      sessionId: session.id,
      fromExclusive: prevAt,
      toInclusive: c.createdAt,
    });
    prevAt = c.createdAt;
  }
  return ctx;
}
