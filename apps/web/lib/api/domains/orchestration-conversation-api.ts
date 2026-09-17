import { fetchJson } from "../client";
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
};
const path = (id: string) => `/api/v1/orchestration/tasks/${encodeURIComponent(id)}`;
export const getConversation = (id: string) => fetchJson<ConversationTask>(path(id));
export async function getConversationComments(id: string): Promise<TaskComment[]> {
  const result = await fetchJson<{ comments: CommentDTO[] }>(`${path(id)}/comments`);
  return (result.comments ?? []).map((row) => ({
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
  }));
}
export const postConversationComment = (id: string, body: { body: string }) =>
  fetchJson(`${path(id)}/comments`, { init: { method: "POST", body: JSON.stringify(body) } });

export const retryConversation = (
  id: string,
  sessionId: string,
  action: "resume" | "fresh_start",
) =>
  fetchJson(`${path(id)}/retry`, {
    init: { method: "POST", body: JSON.stringify({ session_id: sessionId, action }) },
  });
