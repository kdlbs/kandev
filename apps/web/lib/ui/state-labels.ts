import { t } from "@/lib/i18n";
import type { TaskState, TaskSessionState } from "@/lib/types/http";

/**
 * Catalog keys for the task-state and session-state vocabularies.
 *
 * Keys, not resolved labels: these are module-scope tables, so a `t()` here
 * would resolve once at import and freeze at the boot locale. The exported
 * helpers are plain functions called during their callers' render, so they use
 * the module-level `t` and resolve at call time; every consumer re-renders on
 * `languageChanged` (the sidebar and mobile switcher memos carry
 * `i18n.language` in their deps).
 *
 * The record keys are the wire `TaskState` / `TaskSessionState` values and are
 * never translated — they are compared, persisted and sent to the backend.
 *
 * The two vocabularies keep separate catalog keys even where the English
 * coincides ("Completed", "Failed", …): a task and a session are different
 * subjects, and languages that inflect for gender or number need to say them
 * differently.
 */
const TASK_STATE_LABEL_KEYS: Record<TaskState, string> = {
  CREATED: "common:taskStateCreated",
  SCHEDULING: "common:taskStateScheduling",
  TODO: "common:taskStateTodo",
  IN_PROGRESS: "common:taskStateInProgress",
  REVIEW: "common:taskStateReview",
  BLOCKED: "common:taskStateBlocked",
  WAITING_FOR_INPUT: "common:taskStateWaitingForInput",
  COMPLETED: "common:taskStateCompleted",
  FAILED: "common:taskStateFailed",
  CANCELLED: "common:taskStateCancelled",
};

const TASK_SESSION_STATE_LABEL_KEYS: Record<TaskSessionState, string> = {
  CREATED: "common:sessionStateCreated",
  IDLE: "common:sessionStateIdle",
  STARTING: "common:sessionStateStarting",
  RUNNING: "common:sessionStateRunning",
  WAITING_FOR_INPUT: "common:sessionStateWaitingForInput",
  COMPLETED: "common:sessionStateCompleted",
  FAILED: "common:sessionStateFailed",
  CANCELLED: "common:sessionStateCancelled",
};

export function formatTaskStateLabel(state: TaskState | null | undefined): string {
  if (!state) return t("common:taskStateNotStarted");
  // A wire value this build does not know about echoes raw rather than
  // rendering blank — the same fallback the English table had.
  const key = TASK_STATE_LABEL_KEYS[state];
  return key ? t(key) : state;
}

export function formatTaskSessionStateLabel(state: TaskSessionState | null | undefined): string {
  if (!state) return "";
  const key = TASK_SESSION_STATE_LABEL_KEYS[state];
  return key ? t(key) : state;
}
