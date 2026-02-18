'use client';

import { useCallback, useEffect, useRef, useState, memo, useMemo } from 'react';
import { PanelRoot, PanelBody } from './panel-primitives';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore } from '@/components/state-provider';
import { getLocalStorage } from '@/lib/local-storage';
import { useLayoutStore } from '@/lib/state/layout-store';
import { usePanelActions } from '@/hooks/use-panel-actions';
import { useKeyboardShortcut } from '@/hooks/use-keyboard-shortcut';
import { SHORTCUTS } from '@/lib/keyboard/constants';
import { useSessionMessages } from '@/hooks/domains/session/use-session-messages';
import { useSettingsData } from '@/hooks/domains/settings/use-settings-data';
import { useCustomPrompts } from '@/hooks/domains/settings/use-custom-prompts';
import { useSessionState } from '@/hooks/domains/session/use-session-state';
import { useProcessedMessages } from '@/hooks/use-processed-messages';
import { useSessionModel } from '@/hooks/domains/session/use-session-model';
import { useMessageHandler } from '@/hooks/use-message-handler';
import { useQueue } from '@/hooks/domains/session/use-queue';
import { useContextFilesStore, type ContextFile } from '@/lib/state/context-files-store';
import { VirtualizedMessageList } from '@/components/task/chat/virtualized-message-list';
import { ChatInputContainer, type ChatInputContainerHandle, type MessageAttachment } from '@/components/task/chat/chat-input-container';
import { type QueuedMessageIndicatorHandle } from '@/components/task/chat/queued-message-indicator';
import { formatReviewCommentsAsMarkdown } from '@/lib/state/slices/comments/format';
import { useCommentsStore, isPlanComment } from '@/lib/state/slices/comments';
import { usePendingDiffCommentsByFile } from '@/hooks/domains/comments/use-diff-comments';
import { usePendingPlanComments } from '@/hooks/domains/comments/use-pending-comments';
import { getFileName } from '@/lib/utils/file-path';
import type { ContextItem } from '@/lib/types/context';
import { useIsTaskArchived } from './task-archived-context';
import type { DiffComment } from '@/lib/diff/types';
import type { PlanComment } from '@/lib/state/slices/comments';

const EMPTY_CONTEXT_FILES: ContextFile[] = [];
const PLAN_CONTEXT_PATH = 'plan:context';

/** Sort: pinned first, then by kind order, then by label */
const KIND_ORDER: Record<string, number> = {
  plan: 0,
  file: 1,
  prompt: 2,
  comment: 3,
  'plan-comment': 4,
  image: 5,
};

function contextItemSortFn(a: ContextItem, b: ContextItem): number {
  // Pinned first
  const aPinned = a.pinned ? 0 : 1;
  const bPinned = b.pinned ? 0 : 1;
  if (aPinned !== bPinned) return aPinned - bPinned;
  // Then by kind
  const aKind = KIND_ORDER[a.kind] ?? 99;
  const bKind = KIND_ORDER[b.kind] ?? 99;
  if (aKind !== bKind) return aKind - bKind;
  // Then by label
  return a.label.localeCompare(b.label);
}

type BuildContextItemsParams = {
  planContextEnabled: boolean;
  contextFiles: ContextFile[];
  resolvedSessionId: string | null;
  removeContextFile: (sid: string, path: string) => void;
  unpinFile: (sid: string, path: string) => void;
  addPlan: () => void;
  promptsMap: Map<string, { content: string }>;
  onOpenFile?: (path: string) => void;
  pendingCommentsByFile: Record<string, DiffComment[]>;
  handleRemoveCommentFile: (filePath: string) => void;
  handleRemoveComment: (commentId: string) => void;
  onOpenFileAtLine?: (filePath: string) => void;
  planComments: PlanComment[];
  handleClearPlanComments: () => void;
  taskId: string | null;
};

function buildPlanContextItem(params: BuildContextItemsParams): ContextItem | null {
  if (!params.planContextEnabled) return null;
  const { contextFiles, resolvedSessionId: sid, removeContextFile, unpinFile, addPlan, taskId } = params;
  const planFile = contextFiles.find(f => f.path === PLAN_CONTEXT_PATH);
  return {
    kind: 'plan',
    id: PLAN_CONTEXT_PATH,
    label: 'Plan',
    taskId: taskId ?? undefined,
    pinned: planFile?.pinned,
    onRemove: sid ? () => removeContextFile(sid, PLAN_CONTEXT_PATH) : undefined,
    onUnpin: planFile?.pinned && sid ? () => unpinFile(sid, PLAN_CONTEXT_PATH) : undefined,
    onOpen: addPlan,
  };
}

