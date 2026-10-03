"use client";

import { useCallback, useRef, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { t } from "@/lib/i18n";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { startConfigChat } from "@/lib/api/domains/workspace-api";
import { recordAgentProfileRecentUseBestEffort } from "@/lib/agent-profile-recent-use";
import { getQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import { persistQuickChatRename } from "@/lib/quick-chat/rename";
import { activeConfigChatOperations } from "./config-chat-operations";
import { useConfigChatRestart } from "./use-config-chat-restart";
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
  configuredProfile: boolean;
  updateWorkspaceInStore: (workspaceId: string, updates: Record<string, unknown>) => void;
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
  const { deleteTaskAfterUserAction } = await import("@/lib/api/domains/kanban-api");
  deleteTaskAfterUserAction(taskId).catch((error) =>
    console.error("Failed to clean up superseded config chat task:", error),
  );
}

async function registerStartedSession({
  store,
  storeApi,
  response,
  workspaceId,
  agentProfileId,
  prompt,
  isPassthrough,
  openInQuickChat,
  configuredProfile,
  updateWorkspaceInStore,
}: RegisterStartedSessionParams) {
  recordConfigChatProfileUse(agentProfileId, storeApi);
  const now = new Date().toISOString();
  storeApi.getState().setTaskSession({
    id: toSessionId(response.session_id),
    task_id: toTaskId(response.task_id),
    state: "CREATED",
    cancellation_pending: false,
    started_at: now,
    updated_at: now,
    agent_profile_id: toAgentProfileId(agentProfileId),
  });
  if (openInQuickChat) {
    store.closeQuickChatSession(getQuickChatSetupSessionId(workspaceId, "config"));
    store.openQuickChat(
      response.session_id,
      workspaceId,
      agentProfileId,
      "config",
      response.task_id,
    );
  } else {
    store.addQuickChatSession(
      response.session_id,
      workspaceId,
      agentProfileId,
      "config",
      response.task_id,
    );
  }
  // The config-chat endpoint has no title field, so name the tab from the
  // opening prompt and save it to the task — otherwise the label would live
  // only in this browser and be lost on the next resync.
  //
  // "Config Chat" is deliberately NOT translated: `persistQuickChatRename`
  // writes it to the task title, so translating it would store a
  // locale-dependent value that then renders unchanged after a locale switch,
  // on surfaces this directory does not own. Same call as the built-in layout
  // profile names and the seeded workflow step names.
  // i18n-exempt: persisted as the task title. See the comment above.
  const derivedName = prompt.slice(0, 40) || "Config Chat";
  store.renameQuickChatSession(response.session_id, derivedName);
  void persistQuickChatRename(response.session_id, response.task_id, derivedName).catch(
    () => undefined,
  );
  if (!isPassthrough) store.setQuickChatInitialPrompt(response.session_id, prompt);
  await saveDefaultConfigProfile(
    workspaceId,
    agentProfileId,
    configuredProfile,
    updateWorkspaceInStore,
  );
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

function recordConfigChatProfileUse(profileId: string, storeApi: AppStoreApi) {
  recordAgentProfileRecentUseBestEffort("config_chat", profileId, (record) =>
    storeApi.getState().applyAgentProfileRecentUse("config_chat", record),
  );
}

export function useConfigChat(workspaceId: string) {
  const restart = useConfigChatRestart(workspaceId);
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
    restart.resetRestart();
  }, [restart.resetRestart]);

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
        setError(t("configChat:profileUnavailable"));
        return undefined;
      }
      if (
        activeConfigChatOperations.has(workspaceId) ||
        storeApi.getState().quickChat.configChatRestarts[workspaceId]
      )
        return undefined;
      const requestId = ++latestRequestId.current;
      const workspaceStart = Symbol(workspaceId);
      activeRequestId.current = requestId;
      activeConfigChatOperations.set(workspaceId, workspaceStart);
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
        await registerStartedSession({
          store,
          storeApi,
          response,
          workspaceId,
          agentProfileId: response.agent_profile_id ?? agentProfileId,
          prompt,
          isPassthrough,
          openInQuickChat: options.openInQuickChat !== false,
          configuredProfile: Boolean(workspace?.default_config_agent_profile_id),
          updateWorkspaceInStore,
        });
        return response.session_id;
      } catch (err) {
        if (latestRequestId.current !== requestId) return undefined;
        // `err.message` is an API/network diagnostic and stays English by
        // design (docs/i18n.md, "the interpolated-value limit"); the FALLBACK
        // for a payload-less throw is our copy and is translated.
        setError(err instanceof Error ? err.message : t("configChat:unknownError"));
        return undefined;
      } finally {
        if (activeConfigChatOperations.get(workspaceId) === workspaceStart) {
          activeConfigChatOperations.delete(workspaceId);
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
      updateWorkspaceInStore,
      workspace?.default_config_agent_profile_id,
      workspaceId,
    ],
  );

  return {
    ...restart,
    isStarting,
    error: restart.restartError ?? error,
    defaultProfileId,
    reset,
    startSession,
  };
}
