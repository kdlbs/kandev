import type { TaskSession } from "@/lib/types/http";

export type SessionInputMode = "direct" | "queue" | "unavailable";

/**
 * Derives promptability for one selected session.
 *
 * Task-wide activity is deliberately not an input: another session working
 * must never force this session's prompt into the queue.
 */
export function deriveSessionInputMode(
  session: Pick<TaskSession, "state" | "foreground_activity"> | null | undefined,
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
  return session.foreground_activity === "background" ? "direct" : "queue";
}
