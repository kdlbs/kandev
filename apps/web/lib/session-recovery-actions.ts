export type RecoveryActionKind =
  | "resume"
  | "fresh_start"
  | "runtime_retry"
  | "resume_new_branch"
  | "restore";

export function selectPrimaryRecoveryAction(
  actions: readonly RecoveryActionKind[],
  blocked = false,
): RecoveryActionKind | null {
  if (blocked) return null;
  const priority: RecoveryActionKind[] = [
    "resume_new_branch",
    "runtime_retry",
    "resume",
    "restore",
    "fresh_start",
  ];
  return priority.find((action) => actions.includes(action)) ?? null;
}