type FileItemHelpers = { sid: string | null; removeContextFile: (sid: string, path: string) => void; unpinFile: (sid: string, path: string) => void };

function makeRemoveHandler(sid: string | null, path: string, removeContextFile: (sid: string, path: string) => void) {
  return sid ? () => removeContextFile(sid, path) : undefined;
}
function makeUnpinHandler(pinned: boolean | undefined, sid: string | null, path: string, unpinFile: (sid: string, path: string) => void) {
  return pinned && sid ? () => unpinFile(sid, path) : undefined;
}

function buildPromptContextItem(f: ContextFile, helpers: FileItemHelpers, promptsMap: Map<string, { content: string }>): ContextItem {
  const prompt = promptsMap.get(f.path.replace('prompt:', ''));
  return {
    kind: 'prompt', id: f.path, label: f.name, pinned: f.pinned,
    onRemove: makeRemoveHandler(helpers.sid, f.path, helpers.removeContextFile),
    onUnpin: makeUnpinHandler(f.pinned, helpers.sid, f.path, helpers.unpinFile),
    promptContent: prompt?.content,
    onClick: () => {/* navigate to settings/prompts if desired */},
  };
}

function buildFileContextItem(f: ContextFile, helpers: FileItemHelpers, onOpenFile: ((path: string) => void) | undefined): ContextItem {
  return {
    kind: 'file', id: f.path, label: f.name, path: f.path, pinned: f.pinned,
    onRemove: makeRemoveHandler(helpers.sid, f.path, helpers.removeContextFile),
    onUnpin: makeUnpinHandler(f.pinned, helpers.sid, f.path, helpers.unpinFile),
    onOpen: onOpenFile ?? (() => {}),
  };
}

function buildFileAndPromptItems(params: BuildContextItemsParams): ContextItem[] {
  const { contextFiles, resolvedSessionId: sid, removeContextFile, unpinFile, promptsMap, onOpenFile } = params;
  const helpers: FileItemHelpers = { sid, removeContextFile, unpinFile };
  const items: ContextItem[] = [];
  for (const f of contextFiles) {
    if (f.path === PLAN_CONTEXT_PATH) continue;
    items.push(f.path.startsWith('prompt:')
      ? buildPromptContextItem(f, helpers, promptsMap)
      : buildFileContextItem(f, helpers, onOpenFile)
    );
  }
  return items;
}

function buildCommentItems(params: BuildContextItemsParams): ContextItem[] {
  const { pendingCommentsByFile, handleRemoveCommentFile, handleRemoveComment, onOpenFileAtLine } = params;
  const items: ContextItem[] = [];
  if (!pendingCommentsByFile) return items;
  for (const [filePath, comments] of Object.entries(pendingCommentsByFile)) {
    if (comments.length === 0) continue;
    const fileName = getFileName(filePath);
    items.push({
      kind: 'comment',
      id: `comment:${filePath}`,
      label: `${fileName} (${comments.length})`,
      filePath,
      comments,
      onRemove: () => handleRemoveCommentFile(filePath),
      onRemoveComment: (cid) => handleRemoveComment(cid),
      onOpen: onOpenFileAtLine ? () => onOpenFileAtLine(filePath) : undefined,
    });
  }
  return items;
}

function buildContextItems(params: BuildContextItemsParams): ContextItem[] {
  const items: ContextItem[] = [];
  const planItem = buildPlanContextItem(params);
  if (planItem) items.push(planItem);
  items.push(...buildFileAndPromptItems(params));
  items.push(...buildCommentItems(params));

  if (params.planComments.length > 0) {
    items.push({
      kind: 'plan-comment',
      id: 'plan-comments',
      label: `${params.planComments.length} plan comment${params.planComments.length !== 1 ? 's' : ''}`,
      comments: params.planComments,
      onRemove: params.handleClearPlanComments,
      onOpen: params.addPlan,
    });
  }

  return items.sort(contextItemSortFn);
}

