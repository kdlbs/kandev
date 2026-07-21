import type { TaskSession } from "@/lib/types/http";

type PassthroughSessionInput = Pick<TaskSession, "is_passthrough" | "agent_profile_snapshot">;

/**
 * Whether a session should render the passthrough (PTY/xterm) UI instead of chat.
 *
 * `is_passthrough` is authoritative — it matches backend routing in
 * `StartAgentProcess` and what desktop layouts read directly. The snapshot
 * fallback covers legacy rows and the brief window when a `state_changed` WS
 * event updates `is_passthrough` before the full session is hydrated (see
 * `ws/handlers/agent-session.ts`: snapshot writes are truthy-guarded while
 * `is_passthrough` writes use `!== undefined`).
 */
export function isPassthroughSession(session: PassthroughSessionInput | null | undefined): boolean {
  if (session?.is_passthrough !== undefined) return session.is_passthrough;
  const snapshot = session?.agent_profile_snapshot as { cli_passthrough?: boolean } | undefined;
  return snapshot?.cli_passthrough === true;
}
