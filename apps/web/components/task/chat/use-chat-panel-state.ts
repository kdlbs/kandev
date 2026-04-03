"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { getLocalStorage } from "@/lib/local-storage";
import { useLayoutStore } from "@/lib/state/layout-store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { useSessionMessages } from "@/hooks/domains/session/use-session-messages";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { useSessionState } from "@/hooks/domains/session/use-session-state";
import { useSessionMcp } from "@/hooks/domains/session/use-session-mcp";
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
import { useAutoDisablePlanMode, usePlanLayoutHandlers } from "./use-plan-mode-helpers";
import type { ContextItem } from "@/lib/types/context";
import type { DiffComment } from "@/lib/diff/types";
import type { PlanComment, PRFeedbackComment } from "@/lib/state/slices/comments";
import type { ActiveDocument } from "@/lib/state/slices/ui/types";
import type { BuiltInPreset } from "@/lib/state/layout-manager/presets";

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

/** Re-focus chat input after dockview layout rebuild. */
function useRefocusChatAfterLayout() {
  return useCallback(() => {
    const unsub = useDockviewStore.subscribe((state) => {
      if (!state.isRestoringLayout) {
        unsub();
        requestAnimationFrame(() => {
          document.querySelector<HTMLElement>(".tiptap.ProseMirror")?.focus();
        });
      }
    });
  }, []);
}

type AutoApplyPlanLayoutOpts = {
  resolvedSessionId: string | null;
  taskId: string | null;
  sessionMetaPlanMode: boolean;
  currentStepHasPlanMode: boolean;
  setActiveDocument: (sid: string, doc: ActiveDocument | null) => void;
  applyBuiltInPreset: (preset: BuiltInPreset) => void;
  setPlanMode: (sid: string, enabled: boolean) => void;
  addContextFile: (sid: string, file: { path: string; name: string }) => void;
};

/** Auto-apply plan layout when session metadata has plan_mode enabled AND the current
 *  step actually configures enable_plan_mode. The step check guards against stale
 *  sessionMetaPlanMode from deepMerge hydration preserving deleted metadata keys. */
function useAutoApplyPlanLayout(opts: AutoApplyPlanLayoutOpts) {
  const {
    resolvedSessionId,
    taskId,
    sessionMetaPlanMode,
    currentStepHasPlanMode,
    setActiveDocument,
    applyBuiltInPreset,
    setPlanMode,
    addContextFile,
  } = opts;
  const autoAppliedPlanSessionRef = useRef<string | null>(null);
  useEffect(() => {
    if (!resolvedSessionId || !taskId) return;
    // Reset the guard when plan mode is disabled so future plan-mode steps
    // in the same session can be auto-applied (e.g. after proceeding away and back).
    if (!sessionMetaPlanMode && autoAppliedPlanSessionRef.current === resolvedSessionId) {
      autoAppliedPlanSessionRef.current = null;
      return;
    }
    if (autoAppliedPlanSessionRef.current === resolvedSessionId) return;
    // Only auto-apply if both the session metadata AND the current step agree on plan mode.
    // sessionMetaPlanMode can be stale (deepMerge hydration preserves deleted keys),
    // so we cross-check with the step's actual configuration.
    if (sessionMetaPlanMode && currentStepHasPlanMode) {
      autoAppliedPlanSessionRef.current = resolvedSessionId;
      setActiveDocument(resolvedSessionId, { type: "plan", taskId });
      applyBuiltInPreset("plan");
      setPlanMode(resolvedSessionId, true);
      addContextFile(resolvedSessionId, { path: PLAN_CONTEXT_PATH, name: "Plan" });
    }
  }, [
    resolvedSessionId,
    taskId,
    sessionMetaPlanMode,
    currentStepHasPlanMode,
    setActiveDocument,
    applyBuiltInPreset,
    setPlanMode,
    addContextFile,
  ]);
}

