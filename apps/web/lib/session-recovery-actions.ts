export type RecoveryActionKind =
  | "resume"
  | "fresh_start"
  | "runtime_retry"
  | "resume_new_branch"
  | "relocate_and_resume"
  | "continue_from_history"
  | "retry_connection"
  | "restore";

export function selectPrimaryRecoveryAction(
  actions: readonly RecoveryActionKind[],
  blocked = false,
): RecoveryActionKind | null {
  if (blocked) return null;
  const priority: RecoveryActionKind[] = [
    "retry_connection",
    "resume_new_branch",
    "relocate_and_resume",
    "continue_from_history",
    "runtime_retry",
    "resume",
    "restore",
    "fresh_start",
  ];
  return priority.find((action) => actions.includes(action)) ?? null;
}
