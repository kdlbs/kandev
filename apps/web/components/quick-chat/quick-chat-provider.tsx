"use client";

import { useEffect, useRef, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useQuickChatResync } from "@/hooks/use-quick-chat-resync";
import { restoreQuickChatLauncherFocus } from "./quick-chat-focus";
import { QuickChatModal } from "./quick-chat-modal";

// SSR-safe client mount detection without useEffect setState
const emptySubscribe = () => () => {};
const getClientSnapshot = () => true;
const getServerSnapshot = () => false;

function useIsMounted() {
  return useSyncExternalStore(emptySubscribe, getClientSnapshot, getServerSnapshot);
}

type WorkspaceSelection = {
  sessions: { sessionId: string; workspaceId: string }[];
  terminalTabs: { tabId: string; workspaceId: string }[];
  isOpen: boolean;
  activeKind: "conversation" | "terminal";
  activeSessionId: string | null;
  activeTerminalTabId: string | null;
  activeWorkspace: string | null;
};

export function getWorkspaceId({
  sessions,
  terminalTabs,
  isOpen,
  activeKind,
  activeSessionId,
  activeTerminalTabId,
  activeWorkspace,
}: WorkspaceSelection): string | null {
  if (!isOpen) return null;
  if (activeKind === "terminal") {
    const activeTerminal = terminalTabs.find((tab) => tab.tabId === activeTerminalTabId);
    if (activeTerminal) return activeTerminal.workspaceId;
  }
  return (
    sessions.find((session) => session.sessionId === activeSessionId)?.workspaceId ??
    terminalTabs.find((tab) => tab.tabId === activeTerminalTabId)?.workspaceId ??
    activeWorkspace
  );
}

/**
 * Global provider for Quick Chat functionality.
 * Renders the modal that can be opened from anywhere in the app.
 * Preloads agent profiles so they're available when quick chat is opened.
 */
export function QuickChatProvider({ children }: { children: React.ReactNode }) {
  const quickChatSessions = useAppStore((s) => s.quickChat.sessions);
  const isOpen = useAppStore((s) => s.quickChat.isOpen);
  const quickTerminalTabs = useAppStore((s) => s.quickChat.terminalTabs);
  const activeKind = useAppStore((s) => s.quickChat.activeKind);
  const activeSessionId = useAppStore((s) => s.quickChat.activeSessionId);
  const activeTerminalTabId = useAppStore((s) => s.quickChat.activeTerminalTabId);
  const activeWorkspace = useAppStore((s) => s.workspaces.activeId);
  const mounted = useIsMounted();
  const wasOpen = useRef(false);

  // Preload agent profiles so they're available when quick chat is opened
  useSettingsData(true);
  // Quick chats are shared across devices: re-read the server's list whenever
  // the socket connects so tabs opened or closed elsewhere show up here too.
  useQuickChatResync(activeWorkspace);

  const workspaceId = getWorkspaceId({
    sessions: quickChatSessions,
    terminalTabs: quickTerminalTabs,
    isOpen,
    activeKind,
    activeSessionId,
    activeTerminalTabId,
    activeWorkspace,
  });

  useEffect(() => {
    if (wasOpen.current && !isOpen) restoreQuickChatLauncherFocus();
    wasOpen.current = isOpen;
  }, [isOpen]);

  return (
    <>
      {children}
      {/* Only render modal on client side and if we have a workspace */}
      {mounted && workspaceId && <QuickChatModal workspaceId={workspaceId} />}
    </>
  );
}
