"use client";

import { memo, useCallback, useEffect, useMemo, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { IconArrowsMaximize, IconPlus, IconSparkles, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { QuickChatDeleteDialog } from "@/components/quick-chat/quick-chat-delete-dialog";
import { QuickChatSessionView } from "@/components/quick-chat/quick-chat-session-view";
import { QuickChatTabItem } from "@/components/quick-chat/quick-chat-tab-item";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";
import { ConfigChatSetup } from "./config-chat-setup";
import { useConfigChat } from "./use-config-chat";

const SETUP_ID = "floating-config-chat-setup";

function useConfigChatPanelStore() {
  return useAppStore(
    useShallow((state) => ({
      quickChatSessions: state.quickChat.sessions,
      taskSessions: state.taskSessions.items,
      openQuickChat: state.openQuickChat,
      closeQuickChatSession: state.closeQuickChatSession,
      renameQuickChatSession: state.renameQuickChatSession,
      setQuickChatInitialPrompt: state.setQuickChatInitialPrompt,
    })),
  );
}

type ConfigChatPanelStore = ReturnType<typeof useConfigChatPanelStore>;

function useConfigChatSessionDeletion(store: ConfigChatPanelStore) {
  const { toast } = useToast();
  const [sessionToDelete, setSessionToDelete] = useState<string | null>(null);
  const handleConfirmDelete = useCallback(async () => {
    if (!sessionToDelete) return;
    const sessionId = sessionToDelete;
    const taskId = store.taskSessions[sessionId]?.task_id;
    setSessionToDelete(null);
    store.closeQuickChatSession(sessionId);
    if (!taskId) return;
    try {
      const { deleteTask } = await import("@/lib/api/domains/kanban-api");
      await deleteTask(taskId);
    } catch (error) {
      toast({
        title: "Failed to delete quick chat",
        description: error instanceof Error ? error.message : "Unknown error",
        variant: "error",
      });
    }
  }, [sessionToDelete, store, toast]);
  return { sessionToDelete, setSessionToDelete, handleConfirmDelete };
}

function ConfigChatTabs({
  sessions,
  activeSessionId,
  onActivate,
  onClose,
  onNew,
  onRename,
}: {
  sessions: QuickChatSession[];
  activeSessionId: string;
  onActivate: (sessionId: string) => void;
  onClose: (sessionId: string) => void;
  onNew: () => void;
  onRename: (sessionId: string, name: string) => void;
}) {
  if (sessions.length === 0) return null;
  return (
    <div className="flex shrink-0 items-center gap-1 border-b bg-muted/20 px-2 py-1">
      <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto scrollbar-hide">
        {sessions.map((session, index) => (
          <QuickChatTabItem
            key={session.sessionId}
            name={session.name || `Config ${index + 1}`}
            isActive={session.sessionId === activeSessionId}
            isRenameable
            kind="config"
            onActivate={() => onActivate(session.sessionId)}
            onClose={() => onClose(session.sessionId)}
            onRename={(name) => onRename(session.sessionId, name)}
          />
        ))}
      </div>
      <Button
        size="icon"
        variant="ghost"
        className="h-9 w-9 shrink-0 cursor-pointer"
        onClick={onNew}
        aria-label="Start new configuration chat"
      >
        <IconPlus className="h-4 w-4" />
      </Button>
    </div>
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
  const deletion = useConfigChatSessionDeletion(store);
  const [isOpen, setIsOpen] = useState(false);
  const [activeSessionId, setActiveSessionId] = useState(SETUP_ID);
  const sessions = useMemo(
    () =>
      store.quickChatSessions.filter(
        (session) => session.workspaceId === workspaceId && session.kind === "config",
      ),
    [store.quickChatSessions, workspaceId],
  );
  const activeSession = useMemo(
    () => sessions.find((session) => session.sessionId === activeSessionId),
    [activeSessionId, sessions],
  );

  useEffect(() => {
    if (activeSessionId === SETUP_ID || activeSession) return;
    setActiveSessionId(sessions[0]?.sessionId ?? SETUP_ID);
  }, [activeSession, activeSessionId, sessions]);

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (open && activeSessionId === SETUP_ID && sessions.length > 0) {
        setActiveSessionId(sessions[0].sessionId);
      }
      if (!open) chat.reset();
      setIsOpen(open);
    },
    [activeSessionId, chat.reset, sessions],
  );

  const handleStart = useCallback(
    async (profileId: string, prompt: string) => {
      const sessionId = await chat.startSession(profileId, prompt, { openInQuickChat: false });
      if (sessionId) setActiveSessionId(sessionId);
    },
    [chat.startSession],
  );

  const handleExpand = useCallback(() => {
    chat.reset();
    if (activeSession) {
      store.openQuickChat(
        activeSession.sessionId,
        workspaceId,
        activeSession.agentProfileId,
        "config",
      );
    } else {
      store.openQuickChat("", workspaceId, undefined, "config");
    }
    setIsOpen(false);
  }, [activeSession, chat.reset, store, workspaceId]);

  const handleNew = useCallback(() => {
    chat.reset();
    setActiveSessionId(SETUP_ID);
  }, [chat.reset]);

  return {
    ...chat,
    ...store,
    ...deletion,
    sessions,
    activeSession,
    activeSessionId,
    isOpen,
    setActiveSessionId,
    handleOpenChange,
    handleStart,
    handleExpand,
    handleNew,
  };
}

export const ConfigChatPanel = memo(function ConfigChatPanel({
  workspaceId,
}: {
  workspaceId: string;
}) {
  const panel = useConfigChatPanelController(workspaceId);

  return (
    <>
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
          <PanelHeader
            onExpand={panel.handleExpand}
            onClose={() => panel.handleOpenChange(false)}
          />
          <ConfigChatTabs
            sessions={panel.sessions}
            activeSessionId={panel.activeSessionId}
            onActivate={panel.setActiveSessionId}
            onClose={panel.setSessionToDelete}
            onNew={panel.handleNew}
            onRename={panel.renameQuickChatSession}
          />
          {panel.activeSession ? (
            <QuickChatSessionView
              session={panel.activeSession}
              onInitialPromptSent={() =>
                panel.setQuickChatInitialPrompt(panel.activeSession!.sessionId, undefined)
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
      <QuickChatDeleteDialog
        sessionToDelete={panel.sessionToDelete}
        onOpenChange={(open) => !open && panel.setSessionToDelete(null)}
        onConfirm={panel.handleConfirmDelete}
      />
    </>
  );
});
