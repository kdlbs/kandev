"use client";

import { memo, useCallback, useMemo, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { IconArrowsMaximize, IconSparkles, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { QuickChatSessionView } from "@/components/quick-chat/quick-chat-session-view";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import { ConfigChatSetup } from "./config-chat-setup";
import { useConfigChat } from "./use-config-chat";

function useConfigChatPanelStore() {
  return useAppStore(
    useShallow((state) => ({
      quickChatSessions: state.quickChat.sessions,
      openQuickChat: state.openQuickChat,
      setQuickChatInitialPrompt: state.setQuickChatInitialPrompt,
    })),
  );
}

function PanelHeader({ onExpand, onClose }: { onExpand: () => void; onClose: () => void }) {
  return (
    <header className="flex h-12 shrink-0 items-center justify-between border-b bg-muted/30 pl-3">
      <div className="flex min-w-0 items-center gap-2">
        <IconSparkles className="h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="truncate text-sm font-medium">Configuration Chat</span>
      </div>
      <div className="flex items-center">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="icon"
              variant="ghost"
              className="h-11 w-11 cursor-pointer rounded-none"
              onClick={onExpand}
              aria-label="Open in Quick Chat"
            >
              <IconArrowsMaximize className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Open in Quick Chat</TooltipContent>
        </Tooltip>
        <Button
          size="icon"
          variant="ghost"
          className="h-11 w-11 cursor-pointer rounded-none"
          onClick={onClose}
          aria-label="Close configuration chat"
        >
          <IconX className="h-4 w-4" />
        </Button>
      </div>
    </header>
  );
}

function useConfigChatPanelController(workspaceId: string) {
  const chat = useConfigChat(workspaceId);
  const store = useConfigChatPanelStore();
  const [isOpen, setIsOpen] = useState(false);
  const session = useMemo(
    () =>
      store.quickChatSessions.find(
        (item) =>
          item.workspaceId === workspaceId &&
          item.kind === "config" &&
          !isQuickChatSetupSessionId(item.sessionId),
      ),
    [store.quickChatSessions, workspaceId],
  );

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) chat.reset();
      setIsOpen(open);
    },
    [chat.reset],
  );

  const handleStart = useCallback(
    (profileId: string, prompt: string) =>
      chat.startSession(profileId, prompt, { openInQuickChat: false }),
    [chat.startSession],
  );

  const handleExpand = useCallback(() => {
    chat.reset();
    if (session) {
      store.openQuickChat(session.sessionId, workspaceId, session.agentProfileId, "config");
    } else {
      store.openQuickChat("", workspaceId, undefined, "config");
    }
    setIsOpen(false);
  }, [chat.reset, session, store, workspaceId]);

  return {
    ...chat,
    ...store,
    session,
    isOpen,
    handleOpenChange,
    handleStart,
    handleExpand,
  };
}

export const ConfigChatPanel = memo(function ConfigChatPanel({
  workspaceId,
}: {
  workspaceId: string;
}) {
  const panel = useConfigChatPanelController(workspaceId);

  return (
    <Popover open={panel.isOpen} onOpenChange={panel.handleOpenChange}>
      <Tooltip open={panel.isOpen ? false : undefined}>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              size="icon"
              className="fixed bottom-6 right-6 z-50 h-12 w-12 cursor-pointer rounded-full shadow-lg"
              aria-label="Configuration Chat"
            >
              <IconSparkles className="h-6 w-6" />
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent side="left">
          <p className="font-medium">Configuration Chat</p>
          <p className="text-xs text-muted-foreground">Configure Kandev with natural language</p>
        </TooltipContent>
      </Tooltip>
      <PopoverContent
        side="top"
        align="end"
        sideOffset={8}
        onInteractOutside={(event) => event.preventDefault()}
        data-testid="config-chat-popover"
        className="flex h-[min(550px,calc(100dvh-6rem))] w-[min(420px,calc(100vw-2rem))] flex-col gap-0 overflow-hidden p-0 shadow-2xl"
      >
        <PanelHeader onExpand={panel.handleExpand} onClose={() => panel.handleOpenChange(false)} />
        {panel.session ? (
          <QuickChatSessionView
            session={panel.session}
            onInitialPromptSent={() =>
              panel.setQuickChatInitialPrompt(panel.session!.sessionId, undefined)
            }
          />
        ) : (
          <ConfigChatSetup
            presentation="floating"
            defaultProfileId={panel.defaultProfileId}
            isStarting={panel.isStarting}
            error={panel.error}
            onStart={panel.handleStart}
          />
        )}
      </PopoverContent>
    </Popover>
  );
});
