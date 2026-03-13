"use client";

import { useSyncExternalStore } from "react";
import { usePathname } from "next/navigation";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { ConfigChatPanel } from "./config-chat-panel";

// SSR-safe client mount detection without useEffect setState
const emptySubscribe = () => () => {};
const getClientSnapshot = () => true;
const getServerSnapshot = () => false;

function useIsMounted() {
  return useSyncExternalStore(emptySubscribe, getClientSnapshot, getServerSnapshot);
}

/**
 * Global provider for Config Chat functionality.
 * Renders the floating popover that can be opened from anywhere in the app.
 * The FAB trigger is only visible on settings pages; on other pages the popover
 * can be opened via the command panel (Cmd+K → "Configuration Chat").
 */
export function ConfigChatProvider({ children }: { children: React.ReactNode }) {
  const activeWorkspace = useAppStore((s) => s.workspaces.activeId);
  const mounted = useIsMounted();
  const pathname = usePathname();
  const isSettingsPage = pathname.startsWith("/settings");

  // Preload agent profiles so they're available when config chat is opened
  useSettingsData(true);

  return (
    <>
      {children}
      {mounted && activeWorkspace && (
        <ConfigChatPanel workspaceId={activeWorkspace} showFab={isSettingsPage} />
      )}
    </>
  );
}
