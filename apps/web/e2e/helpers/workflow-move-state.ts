export function hasPendingWorkflowMove(metadata: Record<string, unknown>): boolean {
  return (
    Object.hasOwn(metadata, "manual_move_lifecycle_pending") &&
    !Object.hasOwn(metadata, "manual_move_lifecycle_completed")
  );
}
