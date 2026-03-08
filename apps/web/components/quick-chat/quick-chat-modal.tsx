"use client";

import { memo, useCallback, useState } from "react";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconMessageCircle, IconPlus, IconX } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { startQuickChat } from "@/lib/api/domains/workspace-api";
import { QuickChatContent } from "./quick-chat-content";

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
            <button
              key={s.sessionId || `new-${index}`}
              onClick={() => onTabChange(s.sessionId)}
              className={`flex items-center gap-1.5 px-2.5 py-1 text-xs rounded transition-colors cursor-pointer whitespace-nowrap ${
                isActive
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:bg-muted"
              }`}
            >
              <span className="truncate max-w-[100px]">{tabName}</span>
              <IconX
                className="h-3 w-3 opacity-60 hover:opacity-100 cursor-pointer"
                onClick={(e) => {
                  e.stopPropagation();
                  onTabClose(s.sessionId);
                }}
              />
            </button>
          );
        })}
      </div>
      <Button
        size="sm"
        variant="ghost"
        className="h-6 w-6 p-0 cursor-pointer shrink-0"
        onClick={onNewChat}
      >
        <IconPlus className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

function AgentPickerView({ onSelectAgent }: { onSelectAgent: (agentId: string) => void }) {
  const agentProfiles = useAppStore((s) => s.agentProfiles.items ?? []);

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
  const { toast } = useToast();
  const isOpen = useAppStore((s) => s.quickChat.isOpen);
  const sessions = useAppStore((s) => s.quickChat.sessions);
  const activeSessionId = useAppStore((s) => s.quickChat.activeSessionId);
  const closeQuickChat = useAppStore((s) => s.closeQuickChat);
  const closeQuickChatSession = useAppStore((s) => s.closeQuickChatSession);
  const setActiveQuickChatSession = useAppStore((s) => s.setActiveQuickChatSession);
  const renameQuickChatSession = useAppStore((s) => s.renameQuickChatSession);
  const openQuickChat = useAppStore((s) => s.openQuickChat);
  const agentProfiles = useAppStore((s) => s.agentProfiles.items ?? []);
  const taskSessions = useAppStore((s) => s.taskSessions.items || {});
  const [isCreating, setIsCreating] = useState(false);
  const [showAgentPicker, setShowAgentPicker] = useState(false);
  const [sessionToClose, setSessionToClose] = useState<string | null>(null);

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) {
        closeQuickChat();
        setShowAgentPicker(false);
      }
    },
    [closeQuickChat],
  );

  const handleNewChat = useCallback(() => {
    // Create a new tab with empty session ID (will show agent picker)
    openQuickChat("", workspaceId);
  }, [openQuickChat, workspaceId]);

  const handleSelectAgent = useCallback(
    async (agentId: string) => {
      if (isCreating) return;

      setIsCreating(true);
      try {
        // Get agent name for tab label
        const agent = agentProfiles.find((p) => p.id === agentId);
        const agentName = agent?.label || "Agent";

        // Generate initial tab name with agent name
        const sessionCount = sessions.filter((s) => s.sessionId !== "").length + 1;
        const initialName = `${agentName} - Chat ${sessionCount}`;

        // Create new session with selected agent and title
        const response = await startQuickChat(workspaceId, {
          agent_profile_id: agentId,
          title: initialName,
        });

        // Remove the empty placeholder tab if it exists
        if (activeSessionId === "") {
          closeQuickChatSession("");
        }

        // Open the new session
        openQuickChat(response.session_id, workspaceId);

        // Add session to state with the name
        renameQuickChatSession(response.session_id, initialName);

        setShowAgentPicker(false);
      } catch (error) {
        toast({
          title: "Failed to start quick chat",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      } finally {
        setIsCreating(false);
      }
    },
    [
      workspaceId,
      isCreating,
      activeSessionId,
      closeQuickChatSession,
      openQuickChat,
      toast,
      agentProfiles,
      sessions,
      renameQuickChatSession,
    ],
  );

  const handleCloseTab = useCallback(
    (sessionId: string) => {
      // Skip confirmation for empty sessions (agent picker tabs)
      if (sessionId === "") {
        closeQuickChatSession(sessionId);
        return;
      }
      // Show confirmation dialog
      setSessionToClose(sessionId);
    },
    [closeQuickChatSession],
  );

  const handleConfirmClose = useCallback(async () => {
    if (!sessionToClose) return;

    const sessionId = sessionToClose;
    setSessionToClose(null);

    // Get the task ID for this session before closing
    const session = taskSessions[sessionId];
    const taskId = session?.task_id;

    // Close the session in the UI first (immediate feedback)
    closeQuickChatSession(sessionId);

    // Delete the task (this will stop the agent and clean up)
    // The backend will:
    // 1. Stop the agent execution
    // 2. Delete the task from database
    // 3. Clean up the worktree
    if (taskId) {
      try {
        const { deleteTask } = await import("@/lib/api/domains/kanban-api");
        await deleteTask(taskId);
      } catch (error) {
        console.error("Failed to delete quick chat task:", error);
        toast({
          title: "Failed to delete quick chat",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      }
    }
  }, [sessionToClose, closeQuickChatSession, taskSessions, toast]);

  // Check if active session needs agent picker (empty session ID means new tab)
  const activeSessionNeedsAgent = activeSessionId === "" || showAgentPicker;

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
            <QuickChatContent sessionId={activeSessionId} />
          )}
          {activeSessionNeedsAgent && <AgentPickerView onSelectAgent={handleSelectAgent} />}
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={!!sessionToClose}
        onOpenChange={(open) => !open && setSessionToClose(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Quick Chat?</AlertDialogTitle>
            <AlertDialogDescription asChild>
              <div>
                <p>This will permanently delete this quick chat session, including:</p>
                <ul className="list-disc list-inside mt-2 space-y-1">
                  <li>All conversation history</li>
                  <li>The task and its data</li>
                  <li>The associated worktree</li>
                </ul>
                <p className="mt-2">This action cannot be undone.</p>
              </div>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleConfirmClose}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
});
