import type { TaskSession } from "@/lib/types/http";

export type SessionInputMode = "direct" | "queue" | "unavailable";

/**
 * Derives promptability for one selected session.
 *
 * Task-wide activity is deliberately not an input: another session working
 * must never force this session's prompt into the queue.
 */
export function deriveSessionInputMode(
  session:
    | Pick<TaskSession, "state" | "foreground_activity" | "supports_steering">
    | null
    | undefined,
  queuedCount = 0,
): SessionInputMode {
  if (!session) return "unavailable";
  if (
    session.state === "CREATED" ||
    session.state === "IDLE" ||
    session.state === "WAITING_FOR_INPUT"
  ) {
    return "direct";
  }
  if (session.state === "STARTING") return "queue";
  if (session.state !== "RUNNING") return "unavailable";
  if (session.foreground_activity === "background") return "direct";
  // A generating RUNNING session normally queues. When the connected agent can
  // steer (supports_steering), the send is instead delivered into the running
  // turn, so it is direct rather than queued. Whether the agent folds it or runs
  // it next is the agent's call and not distinguishable here — the composer copy
  // promises delivery, not folding (see mid-turn-steering spec).
  if (session.supports_steering && queuedCount === 0) return "direct";
  return "queue";
}

/**
 * Whether the composer should show the steer-delivery affordance (vs. the
 * ordinary queue affordance) for a session whose backend advertises
 * `supports_steering`.
 *
 * A steer never jumps an already-queued message: `SteerTask` on the backend
 * enqueues behind pending work instead of dispatching (see steer.go's order
 * rule). The composer must match that observable contract — once anything is
 * queued, the next send would actually be queued too, so the affordance must
 * fall back to the ordinary queue copy rather than promising delivery into
 * the running turn.
 */
export function resolvesSteeringAffordance(
  supportsSteering: boolean,
  queuedCount: number,
): boolean {
  return supportsSteering && queuedCount === 0;
}
