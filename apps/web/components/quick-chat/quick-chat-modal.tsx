"use client";

import { memo } from "react";
import { useShallow } from "zustand/react/shallow";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { IconLoader2, IconMessageCircle, IconPlus } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { PassthroughTerminal } from "@/components/task/passthrough-terminal";
import { QuickChatContent } from "./quick-chat-content";
import { QuickChatDeleteDialog } from "./quick-chat-delete-dialog";
import { QuickChatTabItem } from "./quick-chat-tab-item";
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

function AgentPickerView({
  onSelectAgent,
  pendingAgentId,
}: {
  onSelectAgent: (agentId: string) => void;
  pendingAgentId: string | null;
}) {
  const agentProfiles = useAppStore((s) => s.agentProfiles.items) ?? [];
  const isLoading = pendingAgentId !== null;

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
          {agentProfiles.map((profile) => {
            const isPending = pendingAgentId === profile.id;
            return (
              <button
                key={profile.id}
                onClick={() => onSelectAgent(profile.id)}
                disabled={isLoading}
                className="group relative flex flex-col items-start gap-2 rounded-lg border p-4 text-left transition-all hover:border-primary hover:bg-accent cursor-pointer disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:border-border disabled:hover:bg-transparent"
              >
                <div className="flex items-center gap-2 w-full">
                  <div className="flex h-8 w-8 items-center justify-center rounded-md border bg-background">
                    {isPending ? (
                      <IconLoader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <IconMessageCircle className="h-4 w-4" />
                    )}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="font-medium text-sm truncate">{profile.label}</p>
                    <p className="text-xs text-muted-foreground truncate">
                      {isPending ? "Starting agent..." : profile.agent_name}
                    </p>
                  </div>
                </div>
              </button>
            );
          })}
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
            onRename={handleRename}
          />
          {activeSessionId && !activeSessionNeedsAgent && (
            <QuickChatSessionView sessionId={activeSessionId} />
          )}
          {activeSessionNeedsAgent && (
            <AgentPickerView onSelectAgent={handleSelectAgent} pendingAgentId={pendingAgentId} />
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