type TaskChatPanelProps = {
  onSend?: (message: string) => void;
  sessionId?: string | null;
  onOpenFile?: (path: string) => void;
  showRequestChangesTooltip?: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  /** Callback to open a file at a specific line (for comment clicks) */
  onOpenFileAtLine?: (filePath: string) => void;
  /** Whether the dockview group containing this panel is focused */
  isPanelFocused?: boolean;
};

function resolveInputPlaceholder(isAgentBusy: boolean, activeDocumentType: string | undefined, planModeEnabled: boolean): string {
  if (isAgentBusy) return 'Queue instructions to the agent...';
  if (activeDocumentType === 'file') return 'Continue working on the file...';
  if (planModeEnabled) return 'Continue working on the plan...';
  return 'Continue working on the task...';
}

type UseChatPanelStateOptions = {
  sessionId: string | null;
  onOpenFile?: (path: string) => void;
  onOpenFileAtLine?: (filePath: string) => void;
};

function useChatPanelState({ sessionId, onOpenFile, onOpenFileAtLine }: UseChatPanelStateOptions) {
  const { resolvedSessionId, session, task, taskId, taskDescription, isStarting, isWorking, isAgentBusy, isFailed } = useSessionState(sessionId);
  const { planModeEnabled, activeDocument, handlePlanModeChange } = usePlanMode(resolvedSessionId, taskId);
  const { contextFiles, removeContextFile, unpinFile, clearEphemeral, handleToggleContextFile, handleAddContextFile } = useContextFiles(resolvedSessionId);
  const { addPlan } = usePanelActions();
  const { messages, isLoading: messagesLoading } = useSessionMessages(resolvedSessionId);
  const { allMessages, groupedItems, permissionsByToolCallId, childrenByParentToolCallId, todoItems, agentMessageCount, pendingClarification } = useProcessedMessages(messages, taskId, resolvedSessionId, taskDescription);
  const { sessionModel, activeModel } = useSessionModel(resolvedSessionId, session?.agent_profile_id);
  const chatSubmitKey = useAppStore((state) => state.userSettings.chatSubmitKey);
  const agentCommands = useAppStore((state) => resolvedSessionId ? state.availableCommands.bySessionId[resolvedSessionId] : undefined);
  const { queuedMessage, isQueued, cancel: cancelQueue, updateContent: updateQueueContent } = useQueue(resolvedSessionId);
  const hydrateComments = useCommentsStore((state) => state.hydrateSession);
  useEffect(() => {
    if (resolvedSessionId) hydrateComments(resolvedSessionId);
  }, [resolvedSessionId, hydrateComments]);
  const planComments = usePendingPlanComments();
  const pendingCommentsByFile = usePendingDiffCommentsByFile();
  const markCommentsSent = useCommentsStore((state) => state.markCommentsSent);
  const removeComment = useCommentsStore((state) => state.removeComment);
  const clearSessionPlanComments = useCallback(() => {
    // Remove all pending plan comments for this session
    const state = useCommentsStore.getState();
    const ids = resolvedSessionId ? state.bySession[resolvedSessionId] : undefined;
    if (!ids) return;
    for (const id of ids) {
      const c = state.byId[id];
      if (c && isPlanComment(c)) state.removeComment(id);
    }
  }, [resolvedSessionId]);
  const { prompts } = useCustomPrompts();

  const planContextEnabled = useMemo(() => contextFiles.some((f) => f.path === PLAN_CONTEXT_PATH), [contextFiles]);
  const promptsMap = useMemo(() => {
    const map = new Map<string, { content: string }>();
    for (const p of prompts) map.set(p.id, { content: p.content });
    return map;
  }, [prompts]);

  const handleRemoveCommentFile = useCallback((filePath: string) => {
    const comments = pendingCommentsByFile[filePath] || [];
    for (const comment of comments) removeComment(comment.id);
  }, [pendingCommentsByFile, removeComment]);

  const handleRemoveComment = useCallback((commentId: string) => {
    removeComment(commentId);
  }, [removeComment]);

  const contextItems = useMemo(() => buildContextItems({
    planContextEnabled, contextFiles, resolvedSessionId, removeContextFile, unpinFile, addPlan, promptsMap,
    onOpenFile, pendingCommentsByFile, handleRemoveCommentFile, handleRemoveComment, onOpenFileAtLine,
    planComments, handleClearPlanComments: clearSessionPlanComments, taskId,
  }), [planContextEnabled, contextFiles, resolvedSessionId, removeContextFile, unpinFile, addPlan, promptsMap, onOpenFile, pendingCommentsByFile, handleRemoveCommentFile, handleRemoveComment, onOpenFileAtLine, planComments, clearSessionPlanComments, taskId]);

  return {
    resolvedSessionId, session, task, taskId, taskDescription, isStarting, isWorking, isAgentBusy, isFailed,
    planModeEnabled, activeDocument, handlePlanModeChange,
    contextFiles, removeContextFile, unpinFile, clearEphemeral, handleToggleContextFile, handleAddContextFile,
    messages, messagesLoading, allMessages, groupedItems, permissionsByToolCallId, childrenByParentToolCallId,
    todoItems, agentMessageCount, pendingClarification,
    sessionModel, activeModel, chatSubmitKey, agentCommands, queuedMessage, isQueued, cancelQueue, updateQueueContent,
    planComments, pendingCommentsByFile, markCommentsSent, contextItems, planContextEnabled, prompts,
    clearSessionPlanComments, handleRemoveCommentFile, handleRemoveComment,
  };
}

