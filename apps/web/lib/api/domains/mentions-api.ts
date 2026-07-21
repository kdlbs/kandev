import { fetchJson, type ApiRequestOptions } from "../client";
import type { EntityReferenceSearchResponse } from "@/lib/types/entity-reference";

export type SearchEntityReferencesRequest = {
  workspaceId: string;
  query: string;
  limit?: number;
  excludeTaskId?: string;
};

export async function searchEntityReferences(
  request: SearchEntityReferencesRequest,
  options?: ApiRequestOptions,
): Promise<EntityReferenceSearchResponse> {
  const query = new URLSearchParams({ q: request.query });
  if (request.limit !== undefined) query.set("limit", String(request.limit));
  if (request.excludeTaskId) query.set("exclude_task_id", request.excludeTaskId);
  const workspaceId = encodeURIComponent(request.workspaceId);
  return fetchJson<EntityReferenceSearchResponse>(
    `/api/v1/workspaces/${workspaceId}/mentions/search?${query.toString()}`,
    options,
  );
}
