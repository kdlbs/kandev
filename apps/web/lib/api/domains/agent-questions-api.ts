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
  params: { status?: "open" | "history"; limit?: number; cursor?: string } = {},
  options?: ApiRequestOptions,
): Promise<AgentQuestionPage> {
  const search = new URLSearchParams({ workspace_id: workspaceId });
  if (params.status) search.set("status", params.status);
  if (params.limit) search.set("limit", String(params.limit));
  if (params.cursor) search.set("cursor", params.cursor);
  return fetchJson<AgentQuestionPage>(`${BASE}?${search.toString()}`, options);
}
