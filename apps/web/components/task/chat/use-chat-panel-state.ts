"use client";

import { useCallback, useEffect, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { getLocalStorage } from "@/lib/local-storage";
import { useLayoutStore } from "@/lib/state/layout-store";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { useSessionMessages } from "@/hooks/domains/session/use-session-messages";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { useSessionState } from "@/hooks/domains/session/use-session-state";
import { useProcessedMessages } from "@/hooks/use-processed-messages";
import { useSessionModel } from "@/hooks/domains/session/use-session-model";
import { useQueue } from "@/hooks/domains/session/use-queue";
import { useContextFilesStore, type ContextFile } from "@/lib/state/context-files-store";
import { useCommentsStore, isPlanComment, isPRFeedbackComment } from "@/lib/state/slices/comments";
import { usePendingDiffCommentsByFile } from "@/hooks/domains/comments/use-diff-comments";
import {
  usePendingPlanComments,
  usePendingPRFeedback,
} from "@/hooks/domains/comments/use-pending-comments";
import { buildContextItems } from "../chat-context-items";
import type { ContextItem } from "@/lib/types/context";
import type { DiffComment } from "@/lib/diff/types";
import type { PlanComment, PRFeedbackComment } from "@/lib/state/slices/comments";

const EMPTY_CONTEXT_FILES: ContextFile[] = [];
const PLAN_CONTEXT_PATH = "plan:context";

export type CommentsState = {
  planComments: PlanComment[];
  pendingCommentsByFile: Record<string, DiffComment[]>;
  pendingPRFeedback: PRFeedbackComment[];
  markCommentsSent: (ids: string[]) => void;
  handleRemoveCommentFile: (filePath: string) => void;
  handleRemoveComment: (commentId: string) => void;
  handleRemovePRFeedback: (commentId: string) => void;
  handleClearPRFeedback: () => void;
  clearSessionPlanComments: () => void;
};

export function usePlanMode(resolvedSessionId: string | null, taskId: string | null) {
  const activeDocument = useAppStore((state) =>
    resolvedSessionId
      ? (state.documentPanel.activeDocumentBySessionId[resolvedSessionId] ?? null)
      : null,
  );
  const layoutBySession = useLayoutStore((state) => state.columnsBySessionId);
  const closeDocument = useLayoutStore((state) => state.closeDocument);
  const setActiveDocument = useAppStore((state) => state.setActiveDocument);
  const setPlanMode = useAppStore((state) => state.setPlanMode);
  const addContextFile = useContextFilesStore((s) => s.addFile);
  const { addPlan } = usePanelActions();

  const planModeEnabled = useMemo(() => {
    if (!resolvedSessionId || !activeDocument || activeDocument.type !== "plan") return false;
    const layout = layoutBySession[resolvedSessionId];
    return layout?.document === true;
  }, [resolvedSessionId, activeDocument, layoutBySession]);

  useEffect(() => {
    if (!resolvedSessionId) return;
    const stored = getLocalStorage(`plan-mode-${resolvedSessionId}`, false);
    if (stored) {
      setPlanMode(resolvedSessionId, true);
      addContextFile(resolvedSessionId, { path: PLAN_CONTEXT_PATH, name: "Plan", pinned: true });
    }
  }, [resolvedSessionId, setPlanMode, addContextFile]);

  const handlePlanModeChange = useCallback(
    (enabled: boolean) => {
      if (!resolvedSessionId || !taskId) return;
      if (enabled) {
        setActiveDocument(resolvedSessionId, { type: "plan", taskId });
        addPlan();
        setPlanMode(resolvedSessionId, true);
        addContextFile(resolvedSessionId, { path: PLAN_CONTEXT_PATH, name: "Plan", pinned: true });
      } else {
        closeDocument(resolvedSessionId);
        setActiveDocument(resolvedSessionId, null);
        setPlanMode(resolvedSessionId, false);
      }
    },
    [
      resolvedSessionId,
      taskId,
      setActiveDocument,
      addPlan,
      closeDocument,
      setPlanMode,
      addContextFile,
    ],
  );

  return { planModeEnabled, activeDocument, handlePlanModeChange };
}

export function useContextFiles(resolvedSessionId: string | null) {
  const contextFiles = useContextFilesStore((s) =>
    resolvedSessionId
      ? (s.filesBySessionId[resolvedSessionId] ?? EMPTY_CONTEXT_FILES)
      : EMPTY_CONTEXT_FILES,
  );
  const hydrateContextFiles = useContextFilesStore((s) => s.hydrateSession);
  const addContextFile = useContextFilesStore((s) => s.addFile);
  const toggleContextFile = useContextFilesStore((s) => s.toggleFile);
  const removeContextFile = useContextFilesStore((s) => s.removeFile);
  const unpinFile = useContextFilesStore((s) => s.unpinFile);
  const clearEphemeral = useContextFilesStore((s) => s.clearEphemeral);

  useEffect(() => {
    if (resolvedSessionId) hydrateContextFiles(resolvedSessionId);
  }, [resolvedSessionId, hydrateContextFiles]);

  const handleToggleContextFile = useCallback(
    (file: { path: string; name: string; pinned?: boolean }) => {
      if (resolvedSessionId) toggleContextFile(resolvedSessionId, file);
    },
    [resolvedSessionId, toggleContextFile],
  );

  const handleAddContextFile = useCallback(
    (file: { path: string; name: string; pinned?: boolean }) => {
      if (resolvedSessionId) addContextFile(resolvedSessionId, file);
    },
    [resolvedSessionId, addContextFile],
  );

  return {
    contextFiles,
    addContextFile,
    removeContextFile,
    unpinFile,
    clearEphemeral,
    handleToggleContextFile,
    handleAddContextFile,
  };
}

export function useCommentsState(resolvedSessionId: string | null): CommentsState {
  const hydrateComments = useCommentsStore((state) => state.hydrateSession);
  useEffect(() => {
    if (resolvedSessionId) hydrateComments(resolvedSessionId);
  }, [resolvedSessionId, hydrateComments]);
  const planComments = usePendingPlanComments();
  const pendingCommentsByFile = usePendingDiffCommentsByFile();
  const pendingPRFeedback = usePendingPRFeedback();
  const markCommentsSent = useCommentsStore((state) => state.markCommentsSent);
  const removeComment = useCommentsStore((state) => state.removeComment);
  const clearSessionPlanComments = useCallback(() => {
    const state = useCommentsStore.getState();
    const ids = resolvedSessionId ? state.bySession[resolvedSessionId] : undefined;
    if (!ids) return;
    for (const id of ids) {
      const c = state.byId[id];
      if (c && isPlanComment(c)) state.removeComment(id);
    }
  }, [resolvedSessionId]);
  const handleRemoveCommentFile = useCallback(
    (filePath: string) => {
      const comments = pendingCommentsByFile[filePath] || [];
      for (const comment of comments) removeComment(comment.id);
    },
    [pendingCommentsByFile, removeComment],
  );
  const handleRemoveComment = useCallback(
    (commentId: string) => removeComment(commentId),
    [removeComment],
  );
  const handleRemovePRFeedback = useCallback(
    (commentId: string) => removeComment(commentId),
    [removeComment],
  );
  const handleClearPRFeedback = useCallback(() => {
    const state = useCommentsStore.getState();
    const allPending = [...state.pendingForChat];
    for (const id of allPending) {
      const c = state.byId[id];
      if (c && isPRFeedbackComment(c)) state.removeComment(id);
    }
  }, []);
  return {
    planComments,
    pendingCommentsByFile,
    pendingPRFeedback,
    markCommentsSent,
    handleRemoveCommentFile,
    handleRemoveComment,
    handleRemovePRFeedback,
    handleClearPRFeedback,
    clearSessionPlanComments,
  };
}

type ChatContextItemsOptions = {
  planContextEnabled: boolean;
  contextFiles: ContextFile[];
  resolvedSessionId: string | null;
  removeContextFile: (sid: string, path: string) => void;
  unpinFile: (sid: string, path: string) => void;
  comments: CommentsState;
  taskId: string | null;
  onOpenFile?: (path: string) => void;
  onOpenFileAtLine?: (filePath: string) => void;
};

function useChatContextItems(opts: ChatContextItemsOptions) {
  const {
    planContextEnabled,
    contextFiles,
    resolvedSessionId,
    removeContextFile,
    unpinFile,
    comments,
    taskId,
    onOpenFile,
    onOpenFileAtLine,
  } = opts;
  const { addPlan } = usePanelActions();
  const { prompts } = useCustomPrompts();

  const promptsMap = useMemo(() => {
    const map = new Map<string, { content: string }>();
    for (const p of prompts) map.set(p.id, { content: p.content });
    return map;
  }, [prompts]);

  const contextItems = useMemo<ContextItem[]>(
    () =>
      buildContextItems({
        planContextEnabled,
        contextFiles,
        resolvedSessionId,
        removeContextFile,
        unpinFile,
        addPlan,
        promptsMap,
        onOpenFile,
        pendingCommentsByFile: comments.pendingCommentsByFile,
        handleRemoveCommentFile: comments.handleRemoveCommentFile,
        handleRemoveComment: comments.handleRemoveComment,
        onOpenFileAtLine,
        planComments: comments.planComments,
        handleClearPlanComments: comments.clearSessionPlanComments,
        pendingPRFeedback: comments.pendingPRFeedback,
        handleRemovePRFeedback: comments.handleRemovePRFeedback,
        handleClearPRFeedback: comments.handleClearPRFeedback,
        taskId,
      }),
    [
      planContextEnabled,
      contextFiles,
      resolvedSessionId,
      removeContextFile,
      unpinFile,
      addPlan,
      promptsMap,
      onOpenFile,
      comments.pendingCommentsByFile,
      comments.handleRemoveCommentFile,
      comments.handleRemoveComment,
      onOpenFileAtLine,
      comments.planComments,
      comments.clearSessionPlanComments,
      comments.pendingPRFeedback,
      comments.handleRemovePRFeedback,
      comments.handleClearPRFeedback,
      taskId,
    ],
  );

  return { contextItems, prompts };
}

function useSessionData(
  resolvedSessionId: string | null,
  session: ReturnType<typeof useSessionState>["session"],
  taskId: string | null,
  taskDescription: string | null,
) {
  const { messages, isLoading: messagesLoading } = useSessionMessages(resolvedSessionId);
  const processed = useProcessedMessages(messages, taskId, resolvedSessionId, taskDescription);
  const { sessionModel, activeModel } = useSessionModel(
    resolvedSessionId,
    session?.agent_profile_id,
  );
  const chatSubmitKey = useAppStore((state) => state.userSettings.chatSubmitKey);
  const agentCommands = useAppStore((state) =>
    resolvedSessionId ? state.availableCommands.bySessionId[resolvedSessionId] : undefined,
  );
  const {
    cancel: cancelQueue,
    updateContent: updateQueueContent,
    ...queueRest
  } = useQueue(resolvedSessionId);
  return {
    messages,
    messagesLoading,
    ...processed,
    sessionModel,
    activeModel,
    chatSubmitKey,
    agentCommands,
    cancelQueue,
    updateQueueContent,
    ...queueRest,
  };
}

export type UseChatPanelStateOptions = {
  sessionId: string | null;
  onOpenFile?: (path: string) => void;
  onOpenFileAtLine?: (filePath: string) => void;
};

export function useChatPanelState({
  sessionId,
  onOpenFile,
  onOpenFileAtLine,
}: UseChatPanelStateOptions) {
  const sessionState = useSessionState(sessionId);
  const { resolvedSessionId, taskId } = sessionState;
  const planMode = usePlanMode(resolvedSessionId, taskId);
  const contextFilesState = useContextFiles(resolvedSessionId);
  const { contextFiles, removeContextFile, unpinFile } = contextFilesState;
  const sessionData = useSessionData(
    resolvedSessionId,
    sessionState.session,
    taskId,
    sessionState.taskDescription,
  );
  const comments = useCommentsState(resolvedSessionId);

  const planContextEnabled = useMemo(
    () => contextFiles.some((f) => f.path === PLAN_CONTEXT_PATH),
    [contextFiles],
  );

  const { contextItems, prompts } = useChatContextItems({
    planContextEnabled,
    contextFiles,
    resolvedSessionId,
    removeContextFile,
    unpinFile,
    comments,
    taskId,
    onOpenFile,
    onOpenFileAtLine,
  });

  return {
    ...sessionState,
    ...planMode,
    ...contextFilesState,
    ...sessionData,
    ...comments,
    contextItems,
    planContextEnabled,
    prompts,
  };
}
