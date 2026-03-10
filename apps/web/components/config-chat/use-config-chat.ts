"use client";

import { useCallback, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { startConfigChat } from "@/lib/api/domains/workspace-api";
import { updateWorkspaceAction } from "@/app/actions/workspaces";

function useConfigChatStore() {
  return useAppStore(
    useShallow((s) => ({
      isOpen: s.configChat.isOpen,
      sessionId: s.configChat.sessionId,
      taskId: s.configChat.taskId,
      workspaceId: s.configChat.workspaceId,
      openConfigChat: s.openConfigChat,
      closeConfigChat: s.closeConfigChat,
    })),
  );
}

export function useConfigChat(workspaceId: string) {
  const { toast } = useToast();
  const store = useConfigChatStore();
  const [isStarting, setIsStarting] = useState(false);

  const workspace = useAppStore(
    (s) => s.workspaces.items.find((w) => w.id === workspaceId) ?? null,
  );

  const defaultProfileId =
    workspace?.default_config_agent_profile_id ?? workspace?.default_agent_profile_id ?? undefined;

  /** Opens the modal. If there's an existing session for this workspace, re-opens it. */
  const open = useCallback(() => {
    if (store.sessionId && store.workspaceId === workspaceId) {
      store.openConfigChat(store.sessionId, store.taskId ?? "", workspaceId);
      return;
    }
    // Open the modal without a session — the empty state will handle profile selection
    store.openConfigChat("", "", workspaceId);
  }, [workspaceId, store]);

  /** Starts a config chat session with the given profile and optional prompt. */
  const startSession = useCallback(
    async (agentProfileId: string, prompt?: string) => {
      if (isStarting) return;
      setIsStarting(true);
      try {
        const response = await startConfigChat(workspaceId, {
          agent_profile_id: agentProfileId,
          prompt,
        });
        store.openConfigChat(response.session_id, response.task_id, workspaceId);

        // Save the selected profile as the workspace default for future sessions
        if (!workspace?.default_config_agent_profile_id) {
          try {
            await updateWorkspaceAction(workspaceId, {
              default_config_agent_profile_id: agentProfileId,
            });
          } catch {
            // Non-critical — don't fail the chat start for this
          }
        }
      } catch (error) {
        toast({
          title: "Failed to start config chat",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      } finally {
        setIsStarting(false);
      }
    },
    [workspaceId, isStarting, store, toast, workspace?.default_config_agent_profile_id],
  );

  const close = useCallback(() => {
    store.closeConfigChat();
  }, [store]);

  return {
    isOpen: store.isOpen,
    sessionId: store.sessionId,
    taskId: store.taskId,
    isStarting,
    workspace,
    defaultProfileId,
    open,
    startSession,
    close,
  };
}
