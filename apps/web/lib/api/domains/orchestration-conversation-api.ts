import { fetchJson } from "../client";
import { generateUUID } from "@/lib/utils";
import type { CommentTransport } from "@/components/task/simple/comment-transport";
import type { TaskComment } from "@/components/task/simple/types";
export type ConversationTask = {
  id: string;
  title: string;
  workspace_id: string;
  orchestrator_id: string;
};
type CommentDTO = {
  id: string;
  task_id: string;
  author_id: string;
  author_type: "user" | "agent";
  run_id?: string;
  run_status?: TaskComment["runStatus"];
  run_error?: string;
  body: string;
  created_at: string;
  source?: string;
  client_message_id?: string;
  receipt_status?: string;
  intent_revision?: number;
  sequence?: number;
};
const path = (id: string) => `/api/v1/orchestration/tasks/${encodeURIComponent(id)}`;
export const getConversation = (id: string) => fetchJson<ConversationTask>(path(id));
function mapComment(row: CommentDTO): TaskComment {
  return {
    id: row.id,
    taskId: row.task_id,
    authorId: row.author_id,
    authorType: row.author_type,
    runId: row.run_id,
    runStatus: row.run_status,
    runError: row.run_error,
    authorName: "",
    content: row.body,
    createdAt: row.created_at,
    source: row.source,
    clientMessageId: row.client_message_id,
    receiptStatus: row.receipt_status,
    intentRevision: row.intent_revision,
    sequence: row.sequence,
  };
}
export async function getConversationComments(id: string): Promise<TaskComment[]> {
  const result = await fetchJson<{ comments: CommentDTO[] }>(`${path(id)}/comments`);
  return (result.comments ?? []).map(mapComment);
}
export async function getConversationCommentPage(id: string, before = "", signal?: AbortSignal) {
  const query = new URLSearchParams({ before, limit: "50" });
  const result = await fetchJson<{ comments: CommentDTO[]; next_cursor: string }>(
    `${path(id)}/comments?${query}`,
    { init: { signal } },
  );
  return {
    comments: (result.comments ?? []).map(mapComment),
    next_cursor: result.next_cursor ?? "",
  };
}
export function createConversationSender(conversationId: string): CommentTransport {
  let pending: { body: string; id: string } | undefined;
  let inflight: Promise<unknown> | undefined;
  return async (id, body) => {
    if (id !== conversationId) throw new Error("Conversation identity changed");
    if (inflight) {
      if (pending?.body !== body.body) throw new Error("Another message is awaiting a receipt");
      return inflight;
    }
    if (!pending || pending.body !== body.body) pending = { body: body.body, id: generateUUID() };
    const attempt = pending;
    inflight = postConversationComment(id, { body: attempt.body, client_message_id: attempt.id });
    try {
      const result = await inflight;
      if (pending === attempt) pending = undefined;
      return result;
    } finally {
      inflight = undefined;
    }
  };
}
export const postConversationComment = (
  id: string,
  body: { body: string; client_message_id?: string },
) => fetchJson(`${path(id)}/comments`, { init: { method: "POST", body: JSON.stringify(body) } });

export const retryConversation = (
  id: string,
  sessionId: string,
  action: "resume" | "fresh_start",
) =>
  fetchJson(`${path(id)}/retry`, {
    init: { method: "POST", body: JSON.stringify({ session_id: sessionId, action }) },
  });
