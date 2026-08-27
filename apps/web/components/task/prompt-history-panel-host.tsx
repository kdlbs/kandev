"use client";

import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { panelTitle } from "@/lib/state/layout-manager/panel-title";
import { PromptHistoryPanelContent } from "./prompt-history-panel-content";

/**
 * Prompt-history arrow wiring: the panel content is a pure seam consumer, so
 * the host binds `onNavigateToPrompt` to the dockview store's
 * `scrollTranscriptToMessage`, resolving the tab title as the session's name
 * (truthy check — an EMPTY-string name falls back too) or the localized
 * canonical chat title. Shared by the desktop and Office workbench registries
 * so the title rule lives in one place.
 */
export function PromptHistoryContent() {
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const session = useAppStore((state) => (sessionId ? state.taskSessions.items[sessionId] : null));
  const scrollTranscriptToMessage = useDockviewStore((state) => state.scrollTranscriptToMessage);
  return (
    <PromptHistoryPanelContent
      onNavigateToPrompt={(messageId: string) => {
        if (sessionId) {
          scrollTranscriptToMessage(sessionId, messageId, session?.name || panelTitle("chat"));
        }
      }}
    />
  );
}
