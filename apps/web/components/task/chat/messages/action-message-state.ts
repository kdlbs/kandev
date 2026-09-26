import { useAppStore } from "@/components/state-provider";
import type { Message } from "@/lib/types/http";
import {
  hasFailedAgentBootAfter,
  hasSuccessfulAgentBootAfter,
} from "@/hooks/processed-message-filtering";

/** Reads how the agent boots after this message turned out. Each selector returns a
 * derived boolean, not the array, so the memoized message row does not re-render on
 * every streamed token. */
export function useAgentBootOutcomeAfterMessage(
  comment: Message,
  enabled: boolean,
): {
  agentRebooted: boolean;
  agentBootFailed: boolean;
} {
  const agentRebooted = useAppStore((state) =>
    enabled && comment.session_id
      ? hasSuccessfulAgentBootAfter(
          state.messages.bySession[comment.session_id],
          comment.created_at,
        )
      : false,
  );
  const agentBootFailed = useAppStore((state) =>
    enabled && comment.session_id
      ? hasFailedAgentBootAfter(state.messages.bySession[comment.session_id], comment.created_at)
      : false,
  );
  return { agentRebooted, agentBootFailed };
}

export function useActionMessageSession(sessionId: Message["session_id"]) {
  const sessionState = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId]?.state ?? undefined) : undefined,
  );
  const sessionError = useAppStore((state) =>
    sessionId
      ? (state.taskSessions.items[sessionId]?.error_message as string | undefined)
      : undefined,
  );
  const sessionMetadata = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId]?.metadata : undefined,
  );
  const activeTurnId = useAppStore((state) =>
    sessionId ? (state.turns.activeBySession[sessionId] ?? undefined) : undefined,
  );
  return { sessionState, sessionError, sessionMetadata, activeTurnId };
}
