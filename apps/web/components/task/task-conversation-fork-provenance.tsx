import { ConversationForkProvenance } from "./conversation-fork-provenance";

export function conversationForkIdFromTaskMetadata(
  metadata: Record<string, unknown> | null | undefined,
): string | null {
  const value = metadata?.conversation_fork_id;
  return typeof value === "string" && value.length > 0 ? value : null;
}

export function TaskConversationForkProvenance({
  metadata,
}: {
  metadata: Record<string, unknown> | null | undefined;
}) {
  const forkId = conversationForkIdFromTaskMetadata(metadata);
  return forkId ? (
    <div data-testid="task-conversation-fork-provenance">
      <ConversationForkProvenance forkId={forkId} />
    </div>
  ) : null;
}