export function usePlanMode(resolvedSessionId: string | null, taskId: string | null) {
  const activeDocument = useAppStore((state) =>
    resolvedSessionId
      ? (state.documentPanel.activeDocumentBySessionId[resolvedSessionId] ?? null)
      : null,
  );
  const closeDocument = useLayoutStore((state) => state.closeDocument);
  const setActiveDocument = useAppStore((state) => state.setActiveDocument);
  const setPlanMode = useAppStore((state) => state.setPlanMode);
  const addContextFile = useContextFilesStore((s) => s.addFile);
  const removeContextFile = useContextFilesStore((s) => s.removeFile);
  const applyBuiltInPreset = useDockviewStore((s) => s.applyBuiltInPreset);

  const planModeFromStore = useAppStore((state) =>
    resolvedSessionId ? (state.chatInput.planModeBySessionId[resolvedSessionId] ?? false) : false,
  );
  const sessionMetaPlanMode = useAppStore((state) =>
    resolvedSessionId
      ? state.taskSessions.items[resolvedSessionId]?.metadata?.plan_mode === true
      : false,
  );
  const currentStepHasPlanMode = useAppStore((s) => {
    if (!taskId) return false;
    const task = s.kanban.tasks.find((t) => t.id === taskId);
    const stepId = task?.workflowStepId;
    if (!stepId) return false;
    const step = s.kanban.steps.find((st) => st.id === stepId);
    return step?.events?.on_enter?.some((a) => a.type === "enable_plan_mode") ?? false;
  });
  const planModeEnabled = planModeFromStore;
  const planLayoutVisible = activeDocument?.type === "plan";

  useEffect(() => {
    if (!resolvedSessionId) return;
    const stored = getLocalStorage(`plan-mode-${resolvedSessionId}`, false);
    if (stored) {
      setPlanMode(resolvedSessionId, true);
      addContextFile(resolvedSessionId, { path: PLAN_CONTEXT_PATH, name: "Plan" });
    }
  }, [resolvedSessionId, setPlanMode, addContextFile]);

  useAutoApplyPlanLayout({
    resolvedSessionId,
    taskId,
    sessionMetaPlanMode,
    currentStepHasPlanMode,
    setActiveDocument,
    applyBuiltInPreset,
    setPlanMode,
    addContextFile,
  });

  useAutoDisablePlanMode({
    resolvedSessionId,
    taskId,
    sessionMetaPlanMode,
    planModeFromStore,
    applyBuiltInPreset,
    closeDocument,
    setActiveDocument,
    setPlanMode,
    removeContextFile,
  });

  const refocusChatAfterLayout = useRefocusChatAfterLayout();
  const { togglePlanLayout, handlePlanModeChange } = usePlanLayoutHandlers({
    resolvedSessionId,
    taskId,
    setActiveDocument,
    applyBuiltInPreset,
    closeDocument,
    setPlanMode,
    addContextFile,
    removeContextFile,
    refocusChatAfterLayout,
  });

  return {
    planModeEnabled,
    planLayoutVisible,
    activeDocument,
    handlePlanModeChange,
    togglePlanLayout,
  };
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
  // Filter pending comments by sessionId to prevent cross-session leakage
  const planComments = usePendingPlanComments(resolvedSessionId);
  const pendingCommentsByFile = usePendingDiffCommentsByFile(resolvedSessionId);
  const pendingPRFeedback = usePendingPRFeedback(resolvedSessionId);
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
    if (!resolvedSessionId) return;
    const state = useCommentsStore.getState();
    const allPending = [...state.pendingForChat];
    for (const id of allPending) {
      const c = state.byId[id];
      // Only clear PR feedback for the current session
      if (c && isPRFeedbackComment(c) && c.sessionId === resolvedSessionId) {
        state.removeComment(id);
      }
    }
  }, [resolvedSessionId]);
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

type TodoStatus = "pending" | "in_progress" | "completed" | "failed";

function useSessionTodoItems(
  resolvedSessionId: string | null,
  messageTodos: Array<{ text: string; done?: boolean }>,
) {
  const storeTodos = useAppStore((s) =>
    resolvedSessionId ? s.sessionTodos.bySessionId[resolvedSessionId] : undefined,
  );
  return useMemo(() => {
    if (storeTodos && storeTodos.length > 0) {
      return storeTodos.map((e: { description: string; status: string }) => ({
        text: e.description,
        done: e.status === "completed",
        status: e.status as TodoStatus,
      }));
    }
    return messageTodos;
  }, [storeTodos, messageTodos]);
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
  const { supportsMcp, mcpServers } = useSessionMcp(sessionState.session?.agent_profile_id);
  const planModeAvailable = supportsMcp;

  // When MCP is available: full plan mode toggle (layout + chat input state)
  // When MCP is unavailable: layout-only toggle (plan panel visible, but no plan context/border)
  const {
    handlePlanModeChange: rawHandlePlanModeChange,
    togglePlanLayout,
    planModeEnabled,
    planLayoutVisible,
  } = planMode;
  const guardedHandlePlanModeChange = useCallback(
    (enabled: boolean) => {
      if (planModeAvailable) {
        rawHandlePlanModeChange(enabled);
      } else {
        // Toggle based on current layout state, ignoring the passed value
        togglePlanLayout(!planLayoutVisible);
      }
    },
    [planModeAvailable, rawHandlePlanModeChange, togglePlanLayout, planLayoutVisible],
  );

  // Auto-disable plan mode if agent doesn't support MCP (e.g. started from create dialog).
  // Only clear state — do NOT call applyBuiltInPreset("default") because the layout
  // may have just been set via URL intent (?layout=plan) and we don't want to overwrite it.
  const hasAgentProfile = Boolean(sessionState.session?.agent_profile_id);
  const setPlanMode = useAppStore((s) => s.setPlanMode);
  const removeCtxFile = useContextFilesStore((s) => s.removeFile);
  const hasAutoDisabled = useRef(false);
  useEffect(() => {
    if (
      planModeEnabled &&
      hasAgentProfile &&
      !planModeAvailable &&
      resolvedSessionId &&
      !hasAutoDisabled.current
    ) {
      hasAutoDisabled.current = true;
      setPlanMode(resolvedSessionId, false);
      removeCtxFile(resolvedSessionId, PLAN_CONTEXT_PATH);
    }
    if (!planModeEnabled) hasAutoDisabled.current = false;
  }, [
    planModeEnabled,
    hasAgentProfile,
    planModeAvailable,
    resolvedSessionId,
    setPlanMode,
    removeCtxFile,
  ]);

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

  const todoItems = useSessionTodoItems(resolvedSessionId, sessionData.todoItems);

  return {
    ...sessionState,
    ...planMode,
    handlePlanModeChange: guardedHandlePlanModeChange,
    ...contextFilesState,
    ...sessionData,
    ...comments,
    contextItems,
    planContextEnabled,
    planModeAvailable,
    mcpServers,
    prompts,
    todoItems,
  };
}