type ChatInputAreaProps = {
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>;
  queuedMessageRef: React.RefObject<QueuedMessageIndicatorHandle | null>;
  clarificationKey: number;
  onClarificationResolved: () => void;
  handleSubmit: (message: string, reviewComments?: DiffComment[], attachments?: MessageAttachment[], inlineMentions?: ContextFile[]) => Promise<void>;
  handleCancelTurn: () => Promise<void>;
  handleCancelQueue: () => Promise<void>;
  handleQueueEditComplete: () => void;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  isPanelFocused?: boolean;
  panelState: ReturnType<typeof useChatPanelState>;
  isSending: boolean;
};

function ChatInputArea({ chatInputRef, queuedMessageRef, clarificationKey, onClarificationResolved, handleSubmit, handleCancelTurn, handleCancelQueue, handleQueueEditComplete, showRequestChangesTooltip, onRequestChangesTooltipDismiss, isPanelFocused, panelState, isSending }: ChatInputAreaProps) {
  const { resolvedSessionId, task, taskId, taskDescription, isStarting, isAgentBusy, isFailed, planModeEnabled, activeDocument, handlePlanModeChange, contextFiles, handleToggleContextFile, handleAddContextFile, pendingCommentsByFile, chatSubmitKey, agentCommands, isQueued, queuedMessage, updateQueueContent, contextItems, planContextEnabled, todoItems } = panelState;
  const placeholder = resolveInputPlaceholder(isAgentBusy, activeDocument?.type, planModeEnabled);
  return (
    <div className="bg-card flex-shrink-0 px-2 pb-2 pt-1">
      <ChatInputContainer
        ref={chatInputRef}
        key={clarificationKey}
        onSubmit={handleSubmit}
        sessionId={resolvedSessionId}
        taskId={taskId}
        taskTitle={task?.title}
        taskDescription={taskDescription ?? ''}
        planModeEnabled={planModeEnabled}
        onPlanModeChange={handlePlanModeChange}
        isAgentBusy={isAgentBusy}
        isStarting={isStarting}
        isSending={isSending}
        onCancel={handleCancelTurn}
        placeholder={placeholder}
        pendingClarification={panelState.pendingClarification}
        onClarificationResolved={onClarificationResolved}
        showRequestChangesTooltip={showRequestChangesTooltip}
        onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
        pendingCommentsByFile={pendingCommentsByFile}
        submitKey={chatSubmitKey}
        hasAgentCommands={!!(agentCommands && agentCommands.length > 0)}
        isFailed={isFailed}
        isQueued={isQueued}
        contextItems={contextItems}
        planContextEnabled={planContextEnabled}
        contextFiles={contextFiles}
        onToggleContextFile={handleToggleContextFile}
        onAddContextFile={handleAddContextFile}
        todoItems={todoItems}
        queuedMessage={queuedMessage?.content}
        onCancelQueue={handleCancelQueue}
        updateQueueContent={updateQueueContent}
        queuedMessageRef={queuedMessageRef}
        onQueueEditComplete={handleQueueEditComplete}
        isPanelFocused={isPanelFocused}
      />
    </div>
  );
}

