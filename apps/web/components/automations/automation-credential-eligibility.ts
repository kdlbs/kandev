import type { ContinuationPolicy, TaskMode, TriggerType } from "@/lib/types/automation";

export function supportsPersonalCredentialChoices(
  taskMode: TaskMode,
  continuationPolicy: ContinuationPolicy,
  triggers: readonly { type: TriggerType }[],
): boolean {
  return (
    taskMode === "automation_run" &&
    continuationPolicy === "new_task" &&
    triggers.every((trigger) => trigger.type === "manual" || trigger.type === "scheduled")
  );
}
