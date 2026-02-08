'use client';

import { memo, useState, useCallback, useEffect, useRef } from 'react';
import dynamic from 'next/dynamic';
import { IconLoader2, IconFileText, IconRobot, IconMessage, IconClick, IconMessagePlus } from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import { cn } from '@/lib/utils';
import { Button } from '@kandev/ui/button';
import { useTaskPlan } from '@/hooks/domains/session/use-task-plan';
import { useAppStore } from '@/components/state-provider';
import { PlanSelectionPopover } from './plan-selection-popover';
import type { PlanComment } from './plan-comments';
import type { TextSelection, CommentHighlight } from './markdown-editor';
import type { DocumentComment } from '@/lib/state/slices/ui/types';

const EMPTY_COMMENTS: DocumentComment[] = [];

// Dynamic import to avoid SSR issues with Milkdown
const MarkdownEditor = dynamic(
  () => import('./markdown-editor').then((mod) => mod.MarkdownEditor),
  { ssr: false, loading: () => <div className="flex h-full items-center justify-center text-muted-foreground text-sm">Loading editor...</div> }
);

/** Debounce delay for auto-saving plan content (ms) */
const AUTO_SAVE_DELAY = 1500;

type TaskPlanPanelProps = {
  taskId: string | null;
  visible?: boolean;
};

