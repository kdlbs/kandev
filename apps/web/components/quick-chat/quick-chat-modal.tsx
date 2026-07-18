"use client";

import { memo, type CSSProperties } from "react";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { IconMessageCircle, IconPlus, IconSparkles, IconX } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { QuickChatDeleteDialog } from "./quick-chat-delete-dialog";
import { QuickChatSessionView } from "./quick-chat-session-view";
import { QuickChatTabItem } from "./quick-chat-tab-item";
import { QuickChatSetup } from "./quick-chat-setup";
import { useQuickChatModal } from "./use-quick-chat-modal";
import { useQuickChatWidth } from "@/hooks/use-quick-chat-width";
import { ConfigChatSetup } from "@/components/config-chat/config-chat-setup";
import { useConfigChat } from "@/components/config-chat/use-config-chat";
import type { QuickChatSession, QuickChatSessionKind } from "@/lib/state/slices/ui/types";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";

type QuickChatModalProps = {
  workspaceId: string;
};

function quickChatTabName(session: QuickChatSession, index: number) {
  if (!isQuickChatSetupSessionId(session.sessionId)) return session.name || `Chat ${index + 1}`;
  return session.kind === "config" ? "Configuration Chat" : "New Chat";
}

function QuickChatTabs({
  sessions,
  activeSessionId,
  onTabChange,
  onTabClose,
  onNewChat,
  onRename,
  onCloseModal,
}: {
  sessions: QuickChatSession[];
  activeSessionId: string;
  onTabChange: (sessionId: string) => void;
  onTabClose: (sessionId: string) => void;
  onNewChat: (kind: QuickChatSessionKind) => void;
  onRename: (sessionId: string, name: string) => void;
  onCloseModal: () => void;
}) {
  if (sessions.length === 0) return null;

  return (
    <div className="flex items-center gap-1 px-2 py-1 border-b bg-muted/20">
      <div className="flex items-center gap-1 overflow-x-auto flex-1 scrollbar-hide">
        {sessions.map((s, index) => {
          const tabName = quickChatTabName(s, index);
          return (
            <QuickChatTabItem
              key={s.sessionId || `new-${index}`}
              name={tabName}
              isActive={s.sessionId === activeSessionId}
              isRenameable={!isQuickChatSetupSessionId(s.sessionId)}
              kind={s.kind}
              onActivate={() => onTabChange(s.sessionId)}
              onClose={() => onTabClose(s.sessionId)}
              onRename={(name) => onRename(s.sessionId, name)}
            />
          );
        })}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              size="icon"
              variant="ghost"
              className="h-11 w-11 shrink-0 cursor-pointer sm:h-6 sm:w-6"
              aria-label="Start new chat"
            >
              <IconPlus className="h-4 w-4 sm:h-3.5 sm:w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent aria-label="New chat" align="start" className="w-52">
            <DropdownMenuItem
              onSelect={() => onNewChat("chat")}
              className="min-h-11 cursor-pointer gap-2"
            >
              <IconMessageCircle className="h-4 w-4" aria-hidden />
              Quick chat
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={() => onNewChat("config")}
              className="min-h-11 cursor-pointer gap-2"
            >
              <IconSparkles className="h-4 w-4" aria-hidden />
              Configuration chat
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {/* Touch devices have no Escape key or visible overlay to dismiss the
          full-screen dialog, so give them an explicit close control. */}
      <Button
        size="sm"
        variant="ghost"
        className="h-11 w-11 shrink-0 cursor-pointer p-0 sm:hidden"
        onClick={onCloseModal}
        aria-label="Close quick chat"
        data-testid="quick-chat-close"
      >
        <IconX className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

function QuickChatResizeHandle({
  edge,
  onMouseDown,
}: {
  edge: "left" | "right";
  onMouseDown: (event: React.MouseEvent) => void;
}) {
  return (
    <button
      type="button"
      tabIndex={-1}
      aria-label={`Resize quick chat from ${edge}`}
      data-testid={`quick-chat-resize-${edge}`}
      onMouseDown={onMouseDown}
      className={`group absolute inset-y-0 z-20 hidden w-2 cursor-ew-resize items-center justify-center sm:flex ${
        edge === "left" ? "left-0" : "right-0"
      }`}
    >
      <span
        className={`absolute inset-y-0 w-px bg-transparent transition-colors group-hover:bg-primary/60 ${
          edge === "left" ? "-left-px" : "-right-px"
        }`}
      />
    </button>
  );
}

export const QuickChatModal = memo(function QuickChatModal({ workspaceId }: QuickChatModalProps) {
  const configChat = useConfigChat(workspaceId);
  const {
    isOpen,
    sessions,
    activeSessionId,
    activeSession,
    sessionToClose,
    setupKey,
    activeSessionNeedsAgent,
    pendingAgentId,
    setActiveQuickChatSession,
    setSessionToClose,
    handleOpenChange,
    handleNewChat,
    handleSelectAgent,
    handleCloseTab,
    handleConfirmClose,
    handleRename,
  } = useQuickChatModal(workspaceId, configChat.reset);
  const setQuickChatInitialPrompt = useAppStore((state) => state.setQuickChatInitialPrompt);
  const { width, leftResizeHandleProps, rightResizeHandleProps } = useQuickChatWidth();
  const hasCreatedChat = sessions.some((session) => !isQuickChatSetupSessionId(session.sessionId));
  const setupKind =
    activeSession && isQuickChatSetupSessionId(activeSession.sessionId) ? activeSession.kind : null;
  return (
    <>
      <Dialog open={isOpen} onOpenChange={handleOpenChange}>
        <DialogContent
          className="!left-0 !top-0 !h-dvh !max-h-dvh !w-screen !max-w-none !translate-x-0 !translate-y-0 flex flex-col gap-0 p-0 shadow-2xl sm:!left-1/2 sm:!top-1/2 sm:!h-[85vh] sm:!max-h-[85vh] sm:!w-[var(--quick-chat-width)] sm:!max-w-[calc(100vw-2rem)] sm:!-translate-x-1/2 sm:!-translate-y-1/2"
          style={{ "--quick-chat-width": `${width}px` } as CSSProperties}
          showCloseButton={false}
          overlayClassName="bg-transparent"
        >
          <DialogTitle className="sr-only">Quick Chat</DialogTitle>
          <QuickChatResizeHandle edge="left" {...leftResizeHandleProps} />
          <QuickChatResizeHandle edge="right" {...rightResizeHandleProps} />
          <QuickChatTabs
            sessions={sessions}
            activeSessionId={activeSessionId || ""}
            onTabChange={setActiveQuickChatSession}
            onTabClose={handleCloseTab}
            onNewChat={handleNewChat}
            onRename={handleRename}
            onCloseModal={() => handleOpenChange(false)}
          />
          {activeSessionId && activeSession && !activeSessionNeedsAgent && (
            <QuickChatSessionView
              session={activeSession}
              onInitialPromptSent={() => setQuickChatInitialPrompt(activeSessionId, undefined)}
            />
          )}
          {activeSessionNeedsAgent && setupKind === "chat" && (
            <QuickChatSetup
              key={`${workspaceId}:${setupKey}`}
              workspaceId={workspaceId}
              showIntroduction={!hasCreatedChat}
              pendingAgentId={pendingAgentId}
              onStart={handleSelectAgent}
              onCancel={() => handleOpenChange(false)}
            />
          )}
          {activeSessionNeedsAgent && setupKind === "config" && (
            <ConfigChatSetup
              key={`${workspaceId}:config:${setupKey}`}
              defaultProfileId={configChat.defaultProfileId}
              isStarting={configChat.isStarting}
              error={configChat.error}
              onStart={(profileId, prompt) => configChat.startSession(profileId, prompt)}
              onCancel={() => handleOpenChange(false)}
            />
          )}
        </DialogContent>
      </Dialog>

      <QuickChatDeleteDialog
        sessionToDelete={sessionToClose}
        onOpenChange={(open) => !open && setSessionToClose(null)}
        onConfirm={handleConfirmClose}
      />
    </>
  );
});
