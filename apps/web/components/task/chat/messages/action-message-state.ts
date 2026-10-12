import { useAppStore } from "@/components/state-provider";
import type { Message } from "@/lib/types/http";
import { hasAgentActivityAfterNotice } from "@/lib/state/slices/session/running-notice-activity";
import {
  hasFailedAgentBootAfter,
  hasSessionRecoveryResolutionAfter,
  hasSuccessfulAgentBootAfter,
  isSelectionFailureRecoveryMetadata,
} from "@/hooks/processed-message-filtering";

function recoveryErrorStamp(metadata: Record<string, unknown> | undefined): string | undefined {
  if (typeof metadata?.recovery_stamp === "string") return metadata.recovery_stamp;
  if (typeof metadata?.error_stamp === "string") return metadata.error_stamp;
  return undefined;
}

/** Reads how the agent boots after this message turned out. Each selector returns a
 * derived boolean, not the array, so the memoized message row does not re-render on
 * every streamed token. */
export function useAgentBootOutcomeAfterMessage(
  comment: Message,
  enabled: boolean,
  sessionMetadata?: Record<string, unknown> | null,
): {
  agentRebooted: boolean;
  agentBootFailed: boolean;
  recoveryResolved: boolean;
} {
  const metadata = comment.metadata as Record<string, unknown> | undefined;
  const errorStamp = recoveryErrorStamp(metadata);
  const requireExactStamp =
    isSelectionFailureRecoveryMetadata(metadata) ||
    isSelectionFailureRecoveryMetadata(sessionMetadata?.last_agent_error);
  const messages = useAppStore((state) =>
    enabled && comment.session_id ? state.messages.bySession[comment.session_id] : undefined,
  );
  const agentRebooted = useAppStore((state) =>
    enabled && comment.session_id
      ? hasSuccessfulAgentBootAfter(
          state.messages.bySession[comment.session_id],
          comment.created_at,
          errorStamp,
          requireExactStamp,
          sessionMetadata,
        )
      : false,
  );
  const agentBootFailed = useAppStore((state) =>
    enabled && comment.session_id && !requireExactStamp
      ? hasFailedAgentBootAfter(state.messages.bySession[comment.session_id], comment.created_at)
      : false,
  );
  const recoveryResolved =
    enabled &&
    hasSessionRecoveryResolutionAfter(
      sessionMetadata,
      comment.created_at,
      errorStamp,
      messages,
      requireExactStamp,
    );
  return { agentRebooted, agentBootFailed, recoveryResolved };
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

export function useRunningNoticeResolved(comment: Message, enabled: boolean): boolean {
  return useAppStore((state) =>
    enabled && comment.session_id
      ? hasAgentActivityAfterNotice(state.messages.bySession[comment.session_id], comment)
      : false,
  );
}
