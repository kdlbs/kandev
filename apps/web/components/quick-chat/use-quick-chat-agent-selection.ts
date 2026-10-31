"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  getQuickChatRetainedSessionFromError,
  startQuickChat,
  type QuickChatRepositoryInput,
} from "@/lib/api/domains/workspace-api";
import { ApiError } from "@/lib/api/client";
import {
  isTaskDeleteDirtyWorktreeError,
  isTaskDeletePreviewOutdatedError,
} from "@/lib/api/task-delete-errors";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import { recordAgentProfileRecentUseBestEffort } from "@/lib/agent-profile-recent-use";
import type { QuickChatOpeningPayload } from "@/lib/state/slices/ui/types";
import type { QuickChatStore } from "./use-quick-chat-modal";
import { isQuickChatStartupPending } from "./quick-chat-startup-state";

type StartQuickChatForAgentOptions = {
  workspaceId: string;
  agentId: string;
  store: QuickChatStore;
  repositories: QuickChatRepositoryInput[];
  openingPayload?: QuickChatOpeningPayload;
  creationId?: string;
};

function buildQuickChatStartInput({
  workspaceId,
  agentId,
  store,
  repositories,
  openingPayload,
  creationId,
}: StartQuickChatForAgentOptions) {
  const agent = store.agentProfiles.find((profile) => profile.id === agentId);
  const sessionCount =
    store.sessions.filter(
      (session) =>
        session.workspaceId === workspaceId &&
        (session.kind ?? "chat") === "chat" &&
        !isQuickChatSetupSessionId(session.sessionId),
    ).length + 1;
  // i18n-exempt: persisted as the quick-chat task title, same contract as use-config-chat.ts.
  const initialName = `${agent?.label || "Agent"} - Chat ${sessionCount}`;
  const request = {
    ...(creationId ? { creation_id: creationId } : {}),
    agent_profile_id: agentId,
    title: initialName,
    ...(store.agentGeneratedTaskTitles ? { auto_title: true } : {}),
    repositories: repositories.length > 0 ? repositories : undefined,
    ...(agent?.cli_passthrough && openingPayload
      ? {
          prompt: openingPayload.message,
          ...(openingPayload.attachments?.length
            ? { attachments: openingPayload.attachments }
            : {}),
        }
      : {}),
  };
  return { initialName, request };
}

async function startQuickChatForAgent(options: StartQuickChatForAgentOptions) {
  const { workspaceId, agentId, creationId } = options;
  const { initialName, request } = buildQuickChatStartInput(options);
  try {
    const response = await startQuickChat(workspaceId, request);
    return {
      sessionId: response.session_id,
      name: initialName,
      taskId: response.task_id,
      agentProfileId: response.agent_profile_id ?? agentId,
      creationId: response.creation_id,
      creationComplete: creationId
        ? response.creation_complete === true
        : (response.creation_complete ?? true),
      sessionState: response.session_state,
      archived: response.archived === true,
    };
  } catch (error) {
    if (creationId) throw error;
    const retained = getQuickChatRetainedSessionFromError(error);
    if (!retained) throw error;
    return { ...retained, name: initialName, agentProfileId: agentId };
  }
}

type QuickChatStartResult = Awaited<ReturnType<typeof startQuickChatForAgent>>;

function recordRecentAgentUse(store: QuickChatStore, agentProfileId: string): void {
  recordAgentProfileRecentUseBestEffort("quick_chat", agentProfileId, (record) =>
    store.applyAgentProfileRecentUse("quick_chat", record),
  );
}

function upsertIncompleteCreation(
  store: QuickChatStore,
  workspaceId: string,
  creationId: string,
  result: QuickChatStartResult,
): void {
  if (!result.sessionId || !result.taskId) return;
  store.upsertQuickChatSessionFromEvent({
    sessionId: result.sessionId,
    workspaceId,
    agentProfileId: result.agentProfileId,
    kind: "chat",
    taskId: result.taskId,
    creationId,
    startupPending: isQuickChatStartupPending({ sessionState: result.sessionState }),
  });
}

function reconcileSupersededStart(
  store: QuickChatStore,
  workspaceId: string,
  creationId: string | undefined,
  result: QuickChatStartResult,
): void {
  if (!result.sessionId || !result.taskId) return;
  store.upsertQuickChatSessionFromEvent({
    sessionId: result.sessionId,
    workspaceId,
    agentProfileId: result.agentProfileId,
    kind: "chat",
    taskId: result.taskId,
    name: result.name,
    creationId,
    startupPending: isQuickChatStartupPending({ sessionState: result.sessionState }),
  });
  recordRecentAgentUse(store, result.agentProfileId);
}

