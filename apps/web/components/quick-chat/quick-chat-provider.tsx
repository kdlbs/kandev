"use client";

import { useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { QuickChatModal } from "./quick-chat-modal";

/**
 * Global provider for Quick Chat functionality.
 * Renders the modal that can be opened from anywhere in the app.
 * Preloads agent profiles so they're available when quick chat is opened.
 */
export function QuickChatProvider({ children }: { children: React.ReactNode }) {
  const quickChatSessions = useAppStore((s) => s.quickChat.sessions);
  const isOpen = useAppStore((s) => s.quickChat.isOpen);
  const activeWorkspace = useAppStore((s) => s.workspaces.activeId);
  const [mounted, setMounted] = useState(false);

  // Preload agent profiles so they're available when quick chat is opened
  useSettingsData(true);

  // Only render modal on client side to avoid hydration errors
  useEffect(() => {
    setMounted(true);
  }, []);

  // Get workspace ID from first session, or use active workspace if modal is open
  const workspaceId =
    quickChatSessions.length > 0
      ? quickChatSessions[0].workspaceId
      : isOpen
        ? activeWorkspace
        : null;

  return (
    <>
      {children}
      {/* Only render modal on client side and if we have a workspace */}
      {mounted && workspaceId && <QuickChatModal workspaceId={workspaceId} />}
    </>
  );
}
