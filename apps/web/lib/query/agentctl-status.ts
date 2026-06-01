/**
 * Shared read/write helpers for the agentctl (workspace controller) status TQ
 * cache (`qk.session.agentctl(sessionId)`).
 *
 * The status is event-driven server state: the session-state bridge writes it
 * from session.agentctl_starting / _ready / _error, and a few imperative paths
 * (session resumption, office task ensure, the session.state_changed
 * ready-promotion fallback) write it directly. All of them go through these
 * helpers so the cache shape stays consistent and observe-only readers
 * (`sessionAgentctlQueryOptions`) see the same data.
 */

import type { QueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { SessionAgentctlStatus } from "@/lib/query/query-options/session-runtime";

/** Read the current agentctl status for a session (null if none recorded). */
export function readAgentctlStatus(
  qc: QueryClient,
  sessionId: string,
): SessionAgentctlStatus | null {
  return qc.getQueryData<SessionAgentctlStatus | null>(qk.session.agentctl(sessionId)) ?? null;
}

/** Write the agentctl status for a session into the TQ cache. */
export function writeAgentctlStatus(
  qc: QueryClient,
  sessionId: string,
  status: SessionAgentctlStatus,
): void {
  qc.setQueryData<SessionAgentctlStatus | null>(qk.session.agentctl(sessionId), status);
}
