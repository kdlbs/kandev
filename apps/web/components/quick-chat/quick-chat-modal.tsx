"use client";

import { memo, type CSSProperties } from "react";
import { useShallow } from "zustand/react/shallow";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { IconPlus } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { PassthroughTerminal } from "@/components/task/passthrough-terminal";
import { QuickChatContent } from "./quick-chat-content";
import { QuickChatDeleteDialog } from "./quick-chat-delete-dialog";
import { QuickChatTabItem } from "./quick-chat-tab-item";
import { QuickChatSetup } from "./quick-chat-setup";
import { useQuickChatModal } from "./use-quick-chat-modal";
import { useQuickChatWidth } from "@/hooks/use-quick-chat-width";

type QuickChatModalProps = {
  workspaceId: string;
};

function QuickChatTabs({
  sessions,
  activeSessionId,
  onTabChange,
  onTabClose,
  onNewChat,
  onRename,
}: {
  sessions: Array<{ sessionId: string; workspaceId: string; name?: string }>;
  activeSessionId: string;
  onTabChange: (sessionId: string) => void;
  onTabClose: (sessionId: string) => void;
  onNewChat: () => void;
  onRename: (sessionId: string, name: string) => void;
}) {
  if (sessions.length === 0) return null;

  return (
    <div className="flex items-center gap-1 px-2 py-1 border-b bg-muted/20">
      <div className="flex items-center gap-1 overflow-x-auto flex-1 scrollbar-hide">
        {sessions.map((s, index) => {
          // Show "New Chat" for empty session IDs (agent picker tabs)
          const tabName = s.sessionId === "" ? "New Chat" : s.name || `Chat ${index + 1}`;
          return (
            <QuickChatTabItem
              key={s.sessionId || `new-${index}`}
              name={tabName}
              isActive={s.sessionId === activeSessionId}
              isRenameable={s.sessionId !== ""}
              onActivate={() => onTabChange(s.sessionId)}
              onClose={() => onTabClose(s.sessionId)}
              onRename={(name) => onRename(s.sessionId, name)}
            />
          );
        })}
        <Button
          size="sm"
          variant="ghost"
          className="h-6 w-6 p-0 cursor-pointer shrink-0"
          onClick={onNewChat}
          aria-label="Start new chat"
        >
          <IconPlus className="h-3.5 w-3.5" />
        </Button>
      </div>
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

function useIsQuickChatPassthrough(sessionId: string) {
  return useAppStore(
    useShallow((s) => {
      const session = s.taskSessions.items[sessionId];
      if (typeof session?.is_passthrough === "boolean") return session.is_passthrough;
      const profileId =
        session?.agent_profile_id ??
        s.quickChat.sessions.find((qs) => qs.sessionId === sessionId)?.agentProfileId;
      if (!profileId) return false;
      return s.agentProfiles.items.find((p) => p.id === profileId)?.cli_passthrough === true;
    }),
  );
}

function QuickChatSessionView({ sessionId }: { sessionId: string }) {
  const isPassthrough = useIsQuickChatPassthrough(sessionId);
  if (isPassthrough) {
    return (
      <div className="flex-1 min-h-0 overflow-hidden">
        <PassthroughTerminal key={sessionId} sessionId={sessionId} mode="agent" />
      </div>
    );
  }
  return <QuickChatContent sessionId={sessionId} />;
}

export const QuickChatModal = memo(function QuickChatModal({ workspaceId }: QuickChatModalProps) {
  const {
    isOpen,
    sessions,
    activeSessionId,
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
  } = useQuickChatModal(workspaceId);
  const { width, leftResizeHandleProps, rightResizeHandleProps } = useQuickChatWidth();
  const hasCreatedChat = sessions.some((session) => session.sessionId !== "");

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
          />
          {activeSessionId && !activeSessionNeedsAgent && (
            <QuickChatSessionView sessionId={activeSessionId} />
          )}
          {activeSessionNeedsAgent && (
            <QuickChatSetup
              key={`${workspaceId}:${setupKey}`}
              workspaceId={workspaceId}
              showIntroduction={!hasCreatedChat}
              pendingAgentId={pendingAgentId}
              onStart={handleSelectAgent}
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
