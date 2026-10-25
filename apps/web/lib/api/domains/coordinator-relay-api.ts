import { ApiError, fetchJson, type ApiRequestOptions } from "@/lib/api/client";
import type { Message } from "@/lib/types/http";

// Mirrors internal/coordinator/relay.go's RelayClarification.
export type RelayClarification = {
  pending_id: string;
  context: string;
  messages: Message[];
};

// Mirrors internal/coordinator/relay.go's RelayPermission.
export type RelayPermission = {
  message: Message;
};

// Mirrors internal/coordinator/relay.go's RelayResult.
export type Relay = {
  task_id: string;
  session_id: string;
  clarification: RelayClarification | null;
  permission: RelayPermission | null;
};

/**
 * `refused` is the endpoint saying the viewer may not read this task's relay
 * (403 or 404); every other non-200 is `failed`.
 */
export type RelayRead = { kind: "ok"; relay: Relay } | { kind: "refused" } | { kind: "failed" };

export async function readRelay(
  workspaceId: string,
  coordinatorId: string,
  taskId: string,
  options?: ApiRequestOptions,
): Promise<RelayRead> {
  const path = `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/coordinators/${encodeURIComponent(
    coordinatorId,
  )}/relay/${encodeURIComponent(taskId)}`;
  try {
    return { kind: "ok", relay: await fetchJson<Relay>(path, options) };
  } catch (error) {
    if (error instanceof ApiError && (error.status === 403 || error.status === 404)) {
      return { kind: "refused" };
    }
    return { kind: "failed" };
  }
}
