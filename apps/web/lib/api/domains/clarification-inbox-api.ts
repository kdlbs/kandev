import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  ClarificationInboxHiddenPage,
  ClarificationInboxPage,
  ClarificationInboxSnoozeDuration,
} from "@/lib/types/clarification-inbox";

const BASE = "/api/v1/clarification-inbox";

// One canonical parameter set (design-02#Control-flow): every caller passes
// only workspaceId. No cursor -- v1 never forward-pages (F38).
export function listClarificationInbox(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<ClarificationInboxPage> {
  return fetchJson<ClarificationInboxPage>(
    `${BASE}?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

export function listHiddenClarificationInbox(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<ClarificationInboxHiddenPage> {
  return fetchJson<ClarificationInboxHiddenPage>(
    `${BASE}/hidden?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

export function dismissClarificationInboxBundle(
  pendingId: string,
  options?: ApiRequestOptions,
): Promise<void> {
  return fetchJson<void>(`${BASE}/sidecar/${encodeURIComponent(pendingId)}`, {
    ...options,
    init: {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ state: "dismissed" }),
      ...options?.init,
    },
  });
}

export function snoozeClarificationInboxBundle(
  pendingId: string,
  duration: ClarificationInboxSnoozeDuration,
  options?: ApiRequestOptions,
): Promise<void> {
  return fetchJson<void>(`${BASE}/sidecar/${encodeURIComponent(pendingId)}`, {
    ...options,
    init: {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ state: "snoozed", snooze_duration: duration }),
      ...options?.init,
    },
  });
}

export function restoreClarificationInboxBundle(
  pendingId: string,
  options?: ApiRequestOptions,
): Promise<void> {
  return fetchJson<void>(`${BASE}/sidecar/${encodeURIComponent(pendingId)}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}
