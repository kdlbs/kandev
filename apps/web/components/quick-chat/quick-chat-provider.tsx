"use client";

import { useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { QuickChatModal } from "./quick-chat-modal";

// SSR-safe client mount detection without useEffect setState
const emptySubscribe = () => () => {};
const getClientSnapshot = () => true;
const getServerSnapshot = () => false;

function useIsMounted() {
  return useSyncExternalStore(emptySubscribe, getClientSnapshot, getServerSnapshot);
}

function getWorkspaceId(
  sessions: { workspaceId: string }[],
  isOpen: boolean,
  activeWorkspace: string | null,
): string | null {
  if (sessions.length > 0) {
    return sessions[0].workspaceId;
  }
  if (isOpen) {
    return activeWorkspace;
  }
  return null;
}

/**
 * Global provider for Quick Chat functionality.
 * Renders the modal that can be opened from anywhere in the app.
 * Preloads agent profiles so they're available when quick chat is opened.
 */
export function QuickChatProvider({ children }: { children: React.ReactNode }) {
  const quickChatSessions = useAppStore((s) => s.quickChat.sessions);
  const isOpen = useAppStore((s) => s.quickChat.isOpen);
  const activeWorkspace = useAppStore((s) => s.workspaces.activeId);
  const mounted = useIsMounted();

  // Preload agent profiles so they're available when quick chat is opened
  useSettingsData(true);

  // Get workspace ID from first session, or use active workspace if modal is open
  const workspaceId = getWorkspaceId(quickChatSessions, isOpen, activeWorkspace);

  return (
    <>
      {children}
      {/* Only render modal on client side and if we have a workspace */}
      {mounted && workspaceId && <QuickChatModal workspaceId={workspaceId} />}
    </>
  );
}
