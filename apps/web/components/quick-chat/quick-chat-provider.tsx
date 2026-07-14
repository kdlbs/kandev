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

/**
 * Global provider for Quick Chat functionality.
 * Renders the modal that can be opened from anywhere in the app.
 * Preloads agent profiles so they're available when quick chat is opened.
 */
export function QuickChatProvider({ children }: { children: React.ReactNode }) {
  const activeWorkspace = useAppStore((s) => s.workspaces.activeId);
  const mounted = useIsMounted();

  // Preload agent profiles so they're available when quick chat is opened
  useSettingsData(true);

  return (
    <>
      {children}
      {/* Only render modal on client side and if we have a workspace */}
      {mounted && activeWorkspace && <QuickChatModal workspaceId={activeWorkspace} />}
    </>
  );
}
