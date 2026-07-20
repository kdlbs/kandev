"use client";

import { useCallback, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkspaces } from "@/hooks/domains/workspace/use-workspaces";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { startConfigChat } from "@/lib/api/domains/workspace-api";
import { patchWorkspaceCache } from "@/lib/query/workspace-cache";
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

const activeWorkspaceStarts = new Map<string, symbol>();

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

function usePatchWorkspaceDefaults() {
  const queryClient = useQueryClient();
  return useCallback(
    (workspaceId: string, updates: Record<string, unknown>) => {
      patchWorkspaceCache(queryClient, workspaceId, updates);
    },
    [queryClient],
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
  patchWorkspaceDefaults: (workspaceId: string, updates: Record<string, unknown>) => void,
) {
  if (alreadyConfigured) return;
  try {
    const updates = { default_config_agent_profile_id: agentProfileId };
    await updateWorkspaceAction(workspaceId, updates);
    patchWorkspaceDefaults(workspaceId, updates);
  } catch {
    // The chat is usable even when saving the future default fails.
  }
}

export function useConfigChat(workspaceId: string) {
  const store = useConfigChatStore();
  const storeApi = useAppStoreApi();
  const patchWorkspaceDefaults = usePatchWorkspaceDefaults();
  const { agentProfiles } = useSettingsData(true);
  const { items: workspaces } = useWorkspaces();
  const [isStarting, setIsStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const latestRequestId = useRef(0);
  const activeRequestId = useRef<number | null>(null);
  const workspace = workspaces.find((item) => item.id === workspaceId) ?? null;
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
      const profile = agentProfiles.find((item) => item.id === agentProfileId);
      if (!profile) {
        setError("The selected agent profile is not available yet. Try again shortly.");
        return undefined;
      }
      if (activeWorkspaceStarts.has(workspaceId)) return undefined;
      const requestId = ++latestRequestId.current;
      const workspaceStart = Symbol(workspaceId);
      activeRequestId.current = requestId;
      activeWorkspaceStarts.set(workspaceId, workspaceStart);
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
          patchWorkspaceDefaults,
        );
        return response.session_id;
      } catch (err) {
        if (latestRequestId.current !== requestId) return undefined;
        setError(err instanceof Error ? err.message : "Unknown error");
        return undefined;
      } finally {
        if (activeWorkspaceStarts.get(workspaceId) === workspaceStart) {
          activeWorkspaceStarts.delete(workspaceId);
        }
        if (latestRequestId.current === requestId) {
          activeRequestId.current = null;
          setIsStarting(false);
        }
      }
    },
    [
      store,
      storeApi,
      agentProfiles,
      patchWorkspaceDefaults,
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
