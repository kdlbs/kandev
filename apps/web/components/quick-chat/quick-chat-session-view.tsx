"use client";

import { useAppStore } from "@/components/state-provider";
import { useEnsureTaskSession } from "@/hooks/use-ensure-task-session";
import { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { PassthroughTerminal } from "@/components/task/passthrough-terminal";
import { SessionRecoveryFeedback } from "@/components/task/ensure-session-error";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";
import { QuickChatContent } from "./quick-chat-content";
import { useTranslation } from "react-i18next";

function useIsQuickChatPassthrough(sessionId: string) {
  return useAppStore((state) => {
    const session = state.taskSessions.items[sessionId];
    if (typeof session?.is_passthrough === "boolean") return session.is_passthrough;
    const profileId =
      session?.agent_profile_id ??
      state.quickChat.sessions.find((item) => item.sessionId === sessionId)?.agentProfileId;
    if (!profileId) return false;
    return state.agentProfiles.items.find((profile) => profile.id === profileId)?.cli_passthrough;
  });
}

type QuickChatSessionViewProps = {
  session: QuickChatSession;
  onInitialPromptSent?: () => void;
};

export function QuickChatSessionView({ session, onInitialPromptSent }: QuickChatSessionViewProps) {
  const { t } = useTranslation();
  // A tab can arrive from a task event, which carries no session payload.
  // Fetch the row on open so such a tab is usable, not just visible.
  useEnsureTaskSession(session.sessionId);
  const taskSession = useAppStore((state) => state.taskSessions.items[session.sessionId] ?? null);
  const taskId = taskSession ? (session.taskId ?? taskSession.task_id ?? null) : null;
  const resumption = useSessionResumption(taskId, session.sessionId);
  const isPassthrough = useIsQuickChatPassthrough(session.sessionId);
  const recoveryFeedback = (
    <SessionRecoveryFeedback
      error={resumption.error}
      notice={resumption.notice}
      onRetry={() => void resumption.resumeSession()}
    />
  );
  if (isPassthrough) {
    return (
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        {recoveryFeedback}
        <div className="min-h-0 flex-1">
          <PassthroughTerminal key={session.sessionId} sessionId={session.sessionId} mode="agent" />
        </div>
      </div>
    );
  }
  const isConfig = session.kind === "config";
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {recoveryFeedback}
      <div className="flex min-h-0 flex-1 flex-col">
        <QuickChatContent
          sessionId={session.sessionId}
          minimalToolbar={isConfig}
          placeholderOverride={isConfig ? t("chat:configChatPlaceholder") : undefined}
          initialPrompt={session.initialPrompt}
          onInitialPromptSent={onInitialPromptSent}
        />
      </div>
    </div>
  );
}
