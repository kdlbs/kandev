import type { Task } from "@/lib/types/http";

export function isDetachedManagedConversation(
  task: Pick<Task, "metadata"> | null | undefined,
): boolean {
  return (
    task?.metadata?.["kandev.managed_retained"] === true &&
    task.metadata["kandev.detached"] === true
  );
}

export function managedConversationBlocksQueueDelivery(
  task: Pick<Task, "metadata"> | null | undefined,
): boolean {
  const metadata = task?.metadata;
  return (
    metadata?.["kandev.managed_retained"] === true &&
    (metadata["kandev.desired_paused"] !== false || isDetachedManagedConversation(task))
  );
}
