"use client";

import { useCallback, useRef, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { startConfigChat } from "@/lib/api/domains/workspace-api";
import { getQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import {
  agentProfileId as toAgentProfileId,
  sessionId as toSessionId,
  taskId as toTaskId,
} from "@/lib/types/ids";

type StartConfigChatOptions = {
  openInQuickChat?: boolean;
};

function useConfigChatStore() {
  return useAppStore(
    useShallow((state) => ({
      openQuickChat: state.openQuickChat,
      addQuickChatSession: state.addQuickChatSession,
      closeQuickChatSession: state.closeQuickChatSession,
      renameQuickChatSession: state.renameQuickChatSession,
      setQuickChatInitialPrompt: state.setQuickChatInitialPrompt,
    })),
  );
}

type ConfigChatStore = ReturnType<typeof useConfigChatStore>;
type AppStoreApi = ReturnType<typeof useAppStoreApi>;
type StartedConfigChat = Awaited<ReturnType<typeof startConfigChat>>;

type RegisterStartedSessionParams = {
  store: ConfigChatStore;
  storeApi: AppStoreApi;
  response: StartedConfigChat;
  workspaceId: string;
  agentProfileId: string;
  prompt: string;
  isPassthrough: boolean;
  openInQuickChat: boolean;
};

function useUpdateWorkspaceInStore() {
  const storeApi = useAppStoreApi();
  return useCallback(
    (workspaceId: string, updates: Record<string, unknown>) => {
      const { workspaces, setWorkspaces } = storeApi.getState();
      setWorkspaces(workspaces.items.map((w) => (w.id === workspaceId ? { ...w, ...updates } : w)));
    },
    [storeApi],
  );
}

async function deleteSupersededConfigChatTask(taskId: string) {
  const { deleteTask } = await import("@/lib/api/domains/kanban-api");
  deleteTask(taskId).catch((error) =>
    console.error("Failed to clean up superseded config chat task:", error),
  );
}

function registerStartedSession({
  store,
  storeApi,
  response,
  workspaceId,
  agentProfileId,
  prompt,
  isPassthrough,
  openInQuickChat,
}: RegisterStartedSessionParams) {
  const now = new Date().toISOString();
  storeApi.getState().setTaskSession({
    id: toSessionId(response.session_id),
    task_id: toTaskId(response.task_id),
    state: "CREATED",
    started_at: now,
    updated_at: now,
    agent_profile_id: toAgentProfileId(agentProfileId),
  });
  if (openInQuickChat) {
    store.closeQuickChatSession(getQuickChatSetupSessionId(workspaceId, "config"));
    store.openQuickChat(response.session_id, workspaceId, agentProfileId, "config");
  } else {
    store.addQuickChatSession(response.session_id, workspaceId, agentProfileId, "config");
  }
  store.renameQuickChatSession(response.session_id, prompt.slice(0, 40) || "Config Chat");
  if (!isPassthrough) store.setQuickChatInitialPrompt(response.session_id, prompt);
}

async function saveDefaultConfigProfile(
  workspaceId: string,
  agentProfileId: string,
  alreadyConfigured: boolean,
  updateWorkspaceInStore: (workspaceId: string, updates: Record<string, unknown>) => void,
) {
  if (alreadyConfigured) return;
  try {
    const updates = { default_config_agent_profile_id: agentProfileId };
    await updateWorkspaceAction(workspaceId, updates);
    updateWorkspaceInStore(workspaceId, updates);
  } catch {
    // The chat is usable even when saving the future default fails.
  }
}

export function useConfigChat(workspaceId: string) {
  const store = useConfigChatStore();
  const storeApi = useAppStoreApi();
  const updateWorkspaceInStore = useUpdateWorkspaceInStore();
  const [isStarting, setIsStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const latestRequestId = useRef(0);
  const activeRequestId = useRef<number | null>(null);
  const workspace = useAppStore(
    (state) => state.workspaces.items.find((item) => item.id === workspaceId) ?? null,
  );
  const defaultProfileId =
    workspace?.default_config_agent_profile_id ?? workspace?.default_agent_profile_id ?? undefined;

  const reset = useCallback(() => {
    latestRequestId.current += 1;
    activeRequestId.current = null;
    setIsStarting(false);
    setError(null);
  }, []);

  const startSession = useCallback(
    async (
      agentProfileId: string,
      prompt: string,
      options: StartConfigChatOptions = {},
    ): Promise<string | undefined> => {
      if (activeRequestId.current !== null) return undefined;
      const profile = storeApi
        .getState()
        .agentProfiles.items.find((item) => item.id === agentProfileId);
      if (!profile) {
        setError("The selected agent profile is not available yet. Try again shortly.");
        return undefined;
      }
      const requestId = ++latestRequestId.current;
      activeRequestId.current = requestId;
      setIsStarting(true);
      setError(null);
      try {
        const isPassthrough = profile.cli_passthrough === true;
        // ACP chats send through the subscribed shell so a fast turn cannot
        // finish before WS attaches. Passthrough chats render only a terminal.
        const response = await startConfigChat(workspaceId, {
          agent_profile_id: agentProfileId,
          ...(isPassthrough ? { prompt } : {}),
        });
        if (latestRequestId.current !== requestId) {
          await deleteSupersededConfigChatTask(response.task_id);
          return undefined;
        }
        registerStartedSession({
          store,
          storeApi,
          response,
          workspaceId,
          agentProfileId,
          prompt,
          isPassthrough,
          openInQuickChat: options.openInQuickChat !== false,
        });
        await saveDefaultConfigProfile(
          workspaceId,
          agentProfileId,
          Boolean(workspace?.default_config_agent_profile_id),
          updateWorkspaceInStore,
        );
        return response.session_id;
      } catch (err) {
        if (latestRequestId.current !== requestId) return undefined;
        setError(err instanceof Error ? err.message : "Unknown error");
        return undefined;
      } finally {
        if (latestRequestId.current === requestId) {
          activeRequestId.current = null;
          setIsStarting(false);
        }
      }
    },
    [
      store,
      storeApi,
      updateWorkspaceInStore,
      workspace?.default_config_agent_profile_id,
      workspaceId,
    ],
  );

  return {
    isStarting,
    error,
    defaultProfileId,
    reset,
    startSession,
  };
}
