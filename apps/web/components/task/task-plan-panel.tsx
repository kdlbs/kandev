'use client';

import { memo, useState, useCallback, useEffect, useRef } from 'react';
import dynamic from 'next/dynamic';
import { IconLoader2, IconFileText, IconRobot, IconMessage, IconClick } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import { useTaskPlan } from '@/hooks/domains/session/use-task-plan';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import { TaskPlanToolbar } from './task-plan-toolbar';
import { TaskPlanDialogs } from './task-plan-dialogs';
import { PlanSelectionPopover } from './plan-selection-popover';
import type { PlanComment } from './plan-comments';
import type { TextSelection, CommentHighlight } from './markdown-editor';

// Dynamic import to avoid SSR issues with Milkdown
const MarkdownEditor = dynamic(
  () => import('./markdown-editor').then((mod) => mod.MarkdownEditor),
  { ssr: false, loading: () => <div className="flex h-full items-center justify-center text-muted-foreground text-sm">Loading editor...</div> }
);

type TaskPlanPanelProps = {
  taskId: string | null;
  visible?: boolean;
};

export const TaskPlanPanel = memo(function TaskPlanPanel({ taskId, visible = true }: TaskPlanPanelProps) {
  const { plan, isLoading, isSaving, savePlan, deletePlan } = useTaskPlan(taskId, { visible });
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId] ?? null : null
  );
  const isAgentBusy = activeSession?.state === 'STARTING' || activeSession?.state === 'RUNNING';
  const [draftContent, setDraftContent] = useState(plan?.content ?? '');
  const [showDiscardDialog, setShowDiscardDialog] = useState(false);
  const [showClearDialog, setShowClearDialog] = useState(false);
  const [isReanalyzing, setIsReanalyzing] = useState(false);
  // Key to force editor remount on external content changes
  const [editorKey, setEditorKey] = useState(0);
  // Track the last known plan content to detect external updates
  // Initialize to undefined so the first useEffect run always syncs if plan is already loaded
  const lastPlanContentRef = useRef<string | undefined>(undefined);
  // Text selection state for the selection popover
  const [textSelection, setTextSelection] = useState<TextSelection | null>(null);
  // Plan comments state
  const [comments, setComments] = useState<PlanComment[]>([]);
  const [editingCommentId, setEditingCommentId] = useState<string | null>(null);
  const [isSubmittingComments, setIsSubmittingComments] = useState(false);
  // Ref to the editor wrapper for positioning gutter markers
  const editorWrapperRef = useRef<HTMLDivElement>(null);
  // Track editor focus state to show/hide placeholder
  const [isEditorFocused, setIsEditorFocused] = useState(false);

  // Handler to focus the editor when clicking the empty state
  const handleEmptyStateClick = useCallback(() => {
    const editorElement = editorWrapperRef.current?.querySelector('.ProseMirror');
    if (editorElement) {
      (editorElement as HTMLElement).focus();
    }
  }, []);

  // Track focus state using document-level listener
  useEffect(() => {
    const checkFocus = () => {
      const wrapper = editorWrapperRef.current;
      if (!wrapper) return;
      setIsEditorFocused(wrapper.contains(document.activeElement));
    };

    document.addEventListener('focusin', checkFocus);
    document.addEventListener('focusout', checkFocus);
    checkFocus();

    return () => {
      document.removeEventListener('focusin', checkFocus);
      document.removeEventListener('focusout', checkFocus);
    };
  }, []);

  // Sync draft with plan content and force editor remount when plan changes externally
  useEffect(() => {
    const prevContent = lastPlanContentRef.current;
    const newContent = plan?.content;
    lastPlanContentRef.current = newContent;

    // Only update if the plan content actually changed (external update)
    if (newContent !== prevContent) {
      if (newContent !== undefined) {
        setDraftContent(newContent);
      } else {
        setDraftContent('');
      }
      // Force editor remount to reflect new content (Milkdown doesn't respond to prop changes)
      setEditorKey(k => k + 1);
    }
  }, [plan?.content]);

  const hasUnsavedChanges = plan ? draftContent !== plan.content : draftContent.length > 0;

  const handleSave = useCallback(async () => {
    await savePlan(draftContent, plan?.title);
  }, [savePlan, draftContent, plan?.title]);

  const handleDiscard = useCallback(() => {
    setDraftContent(plan?.content || '');
    setEditorKey(k => k + 1); // Force editor remount to reset content
    setShowDiscardDialog(false);
  }, [plan?.content]);

  const handleCopy = useCallback(async () => {
    if (!draftContent) return;
    try {
      await navigator.clipboard.writeText(draftContent);
    } catch (err) {
      console.error('Failed to copy to clipboard:', err);
    }
  }, [draftContent]);

  const handleReanalyze = useCallback(async () => {
    if (!taskId || !activeSessionId || isAgentBusy || isReanalyzing) return;
    const client = getWebSocketClient();
    if (!client) return;

    setIsReanalyzing(true);
    try {
      await client.request(
        'message.add',
        {
          task_id: taskId,
          session_id: activeSessionId,
          content: 'Please review and update the plan based on the current state of the task. Consider what has been completed and what still needs to be done.',
        },
        10000
      );
    } catch (err) {
      console.error('Failed to send re-analyze request:', err);
    } finally {
      setIsReanalyzing(false);
    }
  }, [taskId, activeSessionId, isAgentBusy, isReanalyzing]);

  const handleClear = useCallback(async () => {
    setShowClearDialog(false);
    const success = await deletePlan();
    if (success) {
      setDraftContent('');
      setEditorKey(k => k + 1);
    }
  }, [deletePlan]);

  // Handle adding a comment (from selection popover)
  const handleAddComment = useCallback((comment: string, selectedText: string) => {
    if (editingCommentId) {
      // Update existing comment
      setComments(prev => prev.map(c =>
        c.id === editingCommentId
          ? { ...c, comment, selectedText }
          : c
      ));
      setEditingCommentId(null);
    } else {
      // Add new comment
      const newComment: PlanComment = {
        id: crypto.randomUUID(),
        selectedText,
        comment,
      };
      setComments(prev => [...prev, newComment]);
    }
  }, [editingCommentId]);

  const handleSelectionClose = useCallback(() => {
    setTextSelection(null);
    setEditingCommentId(null);
    // Clear the browser selection
    window.getSelection()?.removeAllRanges();
  }, []);

  // Handle clicking on a highlighted comment in the editor (opens edit popover)
  const handleCommentHighlightClick = useCallback(
    (id: string, position: { x: number; y: number }) => {
      const comment = comments.find((c) => c.id === id);
      if (comment) {
        setEditingCommentId(id);
        setTextSelection({
          text: comment.selectedText,
          position,
        });
      }
    },
    [comments]
  );

  // Handle clearing all comments
  const handleClearComments = useCallback(() => {
    setComments([]);
  }, []);

  // Handle submitting all comments to the agent
  const handleSubmitComments = useCallback(async () => {
    if (!taskId || !activeSessionId || comments.length === 0) return;
    const client = getWebSocketClient();
    if (!client) return;

    setIsSubmittingComments(true);
    try {
      // Format comments for display (short version)
      const commentsList = comments.map((c, i) =>
        `${i + 1}. "${c.selectedText.slice(0, 50)}${c.selectedText.length > 50 ? '...' : ''}" → ${c.comment}`
      ).join('\n');

      // Format full details inside system tags (not rendered in UI)
      const fullDetails = comments.map((c, i) =>
        `Comment ${i + 1}:\n- User comment: ${c.comment}\n- Selected text: "${c.selectedText}"`
      ).join('\n\n');

      const message = `I have feedback on the plan:\n${commentsList}\n\n<kandev-system>\nFull comment details for plan modifications:\n\n${fullDetails}\n\nCurrent plan content:\n${draftContent}\n</kandev-system>`;

      await client.request(
        'message.add',
        {
          task_id: taskId,
          session_id: activeSessionId,
          content: message,
        },
        10000
      );

      // Clear comments after successful submission
      setComments([]);
    } catch (err) {
      console.error('Failed to submit comments:', err);
    } finally {
      setIsSubmittingComments(false);
    }
  }, [taskId, activeSessionId, comments, draftContent]);

  // Ctrl+S to save
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        if (hasUnsavedChanges && !isSaving) {
          handleSave();
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [hasUnsavedChanges, isSaving, handleSave]);

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <IconLoader2 className="h-5 w-5 animate-spin mr-2" />
        <span className="text-sm">Loading plan...</span>
      </div>
    );
  }

  if (!taskId) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <span className="text-sm">No task selected</span>
      </div>
    );
  }

  // Convert PlanComment to CommentHighlight for the editor
  const commentHighlights: CommentHighlight[] = comments.map((c) => ({
    id: c.id,
    selectedText: c.selectedText,
    comment: c.comment,
  }));

  return (
    <div className="flex flex-col h-full">
      {/* Content - Markdown Editor with inline comment highlights */}
      <div
        className={cn(
          "flex-1 min-h-0 relative rounded-lg border transition-colors cursor-text",
          isEditorFocused
            ? "border-primary/50"
            : "border-transparent"
        )}
        ref={editorWrapperRef}
        onClick={handleEmptyStateClick}
      >
        <MarkdownEditor
          key={`${taskId}-${editorKey}`}
          value={draftContent}
          onChange={setDraftContent}
          placeholder="Start typing your plan..."
          onSelectionChange={activeSessionId ? setTextSelection : undefined}
          comments={commentHighlights}
          onCommentClick={handleCommentHighlightClick}
        />
        {/* Rich empty state - shows when no content and editor not focused */}
        {!isLoading && draftContent.trim() === '' && !isEditorFocused && (
          <div
            className="absolute inset-0 flex items-center justify-center pointer-events-none"
            onClick={handleEmptyStateClick}
          >
            <div className="flex flex-col items-center gap-6 max-w-md px-6">
              {/* Main icon */}
              <div className="flex items-center justify-center w-12 h-12 rounded-xl bg-muted/50">
                <IconFileText className="h-6 w-6 text-muted-foreground" />
              </div>

              {/* Title */}
              <div className="text-center">
                <h3 className="text-sm font-medium text-foreground mb-1">Plan your implementation</h3>
                <p className="text-xs text-muted-foreground">
                  A shared document for you and the agent to collaborate on the approach
                </p>
              </div>

              {/* Feature list */}
              <div className="flex flex-col gap-3 w-full">
                <div className="flex items-start gap-3">
                  <IconRobot className="h-4 w-4 text-muted-foreground mt-0.5 shrink-0" />
                  <p className="text-xs text-muted-foreground">
                    The agent can write and update the plan as it works
                  </p>
                </div>
                <div className="flex items-start gap-3">
                  <IconMessage className="h-4 w-4 text-muted-foreground mt-0.5 shrink-0" />
                  <p className="text-xs text-muted-foreground">
                    Select text and press <kbd className="px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-mono text-[10px]">&#8984;I</kbd> to request changes
                  </p>
                </div>
              </div>

              {/* Call to action */}
              <div className="flex items-center gap-2 text-xs text-muted-foreground/70">
                <IconClick className="h-3.5 w-3.5" />
                <span>Click anywhere to start writing</span>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Bottom toolbar */}
      <TaskPlanToolbar
        plan={plan}
        hasUnsavedChanges={hasUnsavedChanges}
        hasDraftContent={draftContent.length > 0}
        isSaving={isSaving}
        isAgentBusy={isAgentBusy}
        isReanalyzing={isReanalyzing}
        hasActiveSession={!!activeSessionId}
        commentCount={comments.length}
        isSubmittingComments={isSubmittingComments}
        onSubmitComments={handleSubmitComments}
        onClearComments={handleClearComments}
        onSave={handleSave}
        onDiscard={() => setShowDiscardDialog(true)}
        onCopy={handleCopy}
        onReanalyze={handleReanalyze}
        onClear={() => setShowClearDialog(true)}
      />

      {/* Confirmation dialogs */}
      <TaskPlanDialogs
        showDiscardDialog={showDiscardDialog}
        showClearDialog={showClearDialog}
        onDiscardDialogChange={setShowDiscardDialog}
        onClearDialogChange={setShowClearDialog}
        onDiscard={handleDiscard}
        onClear={handleClear}
      />

      {/* Selection popover for adding comments */}
      {textSelection && activeSessionId && (
        <PlanSelectionPopover
          selectedText={textSelection.text}
          position={textSelection.position}
          onAdd={handleAddComment}
          onClose={handleSelectionClose}
          editingComment={editingCommentId ? comments.find(c => c.id === editingCommentId)?.comment : undefined}
        />
      )}
    </div>
  );
});

