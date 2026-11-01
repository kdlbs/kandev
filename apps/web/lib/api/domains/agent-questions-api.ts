import { fetchJson, type ApiRequestOptions } from "../client";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";

const BASE = "/api/v1/agent-questions";

export type AgentQuestionAnswer = {
  question_id: string;
  selected_options: string[];
  custom_text?: string;
};

export type RespondToAgentQuestionBody = {
  answers?: AgentQuestionAnswer[];
  rejected?: boolean;
  reject_reason?: string;
  submission_id?: string;
};

export type RespondToAgentQuestionResult = {
  success: boolean;
  won: boolean;
  status: string;
  questions: ClarificationInboxAsyncQuestion[];
};

export type AgentQuestionPage = {
  questions: ClarificationInboxAsyncQuestion[];
  count: number;
  has_more: boolean;
  next_cursor?: string;
};

const questionReads = new Map<string, Promise<ClarificationInboxAsyncQuestion>>();
const questionRefreshes = new Map<string, Promise<ClarificationInboxAsyncQuestion>>();

export function getAgentQuestion(questionId: string): Promise<ClarificationInboxAsyncQuestion> {
  const existing = questionReads.get(questionId);
  if (existing) return existing;
  const request = fetchJson<ClarificationInboxAsyncQuestion>(
    `${BASE}/${encodeURIComponent(questionId)}`,
  );
  const tracked = request.finally(() => {
    if (questionReads.get(questionId) === tracked) questionReads.delete(questionId);
  });
  questionReads.set(questionId, tracked);
  return tracked;
}

export function refreshAgentQuestion(questionId: string): Promise<ClarificationInboxAsyncQuestion> {
  const existing = questionRefreshes.get(questionId);
  if (existing) return existing;
  const currentRead = questionReads.get(questionId);
  const request = (async () => {
    if (currentRead) await currentRead.catch(() => undefined);
    return getAgentQuestion(questionId);
  })();
  const tracked = request.finally(() => {
    if (questionRefreshes.get(questionId) === tracked) questionRefreshes.delete(questionId);
  });
  questionRefreshes.set(questionId, tracked);
  return tracked;
}

export function respondToAgentQuestion(
  questionId: string,
  body: RespondToAgentQuestionBody,
  options?: ApiRequestOptions,
): Promise<RespondToAgentQuestionResult> {
  return fetchJson<RespondToAgentQuestionResult>(
    `${BASE}/${encodeURIComponent(questionId)}/respond`,
    {
      ...options,
      init: {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
        ...options?.init,
      },
    },
  );
}

export function listAgentQuestions(
  workspaceId: string,
  params: { status?: "open" | "history"; taskId?: string; limit?: number; cursor?: string } = {},
  options?: ApiRequestOptions,
): Promise<AgentQuestionPage> {
  const search = new URLSearchParams({ workspace_id: workspaceId });
  if (params.status) search.set("status", params.status);
  if (params.taskId) search.set("task_id", params.taskId);
  if (params.limit) search.set("limit", String(params.limit));
  if (params.cursor) search.set("cursor", params.cursor);
  return fetchJson<AgentQuestionPage>(`${BASE}?${search.toString()}`, options);
}
