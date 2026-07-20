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
import { cn } from "@/lib/utils";

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

type ConfigChatPanelProps = {
  workspaceId: string;
  setFloatingActionsHost?: (host: HTMLElement | null) => void;
};

function ConfigChatFloatingActionsHost({
  setHost,
}: {
  setHost?: ConfigChatPanelProps["setFloatingActionsHost"];
}) {
  return (
    <div
      ref={setHost}
      className="pointer-events-none absolute right-0 bottom-[calc(100%+0.75rem)] z-10 max-w-[calc(100vw_-_2rem_-_env(safe-area-inset-left)_-_env(safe-area-inset-right))]"
      data-testid="config-chat-floating-actions"
    />
  );
}

export const ConfigChatPanel = memo(function ConfigChatPanel({
  workspaceId,
  setFloatingActionsHost,
}: ConfigChatPanelProps) {
  const panel = useConfigChatPanelController(workspaceId);

  return (
    <Popover open={panel.isOpen} onOpenChange={panel.handleOpenChange}>
      <Tooltip open={panel.isOpen ? false : undefined}>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              size="icon"
              aria-hidden={panel.isOpen}
              tabIndex={panel.isOpen ? -1 : undefined}
              className={cn(
                "fixed bottom-6 right-6 z-50 h-12 w-12 cursor-pointer rounded-full shadow-lg",
                panel.isOpen && "pointer-events-none opacity-0",
              )}
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
        className="relative flex h-[min(550px,calc(100dvh_-_11rem_-_env(safe-area-inset-top)_-_env(safe-area-inset-bottom)))] max-h-[550px] w-[min(420px,calc(100vw_-_2rem))] flex-col gap-0 overflow-visible p-0 shadow-2xl"
      >
        <ConfigChatFloatingActionsHost setHost={setFloatingActionsHost} />
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-[inherit]">
          <PanelHeader
            onExpand={panel.handleExpand}
            onClose={() => panel.handleOpenChange(false)}
          />
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
        </div>
      </PopoverContent>
    </Popover>
  );
});
