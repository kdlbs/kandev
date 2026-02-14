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
import { useContextFilesStore } from '@/lib/state/context-files-store';
import { VirtualizedMessageList } from '@/components/task/chat/virtualized-message-list';
import { ChatInputContainer, type ChatInputContainerHandle, type MessageAttachment } from '@/components/task/chat/chat-input-container';
import { type QueuedMessageIndicatorHandle } from '@/components/task/chat/queued-message-indicator';
import { formatReviewCommentsAsMarkdown } from '@/components/task/chat/messages/review-comments-attachment';
import {
  useDiffCommentsStore,
  usePendingCommentsByFile,
} from '@/lib/state/slices/diff-comments';
import { getFileName } from '@/lib/utils/file-path';
import type { ContextItem } from '@/lib/types/context';
import type { DiffComment } from '@/lib/diff/types';
import type { DocumentComment } from '@/lib/state/slices/ui/types';

const EMPTY_COMMENTS: DocumentComment[] = [];
const EMPTY_CONTEXT_FILES: import('@/lib/state/context-files-store').ContextFile[] = [];

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

export const TaskChatPanel = memo(function TaskChatPanel({
  onSend,
  sessionId = null,
  onOpenFile,
  showRequestChangesTooltip = false,
  onRequestChangesTooltipDismiss,
  onOpenFileAtLine,
  isPanelFocused,
}: TaskChatPanelProps) {
  const [isSending, setIsSending] = useState(false);
  const lastAgentMessageCountRef = useRef(0);
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  const queuedMessageRef = useRef<QueuedMessageIndicatorHandle>(null);

  // Ensure agent profile data is loaded
  useSettingsData(true);

  // Custom prompts for context
  const { prompts } = useCustomPrompts();

  // Session state management
  const {
    resolvedSessionId,
    session,
    task,
    taskId,
    taskDescription,
    isStarting,
    isWorking,
    isAgentBusy,
    isFailed,
  } = useSessionState(sessionId);

  // Plan mode state
  const activeDocument = useAppStore((state) =>
    resolvedSessionId ? state.documentPanel.activeDocumentBySessionId[resolvedSessionId] ?? null : null
  );
  const layoutBySession = useLayoutStore((state) => state.columnsBySessionId);
  const closeDocument = useLayoutStore((state) => state.closeDocument);
  const setActiveDocument = useAppStore((state) => state.setActiveDocument);
  const setPlanMode = useAppStore((state) => state.setPlanMode);

  const planModeEnabled = useMemo(() => {
    if (!resolvedSessionId || !activeDocument || activeDocument.type !== 'plan') return false;
    const layout = layoutBySession[resolvedSessionId];
    return layout?.document === true;
  }, [resolvedSessionId, activeDocument, layoutBySession]);

  const { addPlan } = usePanelActions();

  // Context files store
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

  const handlePlanModeChange = useCallback(
    (enabled: boolean) => {
      if (!resolvedSessionId || !taskId) return;
      if (enabled) {
        setActiveDocument(resolvedSessionId, { type: 'plan', taskId });
        addPlan();
        setPlanMode(resolvedSessionId, true);
        addContextFile(resolvedSessionId, { path: 'plan:context', name: 'Plan', pinned: true });
      } else {
        closeDocument(resolvedSessionId);
        setActiveDocument(resolvedSessionId, null);
        setPlanMode(resolvedSessionId, false);
      }
    },
    [resolvedSessionId, taskId, setActiveDocument, addPlan, closeDocument, setPlanMode, addContextFile]
  );

  // Initialize plan mode from localStorage on mount
  useEffect(() => {
    if (resolvedSessionId) {
      const stored = getLocalStorage(`plan-mode-${resolvedSessionId}`, false);
      if (stored) {
        setPlanMode(resolvedSessionId, true);
        addContextFile(resolvedSessionId, { path: 'plan:context', name: 'Plan', pinned: true });
      }
    }
  }, [resolvedSessionId, setPlanMode, addContextFile]);

  const handleToggleContextFile = useCallback(
    (file: { path: string; name: string; pinned?: boolean }) => {
      if (resolvedSessionId) toggleContextFile(resolvedSessionId, file);
    },
    [resolvedSessionId, toggleContextFile]
  );

  const handleAddContextFile = useCallback(
    (file: { path: string; name: string; pinned?: boolean }) => {
      if (resolvedSessionId) addContextFile(resolvedSessionId, file);
    },
    [resolvedSessionId, addContextFile]
  );

  // Fetch messages for this session
  const { messages, isLoading: messagesLoading } = useSessionMessages(resolvedSessionId);

  // Process messages
  const { allMessages, groupedItems, permissionsByToolCallId, childrenByParentToolCallId, todoItems, agentMessageCount, pendingClarification } = useProcessedMessages(
    messages,
    taskId,
    resolvedSessionId,
    taskDescription
  );

  // Clarification state
  const [clarificationKey, setClarificationKey] = useState(0);
  const handleClarificationResolved = useCallback(() => {
    setClarificationKey((k) => k + 1);
  }, []);

  // Model management
  const { sessionModel, activeModel } = useSessionModel(
    resolvedSessionId,
    session?.agent_profile_id
  );

  // User settings
  const chatSubmitKey = useAppStore((state) => state.userSettings.chatSubmitKey);
  const agentCommands = useAppStore((state) =>
    resolvedSessionId ? state.availableCommands.bySessionId[resolvedSessionId] : undefined
  );
  const hasAgentCommands = agentCommands && agentCommands.length > 0;

  // Message queue
  const { queuedMessage, isQueued, cancel: cancelQueue, updateContent: updateQueueContent } = useQueue(resolvedSessionId);

  // Document comments from plan panel
  const documentComments = useAppStore((state) =>
    resolvedSessionId ? state.documentPanel.commentsBySessionId[resolvedSessionId] ?? EMPTY_COMMENTS : EMPTY_COMMENTS
  );
  const setDocumentComments = useAppStore((state) => state.setDocumentComments);

  const handleClearDocumentComments = useCallback(() => {
    if (resolvedSessionId) {
      setDocumentComments(resolvedSessionId, []);
    }
  }, [resolvedSessionId, setDocumentComments]);

  // Derive plan context from context files store
  const planContextEnabled = useMemo(
    () => contextFiles.some((f) => f.path === 'plan:context'),
    [contextFiles]
  );

  // Build prompts map for content lookup
  const promptsMap = useMemo(() => {
    const map = new Map<string, { content: string }>();
    for (const p of prompts) {
      map.set(p.id, { content: p.content });
    }
    return map;
  }, [prompts]);

  // Diff comments management
  const pendingCommentsByFile = usePendingCommentsByFile();
  const markCommentsSent = useDiffCommentsStore((state) => state.markCommentsSent);
  const removeComment = useDiffCommentsStore((state) => state.removeComment);

  const handleRemoveCommentFile = useCallback(
    (filePath: string) => {
      if (!resolvedSessionId) return;
      const comments = pendingCommentsByFile[filePath] || [];
      for (const comment of comments) {
        removeComment(resolvedSessionId, filePath, comment.id);
      }
    },
    [resolvedSessionId, pendingCommentsByFile, removeComment]
  );

  const handleRemoveComment = useCallback(
    (sid: string, filePath: string, commentId: string) => {
      removeComment(sid, filePath, commentId);
    },
    [removeComment]
  );

  // Build unified context items
  const contextItems = useMemo((): ContextItem[] => {
    const items: ContextItem[] = [];
    const sid = resolvedSessionId;

    // Plan
    if (planContextEnabled) {
      const planFile = contextFiles.find(f => f.path === 'plan:context');
      items.push({
        kind: 'plan',
        id: 'plan:context',
        label: 'Plan',
        taskId: taskId ?? undefined,
        pinned: planFile?.pinned,
        onRemove: sid ? () => removeContextFile(sid, 'plan:context') : undefined,
        onUnpin: planFile?.pinned && sid ? () => unpinFile(sid, 'plan:context') : undefined,
        onOpen: addPlan,
      });
    }

    // Files & Prompts from context store
    for (const f of contextFiles) {
      if (f.path === 'plan:context') continue;
      if (f.path.startsWith('prompt:')) {
        const promptId = f.path.replace('prompt:', '');
        const prompt = promptsMap.get(promptId);
        items.push({
          kind: 'prompt',
          id: f.path,
          label: f.name,
          pinned: f.pinned,
          onRemove: sid ? () => removeContextFile(sid, f.path) : undefined,
          onUnpin: f.pinned && sid ? () => unpinFile(sid, f.path) : undefined,
          promptContent: prompt?.content,
          onClick: () => {/* navigate to settings/prompts if desired */},
        });
      } else {
        items.push({
          kind: 'file',
          id: f.path,
          label: f.name,
          path: f.path,
          pinned: f.pinned,
          onRemove: sid ? () => removeContextFile(sid, f.path) : undefined,
          onUnpin: f.pinned && sid ? () => unpinFile(sid, f.path) : undefined,
          onOpen: onOpenFile ?? (() => {}),
        });
      }
    }

    // Diff/editor comments (per file)
    if (pendingCommentsByFile) {
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
          onRemoveComment: (cid) => { if (sid) handleRemoveComment(sid, filePath, cid); },
          onOpen: onOpenFileAtLine ? () => onOpenFileAtLine(filePath) : undefined,
        });
      }
    }

    // Plan comments
    if (documentComments.length > 0) {
      items.push({
        kind: 'plan-comment',
        id: 'plan-comments',
        label: `${documentComments.length} plan comment${documentComments.length !== 1 ? 's' : ''}`,
        comments: documentComments,
        onRemove: handleClearDocumentComments,
        onOpen: addPlan,
      });
    }

    return items.sort(contextItemSortFn);
  }, [
    planContextEnabled,
    contextFiles,
    resolvedSessionId,
    removeContextFile,
    unpinFile,
    addPlan,
    promptsMap,
    onOpenFile,
    pendingCommentsByFile,
    handleRemoveCommentFile,
    handleRemoveComment,
    onOpenFileAtLine,
    documentComments,
    handleClearDocumentComments,
    taskId,
  ]);

  // Message sending
  const { handleSendMessage } = useMessageHandler(
    resolvedSessionId,
    taskId,
    sessionModel,
    activeModel,
    planContextEnabled,
    isAgentBusy,
    activeDocument,
    documentComments,
    contextFiles,
    prompts
  );

  // Clear awaiting state when a new agent message arrives
  useEffect(() => {
    lastAgentMessageCountRef.current = agentMessageCount;
  }, [agentMessageCount]);

  const handleCancelTurn = useCallback(async () => {
    if (!resolvedSessionId) return;
    const client = getWebSocketClient();
    if (!client) return;

    try {
      await client.request('agent.cancel', { session_id: resolvedSessionId }, 15000);
    } catch (error) {
      console.error('Failed to cancel agent turn:', error);
    }
  }, [resolvedSessionId]);

  const handleCancelQueue = useCallback(async () => {
    try {
      await cancelQueue();
    } catch (error) {
      console.error('Failed to cancel queued message:', error);
    }
  }, [cancelQueue]);

  const handleQueueEditComplete = useCallback(() => {
    chatInputRef.current?.focusInput();
  }, []);

  const handleSubmit = useCallback(async (message: string, reviewComments?: DiffComment[], attachments?: MessageAttachment[], inlineMentions?: import('@/lib/state/context-files-store').ContextFile[]) => {
    if (isSending) return;
    setIsSending(true);
    try {
      let finalMessage = message;
      if (reviewComments && reviewComments.length > 0) {
        const reviewMarkdown = formatReviewCommentsAsMarkdown(reviewComments);
        finalMessage = reviewMarkdown + (message ? message : '');
      }

      const hasReviewComments = !!(reviewComments && reviewComments.length > 0);

      if (onSend) {
        await onSend(finalMessage);
      } else {
        await handleSendMessage(finalMessage, attachments, hasReviewComments, inlineMentions);
      }

      // Mark comments as sent
      if (reviewComments && reviewComments.length > 0) {
        markCommentsSent(reviewComments.map((c) => c.id));
      }

      // Clear plan/document comments after sending
      if (documentComments.length > 0) {
        handleClearDocumentComments();
      }

      // Clear ephemeral context items after send
      if (resolvedSessionId) {
        clearEphemeral(resolvedSessionId);
      }
    } finally {
      setIsSending(false);
    }
  }, [isSending, onSend, handleSendMessage, markCommentsSent, documentComments.length, handleClearDocumentComments, resolvedSessionId, clearEphemeral]);

  // Focus input with / shortcut
  useKeyboardShortcut(
    SHORTCUTS.FOCUS_INPUT,
    useCallback((event: KeyboardEvent) => {
      const activeElement = document.activeElement;
      const isTyping =
        activeElement instanceof HTMLInputElement ||
        activeElement instanceof HTMLTextAreaElement ||
        (activeElement instanceof HTMLElement && activeElement.isContentEditable);

      if (isTyping) return;

      const inputHandle = chatInputRef.current;
      if (inputHandle) {
        event.preventDefault();
        inputHandle.focusInput();
      }
    }, []),
    { enabled: true, preventDefault: false }
  );

  return (
    <PanelRoot>
      {/* Scrollable messages area */}
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

      {/* Sticky input at bottom */}
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
          placeholder={
            isAgentBusy
              ? 'Queue instructions to the agent...'
              : activeDocument?.type === 'file'
                ? `Continue working on ${activeDocument.name}...`
                : planModeEnabled
                  ? 'Continue working on the plan...'
                  : 'Continue working on the task...'
          }
          pendingClarification={pendingClarification}
          onClarificationResolved={handleClarificationResolved}
          showRequestChangesTooltip={showRequestChangesTooltip}
          onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
          pendingCommentsByFile={pendingCommentsByFile}
          submitKey={chatSubmitKey}
          hasAgentCommands={hasAgentCommands}
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
    </PanelRoot>
  );
});
