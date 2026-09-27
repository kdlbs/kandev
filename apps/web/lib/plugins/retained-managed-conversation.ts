import type { Task } from "@/lib/types/http";

export function isDetachedManagedConversation(
  task: Pick<Task, "metadata"> | null | undefined,
): boolean {
  return (
    task?.metadata?.["kandev.managed_retained"] === true &&
    task.metadata["kandev.detached"] === true
  );
}
