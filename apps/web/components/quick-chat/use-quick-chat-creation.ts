"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import type { FileAttachment } from "@/components/task/chat/file-attachment";
import { ApiError } from "@/lib/api/client";
import { deleteTaskAfterUserAction } from "@/lib/api/domains/kanban-api";
import {
  getQuickChatCreation,
  type QuickChatCreationStatusResponse,
  type QuickChatRepositoryInput,
} from "@/lib/api/domains/workspace-api";
import {
  getChatDraftAttachments,
  getChatDraftText,
  setChatDraftAttachments,
  setChatDraftText,
} from "@/lib/local-storage";
import type { QuickChatOpeningPayload } from "@/lib/state/slices/ui/types";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import { isQuickChatStartupPending } from "./quick-chat-startup-state";
import { toQuickChatDraftAttachments } from "@/lib/state/slices/ui/quick-chat-opening-draft";
import {
  isCreationStatusResponse,
  pendingCreationPayload,
  QUICK_CHAT_RUNTIME_ID,
  type PendingQuickChatCreation,
} from "./quick-chat-creation";
import type { useQuickChatModal } from "./use-quick-chat-modal";
import type { useQuickChatSetupDraft } from "./use-quick-chat-setup-draft";

type SetupDraft = ReturnType<typeof useQuickChatSetupDraft>;
type QuickChatState = ReturnType<typeof useQuickChatModal>;
type QuickChatOwner = {
  isOpen: boolean;
  activeSessionId: string | null;
  workspaceId: string;
  userId: string;
  setupKey: number;
  generation: number;
};

export type QuickChatCreationActions = {
  pending: PendingQuickChatCreation | null;
  checking: boolean;
  start: (
    agentId: string,
    repositories: QuickChatRepositoryInput[],
    payload: QuickChatOpeningPayload,
    capturedDraft: { message: string; attachments: FileAttachment[] },
  ) => Promise<boolean>;
  check: (pending?: PendingQuickChatCreation) => Promise<void>;
  retry: () => Promise<boolean>;
  remove: () => Promise<void>;
};

type QuickChatCreationArgs = {
  workspaceId: string;
  userId: string;
  quickChat: Pick<
    QuickChatState,
    | "isOpen"
    | "activeSessionId"
    | "setupKey"
    | "sessions"
    | "handleSelectAgent"
    | "reportSetupError"
  >;
  setupDraft: SetupDraft;
};

function isDefiniteCreationRejection(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.errorCode === "quick_chat_creation_rejected" &&
    error.status >= 400 &&
    error.status < 500 &&
    error.status !== 408 &&
    error.status !== 429
  );
}

function ownsQuickChatSetup(owner: QuickChatOwner, current: QuickChatOwner): boolean {
  return (
    owner.isOpen &&
    owner.activeSessionId !== null &&
    isQuickChatSetupSessionId(owner.activeSessionId) &&
    current.isOpen &&
    current.activeSessionId === owner.activeSessionId &&
    current.workspaceId === owner.workspaceId &&
    current.userId === owner.userId &&
    current.generation === owner.generation
  );
}

function useQuickChatSetupOwner(args: {
  quickChat: Pick<QuickChatState, "isOpen" | "activeSessionId" | "setupKey">;
  workspaceId: string;
  userId: string;
}): React.MutableRefObject<QuickChatOwner> {
  const { quickChat, workspaceId, userId } = args;
  const activationRef = useRef<QuickChatOwner>({
    isOpen: quickChat.isOpen,
    activeSessionId: quickChat.activeSessionId,
    workspaceId,
    userId,
    setupKey: quickChat.setupKey,
    generation: 0,
  });
  useLayoutEffect(() => {
    const previous = activationRef.current;
    const changed =
      previous.isOpen !== quickChat.isOpen ||
      previous.activeSessionId !== quickChat.activeSessionId ||
      previous.workspaceId !== workspaceId ||
      previous.userId !== userId ||
      previous.setupKey !== quickChat.setupKey;
    activationRef.current = {
      isOpen: quickChat.isOpen,
      activeSessionId: quickChat.activeSessionId,
      workspaceId,
      userId,
      setupKey: quickChat.setupKey,
      generation: previous.generation + (changed ? 1 : 0),
    };
  }, [quickChat.activeSessionId, quickChat.isOpen, quickChat.setupKey, userId, workspaceId]);
  useLayoutEffect(
    () => () => {
      const previous = activationRef.current;
      activationRef.current = {
        ...previous,
        isOpen: false,
        generation: previous.generation + 1,
      };
    },
    [],
  );
  return activationRef;
}

