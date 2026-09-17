import { fetchJson, type ApiRequestOptions } from "../client";
import type { InboxHistoryPage } from "@/lib/types/inbox-history";

const BASE = "/api/v1/clarification-inbox/history";

export type ListInboxHistoryOptions = ApiRequestOptions & {
  cursor?: string;
};

// The read-only History endpoint. Cursor forwarding is explicit, and this
// module has no mutating call.
export function listInboxHistory(
  workspaceId: string,
  options?: ListInboxHistoryOptions,
): Promise<InboxHistoryPage> {
  const { cursor, ...requestOptions } = options ?? {};
  const query = new URLSearchParams({ workspace_id: workspaceId });
  if (cursor) query.set("cursor", cursor);
  return fetchJson<InboxHistoryPage>(`${BASE}?${query.toString()}`, requestOptions);
}
