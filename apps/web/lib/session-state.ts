import type { TaskSessionState } from "@/lib/types/http";

/**
 * Lifecycle rank for launch-response regression checks. Ordered from
 * "agent not running" (CREATED/IDLE) through STARTING to live states
 * (RUNNING/WAITING_FOR_INPUT) and terminal states. A launch response
 * (typically STARTING) must never overwrite a live state that is FURTHER
 * along the lifecycle: a WebSocket RUNNING/FAILED transition can land before
 * `session.launch` resolves, and unconditionally writing the older response
 * state would hide a running agent or a failure's recovery affordances.
 */
const LAUNCH_STATE_ORDER: Record<TaskSessionState, number> = {
  CREATED: 0,
  IDLE: 0,
  STARTING: 1,
  RUNNING: 2,
  WAITING_FOR_INPUT: 2,
  COMPLETED: 3,
  FAILED: 3,
  CANCELLED: 3,
};

/**
 * True when applying `incomingState` over `liveState` would regress the
 * session (incoming is earlier in the lifecycle than what is live). Unknown
 * states never count as a regression, so a live row with an unfamiliar state
 * still accepts the launch response (the button-hiding hydration wins).
 */
export function isLaunchStateRegression(
  liveState: string | undefined,
  incomingState: string,
): boolean {
  if (!liveState) return false;
  const live = LAUNCH_STATE_ORDER[liveState as TaskSessionState];
  const incoming = LAUNCH_STATE_ORDER[incomingState as TaskSessionState];
  if (live === undefined || incoming === undefined) return false;
  return incoming < live;
}
