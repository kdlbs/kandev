import { fetchJson, type ApiRequestOptions } from "../client";
import type { ConversationSearchResponse } from "@/lib/types/conversation-search";

export function searchTaskConversations(
  workspaceId: string,
  query: string,
  limit: number,
  options?: ApiRequestOptions,
) {
  const params = new URLSearchParams({ query, limit: String(limit) });
  return fetchJson<ConversationSearchResponse>(
    `/api/v1/workspaces/${workspaceId}/tasks/conversations?${params}`,
    options,
  );
}