function activateQuickChatStart(
  options: StartQuickChatForAgentOptions & {
    setupSessionId: string | null;
    result: QuickChatStartResult & { sessionId: string; taskId: string };
  },
): void {
  const { workspaceId, agentId, store, openingPayload, creationId, setupSessionId, result } =
    options;
  recordRecentAgentUse(store, result.agentProfileId);
  if (setupSessionId && isQuickChatSetupSessionId(setupSessionId)) {
    store.closeQuickChatSession(setupSessionId);
  }
  if (creationId) {
    store.upsertQuickChatSessionFromEvent({
      sessionId: result.sessionId,
      workspaceId,
      agentProfileId: result.agentProfileId,
      kind: "chat",
      taskId: result.taskId,
      name: result.name,
      creationId,
    });
  }
  store.openQuickChat(result.sessionId, workspaceId, result.agentProfileId, "chat", result.taskId);
  const profile = store.agentProfiles.find((candidate) => candidate.id === agentId);
  if (openingPayload && !profile?.cli_passthrough) {
    store.setQuickChatInitialPrompt(result.sessionId, openingPayload);
  }
  store.renameQuickChatSession(result.sessionId, result.name);
}

function setQuickChatSetupError(error: unknown): string | null {
  return error instanceof ApiError && error.status >= 400 && error.status < 500
    ? error.message
    : null;
}

function useQuickChatSelectionRequestState(workspaceId: string) {
  const [pendingAgentId, setPendingAgentId] = useState<string | null>(null);
  const [setupError, setSetupError] = useState<string | null>(null);
  const latestRequestId = useRef(0);
  const workspaceRef = useRef(workspaceId);

  useEffect(() => {
    if (workspaceRef.current === workspaceId) return;
    workspaceRef.current = workspaceId;
    latestRequestId.current += 1;
    setPendingAgentId(null);
    setSetupError(null);
  }, [workspaceId]);

  const reset = useCallback(() => {
    latestRequestId.current += 1;
    setPendingAgentId(null);
    setSetupError(null);
  }, []);
  return { pendingAgentId, setPendingAgentId, setupError, setSetupError, latestRequestId, reset };
}

export function useAgentSelection(workspaceId: string, store: QuickChatStore) {
  const { t } = useTranslation();
  const requestState = useQuickChatSelectionRequestState(workspaceId);
  const { pendingAgentId, setPendingAgentId, setupError, setSetupError, latestRequestId, reset } =
    requestState;
  const reportSetupError = useCallback(
    (error: unknown) => {
      if (isTaskDeletePreviewOutdatedError(error))
        return setSetupError(t("task:deletePreflightError"));
      if (isTaskDeleteDirtyWorktreeError(error))
        return setSetupError(t("task:deleteDirtyWorktreeDescription"));
      setSetupError(setQuickChatSetupError(error) ?? t("chat:unknownError"));
    },
    [setSetupError, t],
  );

  const handleSelectAgent = useCallback(
    async (
      agentId: string,
      repositories: QuickChatRepositoryInput[] = [],
      openingPayload?: QuickChatOpeningPayload,
      creationId?: string,
      onFailure?: (error: unknown) => void,
    ): Promise<boolean> => {
      const requestId = ++latestRequestId.current;
      const setupSessionId = store.activeSessionId;
      setSetupError(null);
      setPendingAgentId(agentId);
      try {
        const result = await startQuickChatForAgent({
          workspaceId,
          agentId,
          store,
          repositories,
          openingPayload,
          creationId,
        });
        if (
          creationId &&
          (result.creationId !== creationId ||
            !result.creationComplete ||
            !result.sessionId ||
            result.archived)
        ) {
          if (!result.archived) upsertIncompleteCreation(store, workspaceId, creationId, result);
          return false;
        }
        if (!result.sessionId || !result.taskId) throw new Error(t("chat:unknownError"));
        if (latestRequestId.current !== requestId) {
          reconcileSupersededStart(store, workspaceId, creationId, result);
          return false;
        }
        activateQuickChatStart({
          workspaceId,
          agentId,
          store,
          repositories,
          openingPayload,
          creationId,
          setupSessionId,
          result: { ...result, sessionId: result.sessionId, taskId: result.taskId },
        });
        return true;
      } catch (error) {
        onFailure?.(error);
        if (latestRequestId.current !== requestId) return false;
        setSetupError(setQuickChatSetupError(error));
        return false;
      } finally {
        if (latestRequestId.current === requestId) setPendingAgentId(null);
      }
    },
    [latestRequestId, setPendingAgentId, setSetupError, store, t, workspaceId],
  );

  return { pendingAgentId, setupError, reportSetupError, reset, handleSelectAgent };
}