function usePlanMode(resolvedSessionId: string | null, taskId: string | null) {
  const activeDocument = useAppStore((state) =>
    resolvedSessionId ? state.documentPanel.activeDocumentBySessionId[resolvedSessionId] ?? null : null
  );
  const layoutBySession = useLayoutStore((state) => state.columnsBySessionId);
  const closeDocument = useLayoutStore((state) => state.closeDocument);
  const setActiveDocument = useAppStore((state) => state.setActiveDocument);
  const setPlanMode = useAppStore((state) => state.setPlanMode);
  const addContextFile = useContextFilesStore((s) => s.addFile);
  const { addPlan } = usePanelActions();

  const planModeEnabled = useMemo(() => {
    if (!resolvedSessionId || !activeDocument || activeDocument.type !== 'plan') return false;
    const layout = layoutBySession[resolvedSessionId];
    return layout?.document === true;
  }, [resolvedSessionId, activeDocument, layoutBySession]);

  useEffect(() => {
    if (!resolvedSessionId) return;
    const stored = getLocalStorage(`plan-mode-${resolvedSessionId}`, false);
    if (stored) {
      setPlanMode(resolvedSessionId, true);
      addContextFile(resolvedSessionId, { path: PLAN_CONTEXT_PATH, name: 'Plan', pinned: true });
    }
  }, [resolvedSessionId, setPlanMode, addContextFile]);

  const handlePlanModeChange = useCallback((enabled: boolean) => {
    if (!resolvedSessionId || !taskId) return;
    if (enabled) {
      setActiveDocument(resolvedSessionId, { type: 'plan', taskId });
      addPlan();
      setPlanMode(resolvedSessionId, true);
      addContextFile(resolvedSessionId, { path: PLAN_CONTEXT_PATH, name: 'Plan', pinned: true });
    } else {
      closeDocument(resolvedSessionId);
      setActiveDocument(resolvedSessionId, null);
      setPlanMode(resolvedSessionId, false);
    }
  }, [resolvedSessionId, taskId, setActiveDocument, addPlan, closeDocument, setPlanMode, addContextFile]);

  return { planModeEnabled, activeDocument, handlePlanModeChange };
}

