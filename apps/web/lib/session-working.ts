import type { TaskSession } from "@/lib/types/http";

/** Returns whether a session still has live agent or background work. */
export function isSessionWorking(
  session: Pick<TaskSession, "state" | "foreground_activity"> | null | undefined,
): boolean {
  if (!session) return false;
  return (
    session.state === "STARTING" ||
    session.state === "RUNNING" ||
    session.foreground_activity === "background"
  );
}
