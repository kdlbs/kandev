"use client";

import { useMemo } from "react";
import type { Message, TaskSessionState } from "@/lib/types/http";
import { useAppStore } from "@/components/state-provider";
import { shouldShowComposerAgentStartHint } from "@/lib/session-state";

/** True when any message in the list carries recovery-action metadata. */
function hasRecoveryActions(messages: Message[]): boolean {
  return messages.some(
    (message) =>
      (message.metadata as Record<string, unknown> | undefined)?.recovery_actions === true,
  );
}

/**
 * Whether the composer shows the "a message will auto-start the agent" hint
 * for a recovered-idle (resume-skipped) session — the affordance that
 * replaced the footer Start agent button (prevent-auto-start-on-open
 * preference). Reads the resume-skipped flag from the store and derives the
 * recovery-actions check from the transcript, then delegates to the shared
 * {@link shouldShowComposerAgentStartHint} predicate.
 */
export function useComposerAgentStartHint(
  sessionId: string | null,
  sessionState: TaskSessionState | undefined,
  messages: Message[],
  footerActionMessages: Message[],
): boolean {
  const resumeSkipped = useAppStore((state) =>
    sessionId ? state.tasks.resumeSkippedSessionIds[sessionId] === true : false,
  );
  const recoveryActionsVisible = useMemo(
    () => hasRecoveryActions(messages) || hasRecoveryActions(footerActionMessages),
    [messages, footerActionMessages],
  );
  return shouldShowComposerAgentStartHint({
    resumeSkipped,
    sessionState,
    hasRecoveryActions: recoveryActionsVisible,
    sessionId,
  });
}
