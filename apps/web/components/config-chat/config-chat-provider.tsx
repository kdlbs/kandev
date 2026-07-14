"use client";

import { useSyncExternalStore } from "react";
import { usePathname } from "@/lib/routing/client-router";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconSparkles } from "@tabler/icons-react";

// SSR-safe client mount detection without useEffect setState
const emptySubscribe = () => () => {};
const getClientSnapshot = () => true;
const getServerSnapshot = () => false;

function useIsMounted() {
  return useSyncExternalStore(emptySubscribe, getClientSnapshot, getServerSnapshot);
}

/**
 * Global provider for Config Chat functionality.
 * Renders the Settings FAB that opens a typed configuration tab in Quick Chat.
 * Other pages use the command panel (Cmd+K -> "Configuration Chat").
 */
export function ConfigChatProvider({ children }: { children: React.ReactNode }) {
  const activeWorkspace = useAppStore((s) => s.workspaces.activeId);
  const mounted = useIsMounted();
  const pathname = usePathname();
  const isSettingsPage = pathname.startsWith("/settings");
  const openConfigChat = useQuickChatLauncher(activeWorkspace, "config");

  // Preload agent profiles so they're available when config chat is opened
  useSettingsData(true);

  return (
    <>
      {children}
      {mounted && activeWorkspace && isSettingsPage && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="icon"
              onClick={openConfigChat}
              className="fixed bottom-6 right-6 z-50 h-12 w-12 cursor-pointer rounded-full shadow-lg"
              aria-label="Configuration Chat"
            >
              <IconSparkles className="h-6 w-6" aria-hidden />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="left">
            <p className="font-medium">Configuration Chat</p>
            <p className="text-xs text-muted-foreground">Configure Kandev with natural language</p>
          </TooltipContent>
        </Tooltip>
      )}
    </>
  );
}
