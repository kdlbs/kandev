"use client";

import { AdvancedChatPanel } from "../../../../tasks/[id]/advanced-panels/chat-panel";

type Props = {
  taskId: string;
  sessionId: string;
};

/**
 * Thin wrapper that embeds the existing chat panel scoped to the
 * run's session. The chat panel already covers messages, tool calls,
 * status rows, and live updates so we don't ship a bespoke
 * transcript renderer. `hideInput` keeps the embed read-only — the
 * run detail is for review, not for sending follow-ups (those happen
 * via the task page).
 */
export function RunConversation({ taskId, sessionId }: Props) {
  if (!taskId || !sessionId) {
    return (
      <div
        className="rounded-lg border border-border p-4 text-xs text-muted-foreground"
        data-testid="run-conversation-empty"
      >
        No conversation linked to this run.
      </div>
    );
  }
  return (
    <div className="rounded-lg border border-border overflow-hidden" data-testid="run-conversation">
      <AdvancedChatPanel taskId={taskId} sessionId={sessionId} hideInput />
    </div>
  );
}
