"use client";

import { useMemo } from "react";
import type { Message, TaskSessionState } from "@/lib/types/http";
import { AgentStatus } from "@/components/task/chat/messages/agent-status";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import { SessionResumeStartButton } from "@/components/task/chat/session-resume-start-button";
import { useAppStore } from "@/components/state-provider";

type MessageListFooterProps = {
  sessionState?: TaskSessionState;
  sessionId: string | null;
  messages: Message[];
  isWorking?: boolean;
  footerActionMessages?: Message[];
};

function isMissingBranchFailure(message: Message): boolean {
  const metadata = message.metadata as Record<string, unknown> | undefined;
  const actions = metadata?.actions;
  if (!Array.isArray(actions) || actions.length === 0) return false;
  return metadata?.failure_kind === "missing_pr_branch";
}

function isActionableFailure(message: Message): boolean {
  const metadata = message.metadata as Record<string, unknown> | undefined;
  return Array.isArray(metadata?.actions) && metadata.actions.length > 0;
}

function findCurrentActionableFailure(
  messages: Message[],
  footerActionMessages: Message[],
): Message | undefined {
  for (let index = messages.length - 1; index >= 0; index--) {
    if (isActionableFailure(messages[index])) return messages[index];
  }
  return footerActionMessages.at(-1);
}

export function MessageListFooter({
  sessionState,
  sessionId,
  messages,
  isWorking,
  footerActionMessages = [],
}: MessageListFooterProps) {
  const currentActionableFailure = findCurrentActionableFailure(messages, footerActionMessages);
  const recoveryOwnsFailure =
    sessionState === "FAILED" &&
    currentActionableFailure !== undefined &&
    isMissingBranchFailure(currentActionableFailure);
  const visibleFooterActionMessages = footerActionMessages.filter(
    (message) => !isMissingBranchFailure(message) || message.id === currentActionableFailure?.id,
  );
  // The task-description start button only renders for EMPTY sessions. A
  // resume-skipped session (prevent-auto-start-on-open preference, agent
  // stopped with history) gets its Start agent button here instead — but only
  // when no recovery actions are already visible: if the conversation shows
  // "Resume session" / "Start fresh session" (an agent-kill recovery action
  // message), that is the manual affordance and a second Start agent button
  // would be duplicate noise.
  const resumeSkipped = useAppStore((state) =>
    sessionId ? state.tasks.resumeSkippedSessionIds[sessionId] === true : false,
  );
  const hasRecoveryActions = useMemo(
    () =>
      messages.some(
        (message) =>
          (message.metadata as Record<string, unknown> | undefined)?.recovery_actions === true,
      ) ||
      footerActionMessages.some(
        (message) =>
          (message.metadata as Record<string, unknown> | undefined)?.recovery_actions === true,
      ),
    [messages, footerActionMessages],
  );
  // The footer is the single Start-agent surface for resume-skipped sessions
  // (prevent-auto-start-on-open): it must render even when the session has
  // no messages or the task has no description, so every gated recovered-idle
  // session keeps the manual affordance.
  const showResumeStartButton =
    resumeSkipped &&
    !hasRecoveryActions &&
    sessionState !== "FAILED" &&
    sessionState !== "RUNNING" &&
    sessionState !== "STARTING" &&
    sessionId !== null;
  return (
    <>
      {!recoveryOwnsFailure && (
        <AgentStatus
          sessionState={sessionState}
          sessionId={sessionId}
          messages={messages}
          isWorking={isWorking}
        />
      )}
      {showResumeStartButton && sessionId !== null && (
        <SessionResumeStartButton sessionId={sessionId} />
      )}
      {visibleFooterActionMessages.map((message) => (
        <MessageRenderer key={message.id} comment={message} isTaskDescription={false} />
      ))}
    </>
  );
}