function useQuickChatCreationStatus(args: QuickChatCreationArgs) {
  const { workspaceId, userId, quickChat, setupDraft } = args;
  const [checking, setChecking] = useState(false);
  const autoCheckedCreationRef = useRef<string | null>(null);
  const activeLookupsRef = useRef(new Set<string>());
  const activationRef = useQuickChatSetupOwner({ quickChat, workspaceId, userId });
  const openQuickChat = useAppStore((state) => state.openQuickChat);
  const upsertQuickChatSessionFromEvent = useAppStore(
    (state) => state.upsertQuickChatSessionFromEvent,
  );
  const agentProfiles = useAppStore((state) => state.agentProfiles?.items ?? []);
  const setQuickChatInitialPrompt = useAppStore((state) => state.setQuickChatInitialPrompt);
  const pendingCreation = setupDraft.pendingCreation;

  const check = useCallback(
    async (candidate?: PendingQuickChatCreation) => {
      const pending = candidate ?? pendingCreation;
      if (!pending || activeLookupsRef.current.has(pending.creationId)) return;
      autoCheckedCreationRef.current = pending.creationId;
      activeLookupsRef.current.add(pending.creationId);
      try {
        await lookupQuickChatCreation({
          workspaceId,
          userId,
          candidate: pending,
          pendingCreation,
          activationRef,
          setupDraft,
          agentProfiles,
          openQuickChat,
          upsertQuickChatSessionFromEvent,
          setQuickChatInitialPrompt,
          setChecking,
        });
      } finally {
        activeLookupsRef.current.delete(pending.creationId);
      }
    },
    [
      agentProfiles,
      openQuickChat,
      pendingCreation,
      setQuickChatInitialPrompt,
      setupDraft,
      upsertQuickChatSessionFromEvent,
      userId,
      workspaceId,
    ],
  );
  const pendingCreationSession = quickChat.sessions.find(
    (session) => session.creationId === pendingCreation?.creationId,
  );
  useEffect(() => {
    if (!pendingCreation) {
      autoCheckedCreationRef.current = null;
      return;
    }
    const restoredReceipt = pendingCreation.state !== "submitting";
    const receivedSession = pendingCreation.state === "submitting" && pendingCreationSession;
    if (
      (!restoredReceipt && !receivedSession) ||
      autoCheckedCreationRef.current === pendingCreation.creationId
    )
      return;
    autoCheckedCreationRef.current = pendingCreation.creationId;
    void check(pendingCreation);
  }, [check, pendingCreation, pendingCreationSession]);

  return { checking, check };
}

async function lookupQuickChatCreation(args: {
  workspaceId: string;
  userId: string;
  candidate?: PendingQuickChatCreation;
  pendingCreation: PendingQuickChatCreation | null;
  activationRef: React.MutableRefObject<QuickChatOwner>;
  setupDraft: SetupDraft;
  agentProfiles: Array<{ id: string; cli_passthrough?: boolean }>;
  openQuickChat: (...args: [string, string, string, "chat", string]) => void;
  upsertQuickChatSessionFromEvent: (args: {
    kind: "chat";
    sessionId: string;
    taskId: string;
    workspaceId: string;
    agentProfileId: string;
    creationId: string;
    startupPending: boolean;
  }) => void;
  setQuickChatInitialPrompt: (sessionId: string, payload: QuickChatOpeningPayload) => void;
  setChecking: (checking: boolean) => void;
}): Promise<void> {
  const pending = args.candidate ?? args.pendingCreation;
  if (!pending) return;
  const owner = args.activationRef.current;
  args.setChecking(true);
  try {
    const response: unknown = await getQuickChatCreation(args.workspaceId, pending.creationId);
    applyQuickChatCreationStatus(args, pending, owner, response);
  } catch {
    args.setupDraft.updatePendingCreation(pending.creationId, { state: "unknown" });
  } finally {
    args.setChecking(false);
  }
}

function applyQuickChatCreationStatus(
  args: Parameters<typeof lookupQuickChatCreation>[0],
  pending: PendingQuickChatCreation,
  owner: QuickChatOwner,
  response: unknown,
) {
  if (!isCreationStatusResponse(response, pending.creationId)) {
    args.setupDraft.updatePendingCreation(pending.creationId, { state: "unknown" });
    return;
  }
  const resolved = response as QuickChatCreationStatusResponse;
  const complete = resolved.creation_complete && Boolean(resolved.session_id) && !resolved.archived;
  const accepted = args.setupDraft.updatePendingCreation(pending.creationId, {
    state: complete ? "accepted" : "incomplete",
    taskId: resolved.task_id,
    sessionId: resolved.session_id,
    sessionState: resolved.session_state,
    archived: resolved.archived,
  });
  if (!accepted || !complete || !resolved.session_id) return;
  if (!ownsQuickChatSetup(owner, args.activationRef.current)) return;
  handOffAcceptedQuickChat(args, pending, resolved);
}

