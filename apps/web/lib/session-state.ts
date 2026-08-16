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

/**
 * Visibility rule for the composer's "a message will auto-start the agent"
 * hint (the recovered-idle / resume-skipped affordance that replaced the
 * footer Start agent button). The hint promises that sending a message will
 * start the agent, so it may only render for states the composer can
 * actually send into: WAITING_FOR_INPUT or IDLE (deriveSessionInputMode's
 * "direct" set for non-CREATED sessions). Terminal states (FAILED,
 * CANCELLED, COMPLETED) are excluded: the composer rejects sends there
 * ("Session has ended"), so the hint would be a dead-end promise; recovery
 * actions or the session menu own those surfaces.
 */
export function shouldShowComposerAgentStartHint({
  resumeSkipped,
  sessionState,
  hasRecoveryActions,
  sessionId,
}: {
  resumeSkipped: boolean;
  sessionState?: TaskSessionState;
  hasRecoveryActions: boolean;
  sessionId: string | null;
}): boolean {
  return (
    resumeSkipped &&
    !hasRecoveryActions &&
    (sessionState === "WAITING_FOR_INPUT" || sessionState === "IDLE") &&
    sessionId !== null
  );
}
