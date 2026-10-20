import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  ClarificationInboxEarlierPage,
  ClarificationInboxHiddenPage,
  ClarificationInboxPage,
  ClarificationInboxSnoozeDuration,
  ClarificationInboxUnansweredPage,
  InboxRecoveryBlockPage,
} from "@/lib/types/clarification-inbox";

const BASE = "/api/v1/clarification-inbox";

// One canonical parameter set (design-02#Control-flow): every caller passes
// only workspaceId. No cursor -- v1 never forward-pages.
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

export function listEarlierTurnClarificationInbox(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<ClarificationInboxEarlierPage> {
  return fetchJson<ClarificationInboxEarlierPage>(
    `${BASE}/earlier-turns?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

export function listInboxRecoveryBlocks(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<InboxRecoveryBlockPage> {
  return fetchJson<InboxRecoveryBlockPage>(
    `${BASE}/recovery-blocks?workspace_id=${encodeURIComponent(workspaceId)}`,
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

export function listUnansweredClarificationInbox(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<ClarificationInboxUnansweredPage> {
  return fetchJson<ClarificationInboxUnansweredPage>(
    `${BASE}/unanswered?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

export function closeUnansweredClarificationInboxBundle(
  pendingId: string,
  options?: ApiRequestOptions,
): Promise<void> {
  return fetchJson<void>(`${BASE}/unanswered/${encodeURIComponent(pendingId)}/close`, {
    ...options,
    init: { method: "POST", ...options?.init },
  });
}

/** Records that an admitted late-answer message answered this unanswered question. */
export function markClarificationInboxLateAnswer(
  pendingId: string,
  admission: { messageId: string; sessionId: string },
  options?: ApiRequestOptions,
): Promise<void> {
  return fetchJson<void>(`${BASE}/unanswered/${encodeURIComponent(pendingId)}/late-answer`, {
    ...options,
    init: {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message_id: admission.messageId, session_id: admission.sessionId }),
      ...options?.init,
    },
  });
}