function handOffAcceptedQuickChat(
  args: Parameters<typeof lookupQuickChatCreation>[0],
  pending: PendingQuickChatCreation,
  accepted: QuickChatCreationStatusResponse,
) {
  const profileId = accepted.agent_profile_id ?? pending.agentProfileId;
  args.upsertQuickChatSessionFromEvent({
    kind: "chat",
    sessionId: accepted.session_id!,
    taskId: accepted.task_id,
    workspaceId: args.workspaceId,
    agentProfileId: profileId,
    creationId: pending.creationId,
    startupPending: isQuickChatStartupPending({ sessionState: accepted.session_state }),
  });
  const isPassthrough = args.agentProfiles.find(
    (profile) => profile.id === profileId,
  )?.cli_passthrough;
  if (pending.runtimeId === QUICK_CHAT_RUNTIME_ID && !pending.messageAttempted && !isPassthrough) {
    args.setQuickChatInitialPrompt(accepted.session_id!, pendingCreationPayload(pending));
  } else if (!isPassthrough) {
    restoreAcceptedDraft(accepted.session_id!, pending);
  }
  args.openQuickChat(accepted.session_id!, args.workspaceId, profileId, "chat", accepted.task_id);
  args.setupDraft.clearCapturedDraft(
    pending.message,
    toQuickChatDraftAttachments(pending.attachments),
  );
  args.setupDraft.clearPendingCreation(pending.creationId);
}

function restoreAcceptedDraft(sessionId: string, pending: PendingQuickChatCreation) {
  if (getChatDraftText(sessionId) || getChatDraftAttachments(sessionId).length > 0) return;
  setChatDraftText(sessionId, pending.message);
  setChatDraftAttachments(sessionId, toQuickChatDraftAttachments(pending.attachments));
}

function useQuickChatCreationSubmission(
  args: QuickChatCreationArgs,
  check: (pending?: PendingQuickChatCreation) => Promise<void>,
) {
  const { setupDraft, quickChat } = args;
  const {
    pendingCreation,
    beginPendingCreation,
    updatePendingCreation,
    clearPendingCreation,
    clearCapturedDraft,
  } = setupDraft;
  const { handleSelectAgent } = quickChat;
  const clearAcceptedDraft = useCallback(
    (
      pending: PendingQuickChatCreation,
      attachments: FileAttachment[] = toQuickChatDraftAttachments(pending.attachments),
    ) => clearCapturedDraft(pending.message, attachments),
    [clearCapturedDraft],
  );
  const handleFailure = useCallback(
    (pending: PendingQuickChatCreation, error: unknown) => {
      if (isDefiniteCreationRejection(error)) {
        clearPendingCreation(pending.creationId);
        return true;
      }
      updatePendingCreation(pending.creationId, { state: "unknown" });
      return false;
    },
    [clearPendingCreation, updatePendingCreation],
  );
  const submit = useCallback(
    async (
      pending: PendingQuickChatCreation,
      payload: QuickChatOpeningPayload,
      capturedAttachments?: FileAttachment[],
    ) => {
      let definiteFailure = false;
      const started = await handleSelectAgent(
        pending.agentProfileId,
        pending.repositories,
        payload,
        pending.creationId,
        (error) => {
          definiteFailure = handleFailure(pending, error);
        },
      );
      if (started) {
        clearAcceptedDraft(pending, capturedAttachments);
        clearPendingCreation(pending.creationId);
        return true;
      }
      if (!definiteFailure) {
        updatePendingCreation(pending.creationId, { state: "unknown" });
        await check(pending);
      }
      return false;
    },
    [
      check,
      clearAcceptedDraft,
      clearPendingCreation,
      handleFailure,
      handleSelectAgent,
      updatePendingCreation,
    ],
  );
  const start = useCallback(
    async (
      agentId: string,
      repositories: QuickChatRepositoryInput[],
      payload: QuickChatOpeningPayload,
      capturedDraft: { message: string; attachments: FileAttachment[] },
    ) => {
      const pending = beginPendingCreation(payload, agentId, repositories);
      if (!pending) return false;
      return submit(pending, payload, capturedDraft.attachments);
    },
    [beginPendingCreation, submit],
  );
  const retry = useCallback(async () => {
    if (!pendingCreation) return false;
    updatePendingCreation(pendingCreation.creationId, { state: "submitting" });
    return submit(pendingCreation, pendingCreationPayload(pendingCreation));
  }, [pendingCreation, submit, updatePendingCreation]);
  const remove = useCallback(async () => {
    const pending = pendingCreation;
    if (!pending?.taskId) return;
    try {
      await deleteTaskAfterUserAction(pending.taskId);
      clearPendingCreation(pending.creationId);
    } catch (error) {
      quickChat.reportSetupError(error);
    }
  }, [clearPendingCreation, pendingCreation, quickChat]);

  return { start, retry, remove };
}

export function useQuickChatCreation(args: QuickChatCreationArgs): QuickChatCreationActions {
  const status = useQuickChatCreationStatus(args);
  const submission = useQuickChatCreationSubmission(args, status.check);
  return useMemo(
    () => ({
      pending: args.setupDraft.pendingCreation,
      checking: status.checking,
      check: status.check,
      ...submission,
    }),
    [args.setupDraft.pendingCreation, status.check, status.checking, submission],
  );
}
