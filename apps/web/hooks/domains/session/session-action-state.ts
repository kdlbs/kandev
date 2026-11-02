import type { TaskSessionState } from "@/lib/types/http";

/** Identifies active states eligible for Stop and the composed Restart action. */
export function isSessionStoppable(s: TaskSessionState): boolean {
  return s === "RUNNING" || s === "STARTING" || s === "WAITING_FOR_INPUT";
}
/** Excludes starting and running sessions from the menu's deletion eligibility. */
export function isSessionDeletable(s: TaskSessionState): boolean {
  return s !== "RUNNING" && s !== "STARTING";
}
/** Identifies terminal states eligible for normal same-session Resume. */
export function isSessionResumable(s: TaskSessionState): boolean {
  return s === "COMPLETED" || s === "FAILED" || s === "CANCELLED";
}