export const TaskPlanPanel = memo(function TaskPlanPanel({ taskId, visible = true }: TaskPlanPanelProps) {
  const { plan, isLoading, isSaving, savePlan } = useTaskPlan(taskId, { visible });
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId] ?? null : null
  );
  const isAgentBusy = activeSession?.state === 'STARTING' || activeSession?.state === 'RUNNING';
  const [draftContent, setDraftContent] = useState(plan?.content ?? '');
  // Ref mirror of draftContent so the sync effect can read it without re-running on every keystroke
  const draftContentRef = useRef(draftContent);
  // Key to force editor remount on external content changes
  const [editorKey, setEditorKey] = useState(0);
  // Track the last known plan content to detect external updates
  // Initialize to undefined so the first useEffect run always syncs if plan is already loaded
  const lastPlanContentRef = useRef<string | undefined>(undefined);
  // Track whether content change is from external update (skip auto-save)
  const isExternalUpdateRef = useRef(false);
  // Text selection state for the selection popover
  const [textSelection, setTextSelection] = useState<TextSelection | null>(null);
  // Floating button position (shown after selection ends)
  const [floatingButtonPos, setFloatingButtonPos] = useState<{ x: number; y: number } | null>(null);
  const [currentSelectionText, setCurrentSelectionText] = useState<string | null>(null);
  // Plan comments state
  const setDocumentComments = useAppStore((state) => state.setDocumentComments);
  const storeComments = useAppStore((state) =>
    activeSessionId ? state.documentPanel.commentsBySessionId[activeSessionId] ?? EMPTY_COMMENTS : EMPTY_COMMENTS
  );
  const [comments, setComments] = useState<PlanComment[]>([]);
  const [editingCommentId, setEditingCommentId] = useState<string | null>(null);
  // Ref to the editor wrapper for positioning gutter markers
  const editorWrapperRef = useRef<HTMLDivElement>(null);
  // Track editor focus state to show/hide placeholder
  const [isEditorFocused, setIsEditorFocused] = useState(false);
  // Auto-save debounce timer
  const autoSaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

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

  // Keep draftContentRef in sync (must be in effect, not render, per React Compiler)
  useEffect(() => {
    draftContentRef.current = draftContent;
  }, [draftContent]);

  // Sync draft with plan content and force editor remount when plan changes externally
  useEffect(() => {
    const prevContent = lastPlanContentRef.current;
    const newContent = plan?.content;
    lastPlanContentRef.current = newContent;

    // Only update if the plan content actually changed (external update)
    if (newContent !== prevContent) {
      // Skip remount if the editor already has this content (our own save response)
      const resolvedContent = newContent ?? '';
      if (resolvedContent === draftContentRef.current) return;

      isExternalUpdateRef.current = true;
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing external plan data to local editor state
      setDraftContent(resolvedContent);
      // Force editor remount to reflect new content (Milkdown doesn't respond to prop changes)
      setEditorKey(k => k + 1);
    }
  }, [plan?.content]);

  const hasUnsavedChanges = plan ? draftContent !== plan.content : draftContent.length > 0;

  const handleSave = useCallback(async () => {
    // Cancel any pending auto-save
    if (autoSaveTimerRef.current) {
      clearTimeout(autoSaveTimerRef.current);
      autoSaveTimerRef.current = null;
    }
    await savePlan(draftContent, plan?.title);
  }, [savePlan, draftContent, plan?.title]);

  // Auto-save with debounce when draft content changes
  useEffect(() => {
    // Skip auto-save when content change came from external update (agent/poll)
    if (isExternalUpdateRef.current) {
      isExternalUpdateRef.current = false;
      return;
    }

    // Only auto-save when there are actual changes to save
    const hasChanges = plan ? draftContent !== plan.content : draftContent.length > 0;
    if (!hasChanges || isSaving) return;

    // Clear previous timer
    if (autoSaveTimerRef.current) {
      clearTimeout(autoSaveTimerRef.current);
    }

    autoSaveTimerRef.current = setTimeout(() => {
      autoSaveTimerRef.current = null;
      savePlan(draftContent, plan?.title);
    }, AUTO_SAVE_DELAY);

    return () => {
      if (autoSaveTimerRef.current) {
        clearTimeout(autoSaveTimerRef.current);
        autoSaveTimerRef.current = null;
      }
    };
  }, [draftContent, plan, isSaving, savePlan]);

  // Ctrl+S to save immediately
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
    setFloatingButtonPos(null);
    setCurrentSelectionText(null);
    // Clear the browser selection
    window.getSelection()?.removeAllRanges();
  }, []);

  // Handle selection end (mouseup) - show floating button
  const handleSelectionEnd = useCallback((selection: TextSelection | null) => {
    if (!activeSessionId) return;
    if (selection) {
      setFloatingButtonPos(selection.position);
      setCurrentSelectionText(selection.text);
    } else {
      setFloatingButtonPos(null);
      setCurrentSelectionText(null);
    }
  }, [activeSessionId]);

  // Handle floating button click - open the popover
  const handleFloatingButtonClick = useCallback(() => {
    if (!floatingButtonPos || !currentSelectionText) return;
    setTextSelection({
      text: currentSelectionText,
      position: floatingButtonPos,
    });
    setFloatingButtonPos(null);
    setCurrentSelectionText(null);
  }, [floatingButtonPos, currentSelectionText]);

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

  // Handle deleting a single comment
  const handleDeleteComment = useCallback((commentId: string) => {
    setComments(prev => prev.filter(c => c.id !== commentId));
  }, []);

  // Sync plan comments to document panel store for chat integration
  useEffect(() => {
    if (activeSessionId) {
      setDocumentComments(activeSessionId, comments.map(c => ({
        id: c.id,
        selectedText: c.selectedText,
        comment: c.comment,
      })));
    }
  }, [activeSessionId, comments, setDocumentComments]);

  // Detect when comments are cleared externally (e.g., after message send from chat).
  // Only react when the store transitions from non-empty → empty (a true external clear),
  // not when the store simply hasn't synced yet from a local addition.
  const prevStoreCommentsLenRef = useRef(storeComments.length);
  useEffect(() => {
    const prevLen = prevStoreCommentsLenRef.current;
    prevStoreCommentsLenRef.current = storeComments.length;

    if (prevLen > 0 && storeComments.length === 0 && comments.length > 0) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing store clear to local state
      setComments([]);
    }
  }, [storeComments.length, comments.length]);

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

  // Show loading state while agent is creating the initial plan
  const isAgentCreatingPlan = isAgentBusy && !plan && draftContent.trim() === '';

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
          onSelectionEnd={activeSessionId ? handleSelectionEnd : undefined}
          comments={commentHighlights}
          onCommentClick={handleCommentHighlightClick}
        />

        {/* Floating "Comment" button when text is selected */}
        {floatingButtonPos && !textSelection && activeSessionId && (
          <Button
            size="sm"
            variant="secondary"
            className="floating-comment-btn fixed z-50 gap-1.5 shadow-lg animate-in fade-in-0 zoom-in-95 duration-100 cursor-pointer"
            style={{
              left: floatingButtonPos.x + 8,
              top: floatingButtonPos.y + 8,
            }}
            onMouseDown={(e) => e.stopPropagation()}
            onClick={handleFloatingButtonClick}
          >
            <IconMessagePlus className="h-3.5 w-3.5" />
            Comment
          </Button>
        )}
        {/* Agent creating plan overlay */}
        {isAgentCreatingPlan && (
          <div className="absolute inset-0 flex items-center justify-center bg-background">
            <div className="flex flex-col items-center gap-4">
              <GridSpinner className="h-5 w-5" />
              <span className="text-sm text-muted-foreground">Agent is creating a plan...</span>
            </div>
          </div>
        )}
        {/* Rich empty state - shows when no content and editor not focused */}
        {!isLoading && draftContent.trim() === '' && !isEditorFocused && !isAgentCreatingPlan && (
          <div
            className="absolute inset-0 flex items-center justify-center pointer-events-none bg-background"
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

      {/* Selection popover for adding comments */}
      {textSelection && activeSessionId && (
        <PlanSelectionPopover
          selectedText={textSelection.text}
          position={textSelection.position}
          onAdd={handleAddComment}
          onClose={handleSelectionClose}
          editingComment={editingCommentId ? comments.find(c => c.id === editingCommentId)?.comment : undefined}
          onDelete={editingCommentId ? () => handleDeleteComment(editingCommentId) : undefined}
        />
      )}
    </div>
  );
});
