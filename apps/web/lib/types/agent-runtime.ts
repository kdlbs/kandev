export type AgentRuntimeAvailabilityStatus = "available" | "recovering" | "unavailable";

export type AgentRuntimeAvailabilityReason =
  | "agentctl_exited"
  | "ownership_unverified"
  | "start_failed"
  | "recovery_exhausted";

/**
 * Revision and boot fields are optional for older backend snapshots. New
 * backends always send them and clients use them to reject stale projections.
 */
export interface AgentRuntimeAvailability {
  status: AgentRuntimeAvailabilityStatus;
  reason?: AgentRuntimeAvailabilityReason;
  occurred_at?: string;
  boot_id?: string;
  revision?: number;
  runtime_epoch?: number;
  recovery_id?: string;
  retry_allowed?: boolean;
}

export interface AgentRuntimeRetryRequest {
  boot_id: string;
  runtime_epoch: number;
  revision: number;
  request_id: string;
}

export function newerAgentRuntimeSnapshot(
  current: AgentRuntimeAvailability | null,
  incoming: AgentRuntimeAvailability | null,
): AgentRuntimeAvailability | null {
  if (incoming === null) return current;
  if (current === null) return incoming;
  if (
    current.boot_id &&
    incoming.boot_id === current.boot_id &&
    current.revision !== undefined &&
    incoming.revision !== undefined &&
    incoming.revision <= current.revision
  ) {
    return current;
  }
  return incoming;
}
