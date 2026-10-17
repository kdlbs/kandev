import type { TaskSessionState } from "@/lib/types/http";

export function isSessionStoppable(s: TaskSessionState): boolean {
  return s === "RUNNING" || s === "STARTING" || s === "WAITING_FOR_INPUT";
}
export function isSessionDeletable(s: TaskSessionState): boolean {
  return s !== "RUNNING" && s !== "STARTING";
}
export function isSessionResumable(s: TaskSessionState): boolean {
  return s === "COMPLETED" || s === "FAILED" || s === "CANCELLED";
}
