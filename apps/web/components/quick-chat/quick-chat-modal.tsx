"use client";

import { memo } from "react";
import { useShallow } from "zustand/react/shallow";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { IconMessageCircle, IconPlus, IconX } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { PassthroughTerminal } from "@/components/task/passthrough-terminal";
import { QuickChatContent } from "./quick-chat-content";
import { QuickChatDeleteDialog } from "./quick-chat-delete-dialog";
import { useQuickChatModal } from "./use-quick-chat-modal";

type QuickChatModalProps = {
  workspaceId: string;
};

function QuickChatTabs({
  sessions,
  activeSessionId,
  onTabChange,
  onTabClose,
  onNewChat,
}: {
  sessions: Array<{ sessionId: string; workspaceId: string; name?: string }>;
  activeSessionId: string;
  onTabChange: (sessionId: string) => void;
  onTabClose: (sessionId: string) => void;
  onNewChat: () => void;
}) {
  if (sessions.length === 0) return null;

  return (
    <div className="flex items-center gap-1 px-2 py-1 border-b bg-muted/20">
      <div className="flex items-center gap-1 overflow-x-auto flex-1 scrollbar-hide">
        {sessions.map((s, index) => {
          const isActive = s.sessionId === activeSessionId;
          // Show "New Chat" for empty session IDs (agent picker tabs)
          const tabName = s.sessionId === "" ? "New Chat" : s.name || `Chat ${index + 1}`;
          return (
            <div
              key={s.sessionId || `new-${index}`}
              className={`flex items-center gap-1 rounded transition-colors whitespace-nowrap ${
                isActive
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:bg-muted"
              }`}
            >
              <button
                type="button"
                onClick={() => onTabChange(s.sessionId)}
                className="flex items-center px-2.5 py-1 text-xs cursor-pointer"
              >
                <span className="truncate max-w-[100px]">{tabName}</span>
              </button>
              <button
                type="button"
                aria-label={`Close ${tabName}`}
                className="p-1 cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => onTabClose(s.sessionId)}
              >
                <IconX className="h-3 w-3" />
              </button>
            </div>
          );
        })}
      </div>
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

function AgentPickerView({ onSelectAgent }: { onSelectAgent: (agentId: string) => void }) {
  const agentProfiles = useAppStore((s) => s.agentProfiles.items) ?? [];

  return (
    <div className="flex-1 flex flex-col items-center justify-center p-8">
      <div className="max-w-2xl w-full space-y-6">
        <div className="text-center space-y-2">
          <h3 className="text-lg font-medium">Choose an agent to start chatting</h3>
          <p className="text-sm text-muted-foreground">
            Select an AI agent to begin your conversation
          </p>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {agentProfiles.map((profile) => (
            <button
              key={profile.id}
              onClick={() => onSelectAgent(profile.id)}
              className="group relative flex flex-col items-start gap-2 rounded-lg border p-4 text-left transition-all hover:border-primary hover:bg-accent cursor-pointer"
            >
              <div className="flex items-center gap-2 w-full">
                <div className="flex h-8 w-8 items-center justify-center rounded-md border bg-background">
                  <IconMessageCircle className="h-4 w-4" />
                </div>
                <div className="flex-1 min-w-0">
                  <p className="font-medium text-sm truncate">{profile.label}</p>
                  <p className="text-xs text-muted-foreground truncate">{profile.agent_name}</p>
                </div>
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

export const QuickChatModal = memo(function QuickChatModal({ workspaceId }: QuickChatModalProps) {
  const {
    isOpen,
    sessions,
    activeSessionId,
    sessionToClose,
    activeSessionNeedsAgent,
    setActiveQuickChatSession,
    setSessionToClose,
    handleOpenChange,
    handleNewChat,
    handleSelectAgent,
    handleCloseTab,
    handleConfirmClose,
  } = useQuickChatModal(workspaceId);

  return (
    <>
      <Dialog open={isOpen} onOpenChange={handleOpenChange}>
        <DialogContent
          className="!max-w-[80vw] !w-[80vw] max-h-[85vh] h-[85vh] p-0 gap-0 flex flex-col shadow-2xl"
          showCloseButton={false}
          overlayClassName="bg-transparent"
        >
          <DialogTitle className="sr-only">Quick Chat</DialogTitle>
          <QuickChatTabs
            sessions={sessions}
            activeSessionId={activeSessionId || ""}
            onTabChange={setActiveQuickChatSession}
            onTabClose={handleCloseTab}
            onNewChat={handleNewChat}
          />
          {activeSessionId && !activeSessionNeedsAgent && (
            <QuickChatSessionView sessionId={activeSessionId} />
          )}
          {activeSessionNeedsAgent && <AgentPickerView onSelectAgent={handleSelectAgent} />}
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
