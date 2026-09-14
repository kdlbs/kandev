import { fetchJson, type ApiRequestOptions } from "../client";
import type { InboxHistoryPage } from "@/lib/types/inbox-history";

const BASE = "/api/v1/clarification-inbox/history";

// The single read the History tab issues (AC .14/.15): no cursor forwarding
// beyond what the caller explicitly requests, no mutating call anywhere in
// this module.
export function listInboxHistory(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<InboxHistoryPage> {
  return fetchJson<InboxHistoryPage>(
    `${BASE}?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}
