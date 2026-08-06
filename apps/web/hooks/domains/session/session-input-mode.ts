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
  if (session.supports_steering) return "direct";
  return "queue";
}
