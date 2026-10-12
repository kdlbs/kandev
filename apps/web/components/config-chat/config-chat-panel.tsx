"use client";

import { memo, useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useShallow } from "zustand/react/shallow";
import { IconSparkles } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { QuickChatSessionView } from "@/components/quick-chat/quick-chat-session-view";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import { ChatPopoverShell } from "./chat-popover-shell";
import { ConfigChatSetup } from "./config-chat-setup";
import { useConfigChat } from "./use-config-chat";
import { ConfigChatHeader } from "./config-chat-header";

function useConfigChatPanelStore() {
  return useAppStore(
    useShallow((state) => ({
      quickChatSessions: state.quickChat.sessions,
      openQuickChat: state.openQuickChat,
      setQuickChatInitialPrompt: state.setQuickChatInitialPrompt,
    })),
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

  useEffect(() => {
    handleOpenChange(false);
  }, [workspaceId, handleOpenChange]);

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

function ConfigChatPanelBody({
  panel,
}: {
  panel: ReturnType<typeof useConfigChatPanelController>;
}) {
  const { t } = useTranslation();
  if (panel.restartBlocked)
    return (
      <div
        role="status"
        className="flex min-h-0 flex-1 items-center justify-center p-6 text-sm text-muted-foreground"
      >
        {t(panel.isRestarting ? "configChat:restartingSession" : "configChat:restartCheckStatus")}
      </div>
    );
  if (panel.session)
    return (
      <QuickChatSessionView
        session={panel.session}
        onInitialPromptAttempted={() =>
          panel.setQuickChatInitialPrompt(panel.session!.sessionId, undefined)
        }
      />
    );
  return (
    <ConfigChatSetup
      presentation="floating"
      defaultProfileId={panel.defaultProfileId}
      isStarting={panel.isStarting}
      error={null}
      onStart={panel.handleStart}
    />
  );
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
      className="pointer-events-none absolute inset-x-0 bottom-[calc(100%+0.75rem)] z-10 flex w-full max-w-[calc(100vw_-_2rem_-_env(safe-area-inset-left)_-_env(safe-area-inset-right))] justify-center pl-[calc(0.75rem+_env(safe-area-inset-left))] pr-[calc(0.75rem+_env(safe-area-inset-right))]"
      data-testid="config-chat-floating-actions"
    />
  );
}

export const ConfigChatPanel = memo(function ConfigChatPanel({
  workspaceId,
  setFloatingActionsHost,
}: ConfigChatPanelProps) {
  const panel = useConfigChatPanelController(workspaceId);
  const { t } = useTranslation();

  return (
    <ChatPopoverShell
      open={panel.isOpen}
      onOpenChange={panel.handleOpenChange}
      testId="config-chat-popover"
      icon={<IconSparkles className="h-4 w-4 shrink-0 text-muted-foreground" />}
      title={t("common:configurationChat")}
      closeLabel={t("configChat:closePanel")}
      beforeBody={<ConfigChatFloatingActionsHost setHost={setFloatingActionsHost} />}
      header={
        <ConfigChatHeader
          session={panel.session}
          busy={panel.isStarting || panel.restartBlocked}
          onRestart={(session) => void panel.restartSession(session)}
          onExpand={panel.handleExpand}
          onClose={() => panel.handleOpenChange(false)}
        />
      }
      trigger={
        <Tooltip open={panel.isOpen ? false : undefined}>
          <TooltipTrigger asChild>
            <PopoverTrigger asChild>
              <Button
                size="icon"
                className="fixed bottom-[calc(1.5rem+var(--app-status-bar-height))] right-6 z-50 size-12 max-md:size-12 [@media(pointer:coarse)]:size-12 cursor-pointer rounded-full shadow-lg"
                aria-label={t("common:configurationChat")}
              >
                <IconSparkles className="h-6 w-6" />
              </Button>
            </PopoverTrigger>
          </TooltipTrigger>
          <TooltipContent side="left">
            <p className="font-medium">{t("common:configurationChat")}</p>
            <p className="text-xs text-muted-foreground">{t("configChat:tagline")}</p>
          </TooltipContent>
        </Tooltip>
      }
    >
      {panel.error && (
        <div role="alert" className="shrink-0 border-b p-3 text-sm text-destructive">
          {panel.error}
          {panel.restartBlocked && !panel.isRestarting && (
            <Button
              variant="outline"
              className="ml-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
              onClick={() => void panel.refreshRestart()}
            >
              {t("configChat:refreshSessionStatus")}
            </Button>
          )}
        </div>
      )}
      <ConfigChatPanelBody panel={panel} />
    </ChatPopoverShell>
  );
});