function useContextFiles(resolvedSessionId: string | null) {
  const contextFiles = useContextFilesStore((s) =>
    resolvedSessionId ? s.filesBySessionId[resolvedSessionId] ?? EMPTY_CONTEXT_FILES : EMPTY_CONTEXT_FILES
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

  const handleToggleContextFile = useCallback((file: { path: string; name: string; pinned?: boolean }) => {
    if (resolvedSessionId) toggleContextFile(resolvedSessionId, file);
  }, [resolvedSessionId, toggleContextFile]);

  const handleAddContextFile = useCallback((file: { path: string; name: string; pinned?: boolean }) => {
    if (resolvedSessionId) addContextFile(resolvedSessionId, file);
  }, [resolvedSessionId, addContextFile]);

  return { contextFiles, addContextFile, removeContextFile, unpinFile, clearEphemeral, handleToggleContextFile, handleAddContextFile };
}

function useSubmitHandler(panelState: ReturnType<typeof useChatPanelState>, onSend?: (message: string) => void) {
  const [isSending, setIsSending] = useState(false);
  const { resolvedSessionId, sessionModel, activeModel, planContextEnabled, isAgentBusy, activeDocument, planComments, contextFiles, prompts, markCommentsSent, clearSessionPlanComments, clearEphemeral } = panelState;
  const { handleSendMessage } = useMessageHandler({ resolvedSessionId, taskId: panelState.taskId, sessionModel, activeModel, planMode: planContextEnabled, isAgentBusy, activeDocument, planComments, contextFiles, prompts });

  const handleSubmit = useCallback(async (message: string, reviewComments?: DiffComment[], attachments?: MessageAttachment[], inlineMentions?: ContextFile[]) => {
    if (isSending) return;
    setIsSending(true);
    try {
      let finalMessage = message;
      if (reviewComments && reviewComments.length > 0) {
        const reviewMarkdown = formatReviewCommentsAsMarkdown(reviewComments);
        finalMessage = reviewMarkdown + (message ? message : '');
      }
      const hasReviewComments = !!(reviewComments && reviewComments.length > 0);
      if (onSend) { await onSend(finalMessage); } else { await handleSendMessage(finalMessage, attachments, hasReviewComments, inlineMentions); }
      if (reviewComments && reviewComments.length > 0) markCommentsSent(reviewComments.map((c) => c.id));
      if (planComments.length > 0) clearSessionPlanComments();
      if (resolvedSessionId) clearEphemeral(resolvedSessionId);
    } finally { setIsSending(false); }
  }, [isSending, onSend, handleSendMessage, markCommentsSent, planComments.length, clearSessionPlanComments, resolvedSessionId, clearEphemeral]);

  return { isSending, handleSubmit };
}

export const TaskChatPanel = memo(function TaskChatPanel({
  onSend,
  sessionId = null,
  onOpenFile,
  showRequestChangesTooltip = false,
  onRequestChangesTooltipDismiss,
  onOpenFileAtLine,
  isPanelFocused,
}: TaskChatPanelProps) {
  const isArchived = useIsTaskArchived();
  const lastAgentMessageCountRef = useRef(0);
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  const queuedMessageRef = useRef<QueuedMessageIndicatorHandle>(null);
  const [clarificationKey, setClarificationKey] = useState(0);

  useSettingsData(true);
  const panelState = useChatPanelState({ sessionId, onOpenFile, onOpenFileAtLine });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, onSend);
  const { resolvedSessionId, session, taskId, isWorking, messagesLoading, groupedItems, allMessages, permissionsByToolCallId, childrenByParentToolCallId, agentMessageCount, cancelQueue } = panelState;

  useEffect(() => { lastAgentMessageCountRef.current = agentMessageCount; }, [agentMessageCount]);

  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);

  const handleCancelTurn = useCallback(async () => {
    if (!resolvedSessionId) return;
    const client = getWebSocketClient();
    if (!client) return;
    try { await client.request('agent.cancel', { session_id: resolvedSessionId }, 15000); } catch (error) { console.error('Failed to cancel agent turn:', error); }
  }, [resolvedSessionId]);

  const handleCancelQueue = useCallback(async () => {
    try { await cancelQueue(); } catch (error) { console.error('Failed to cancel queued message:', error); }
  }, [cancelQueue]);

  const handleQueueEditComplete = useCallback(() => { chatInputRef.current?.focusInput(); }, []);

  useKeyboardShortcut(SHORTCUTS.FOCUS_INPUT, useCallback((event: KeyboardEvent) => {
    const el = document.activeElement;
    const isTyping = el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement || (el instanceof HTMLElement && el.isContentEditable);
    if (isTyping) return;
    const inputHandle = chatInputRef.current;
    if (inputHandle) { event.preventDefault(); inputHandle.focusInput(); }
  }, []), { enabled: true, preventDefault: false });

  return (
    <PanelRoot>
      <PanelBody padding={false}>
        <VirtualizedMessageList
          items={groupedItems}
          messages={allMessages}
          permissionsByToolCallId={permissionsByToolCallId}
          childrenByParentToolCallId={childrenByParentToolCallId}
          taskId={taskId ?? undefined}
          sessionId={resolvedSessionId}
          messagesLoading={messagesLoading}
          isWorking={isWorking}
          sessionState={session?.state}
          worktreePath={session?.worktree_path}
          onOpenFile={onOpenFile}
        />
      </PanelBody>
      {isArchived ? (
        <div className="bg-muted/50 flex-shrink-0 px-4 py-3 text-center text-sm text-muted-foreground border-t">
          This task is archived and read-only.
        </div>
      ) : (
        <ChatInputArea
          chatInputRef={chatInputRef}
          queuedMessageRef={queuedMessageRef}
          clarificationKey={clarificationKey}
          onClarificationResolved={handleClarificationResolved}
          handleSubmit={handleSubmit}
          handleCancelTurn={handleCancelTurn}
          handleCancelQueue={handleCancelQueue}
          handleQueueEditComplete={handleQueueEditComplete}
          showRequestChangesTooltip={showRequestChangesTooltip}
          onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
          isPanelFocused={isPanelFocused}
          panelState={panelState}
          isSending={isSending}
        />
      )}
    </PanelRoot>
  );
});
