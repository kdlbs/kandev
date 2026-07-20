"use client";

import { useAppStore } from "@/components/state-provider";
import { PassthroughTerminal } from "@/components/task/passthrough-terminal";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";
import { QuickChatContent } from "./quick-chat-content";

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
  const isPassthrough = useIsQuickChatPassthrough(session.sessionId);
  if (isPassthrough) {
    return (
      <div className="min-h-0 flex-1 overflow-hidden">
        <PassthroughTerminal key={session.sessionId} sessionId={session.sessionId} mode="agent" />
      </div>
    );
  }
  const isConfig = session.kind === "config";
  return (
    <QuickChatContent
      sessionId={session.sessionId}
      minimalToolbar={isConfig}
      placeholderOverride={isConfig ? "Ask anything about your configuration..." : undefined}
      initialPrompt={session.initialPrompt}
      onInitialPromptSent={onInitialPromptSent}
    />
  );
}
