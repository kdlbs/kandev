import { fetchJson, type ApiRequestOptions } from "../client";
import type { FailedInboxPage } from "@/lib/types/failed-inbox";

const BASE = "/api/v1/failed-inbox";

// One canonical parameter set (design-01#Control-flow): every caller passes
// only workspaceId. No cursor -- this read serves exactly one bounded page.
export function listFailedInbox(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<FailedInboxPage> {
  return fetchJson<FailedInboxPage>(
    `${BASE}?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}
