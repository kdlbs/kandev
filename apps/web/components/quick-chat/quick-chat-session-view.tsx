"use client";

import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { PassthroughTerminal } from "@/components/task/passthrough-terminal";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";
import { QuickChatContent } from "./quick-chat-content";

function useIsQuickChatPassthrough(sessionId: string) {
  const sessionData = useAppStore(
    useShallow((state) => {
      const session = state.taskSessions.items[sessionId];
      return {
        isPassthrough: session?.is_passthrough,
        profileId:
          session?.agent_profile_id ??
          state.quickChat.sessions.find((item) => item.sessionId === sessionId)?.agentProfileId,
      };
    }),
  );
  const { agentProfiles } = useSettingsData(typeof sessionData.isPassthrough !== "boolean");
  if (typeof sessionData.isPassthrough === "boolean") return sessionData.isPassthrough;
  if (!sessionData.profileId) return false;
  return agentProfiles.find((profile) => profile.id === sessionData.profileId)?.cli_passthrough;
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
