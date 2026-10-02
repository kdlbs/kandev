import type { Message } from "@/lib/types/http";

export function isGitPushErrorMessage(message: Pick<Message, "type" | "metadata">): boolean {
  const metadata = message.metadata;
  return (
    message.type === "error" &&
    metadata?.git_operation_error === true &&
    metadata.operation === "push"
  );
}

export function isDismissedGitPushErrorMessage(
  message: Pick<Message, "type" | "metadata">,
): boolean {
  if (!isGitPushErrorMessage(message)) return false;
  const dismissedAt = message.metadata?.git_operation_error_dismissed_at;
  return typeof dismissedAt === "string" && dismissedAt.length > 0;
}
