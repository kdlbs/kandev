'use client';

import { memo, useState, useCallback, useEffect, useRef, useMemo } from 'react';
import { PanelRoot, PanelBody } from './panel-primitives';
import dynamic from 'next/dynamic';
import { IconLoader2, IconFileText, IconRobot, IconMessage, IconClick } from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import { cn } from '@/lib/utils';
import { useTaskPlan } from '@/hooks/domains/session/use-task-plan';
import { useAppStore } from '@/components/state-provider';
import { PlanSelectionPopover } from './plan-selection-popover';
import { usePlanComments } from '@/hooks/domains/comments/use-plan-comments';
import type { TextSelection, CommentForEditor } from '@/components/editors/tiptap/tiptap-plan-editor';
import type { Editor } from '@tiptap/core';

// Dynamic import to avoid SSR issues with TipTap
const PlanEditor = dynamic(
  () => import('@/components/editors/tiptap/tiptap-plan-editor').then((mod) => mod.TipTapPlanEditor),
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
  const sessionState = activeSession?.state;
  const isAgentBusy = sessionState === 'STARTING' || sessionState === 'RUNNING';

  const editorWrapperRef = useRef<HTMLDivElement>(null);
  const editorInstanceRef = useRef<Editor | null>(null);
  const { draftContent, setDraftContent, editorKey, isEditorFocused, handleEmptyStateClick, hasUnsavedChanges } =
    usePlanDraft(plan, isSaving, savePlan, editorWrapperRef);
  const commentState = usePlanComments(activeSessionId);
  const selectionState = usePlanSelection(activeSessionId, commentState);

  const handleEditorReady = useCallback((editor: Editor) => {
    editorInstanceRef.current = editor;
  }, []);

  const handleCommentDeleted = useCallback((ids: string[]) => {
    for (const id of ids) {
      commentState.handleDeleteComment(id);
    }
  }, [commentState]);

  const commentHighlights: CommentForEditor[] = useMemo(
    () => commentState.comments.map((c) => ({
      id: c.id, selectedText: c.selectedText, from: c.from, to: c.to,
    })),
    [commentState.comments],
  );

  // Ctrl+S to save immediately
  useSaveShortcut(hasUnsavedChanges, isSaving, savePlan, draftContent, plan?.title);

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

  const { textSelection, setTextSelection } = selectionState;

  return (
    <PanelRoot>
      <PanelBody
        padding={false}
        scroll={false}
        className={cn("relative transition-colors cursor-text")}
        ref={editorWrapperRef}
        onClick={handleEmptyStateClick}
      >
        <PlanEditor
          key={`${taskId}-${editorKey}`}
          value={draftContent}
          onChange={setDraftContent}
          placeholder="Start typing your plan..."
          onSelectionChange={activeSessionId ? setTextSelection : undefined}
          comments={commentHighlights}
          onCommentClick={selectionState.handleCommentHighlightClick}
          onCommentDeleted={handleCommentDeleted}
          onEditorReady={handleEditorReady}
        />
        <PlanCreatingOverlay isAgentCreatingPlan={isAgentCreatingPlan} />
        <PlanEmptyState
          isLoading={isLoading}
          draftContent={draftContent}
          isEditorFocused={isEditorFocused}
          isAgentCreatingPlan={isAgentCreatingPlan}
          onClick={handleEmptyStateClick}
        />
      </PanelBody>

      <PlanSelectionPopoverWrapper
        textSelection={textSelection}
        activeSessionId={activeSessionId}
        commentState={commentState}
        editorRef={editorInstanceRef}
        onClose={selectionState.handleSelectionClose}
      />
    </PanelRoot>
  );
});

/** Conditional selection popover for adding/editing comments */
function PlanSelectionPopoverWrapper({
  textSelection,
  activeSessionId,
  commentState,
  editorRef,
  onClose,
}: {
  textSelection: TextSelection | null;
  activeSessionId: string | null | undefined;
  commentState: ReturnType<typeof usePlanComments>;
  editorRef: React.RefObject<Editor | null>;
  onClose: () => void;
}) {
  const handleAdd = useCallback(
    (comment: string, selectedText: string) => {
      const from = textSelection?.from;
      const to = textSelection?.to;
      const id = commentState.handleAddComment(comment, selectedText, from, to);
      // Apply the mark to the editor at the selection position
      const editor = editorRef.current;
      if (id && editor && from != null && to != null) {
        editor.chain()
          .setTextSelection({ from, to })
          .setMark('commentMark', { commentId: id })
          .run();
      }
    },
    [commentState, textSelection, editorRef],
  );

  if (!textSelection || !activeSessionId) return null;
  const editingComment = commentState.editingCommentId
    ? commentState.comments.find(c => c.id === commentState.editingCommentId)?.text
    : undefined;
  const onDelete = commentState.editingCommentId
    ? () => {
        const id = commentState.editingCommentId!;
        // Remove the mark from the editor
        const editor = editorRef.current;
        if (editor) {
          const markType = editor.state.schema.marks.commentMark;
          if (markType) {
            const { tr } = editor.state;
            tr.removeMark(0, editor.state.doc.content.size, markType.create({ commentId: id }));
            editor.view.dispatch(tr);
          }
        }
        commentState.handleDeleteComment(id);
      }
    : undefined;
  return (
    <PlanSelectionPopover
      selectedText={textSelection.text}
      position={textSelection.position}
      onAdd={handleAdd}
      onClose={onClose}
      editingComment={editingComment}
      onDelete={onDelete}
    />
  );
}

/** Agent creating plan overlay */
function PlanCreatingOverlay({ isAgentCreatingPlan }: { isAgentCreatingPlan: boolean }) {
  if (!isAgentCreatingPlan) return null;
  return (
    <div className="absolute inset-0 flex items-center justify-center bg-background">
      <div className="flex flex-col items-center gap-4">
        <GridSpinner className="h-5 w-5" />
        <span className="text-sm text-muted-foreground">Agent is creating a plan...</span>
      </div>
    </div>
  );
}

/** Draft content, editor key, focus tracking, and auto-save */
function usePlanDraft(
  plan: { content?: string; title?: string } | null | undefined,
  isSaving: boolean,
  savePlan: (content: string, title?: string) => Promise<unknown>,
  editorWrapperRef: React.RefObject<HTMLDivElement | null>,
) {
  const [draftContent, setDraftContent] = useState(plan?.content ?? '');
  const draftContentRef = useRef(draftContent);
  const [editorKey, setEditorKey] = useState(0);
  const lastPlanContentRef = useRef<string | undefined>(undefined);
  const isExternalUpdateRef = useRef(false);
  const [isEditorFocused, setIsEditorFocused] = useState(false);
  const autoSaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleEmptyStateClick = useCallback(() => {
    const el = editorWrapperRef.current?.querySelector('.ProseMirror');
    if (el) (el as HTMLElement).focus();
  }, [editorWrapperRef]);

  // Track focus state
  useEffect(() => {
    const checkFocus = () => {
      const wrapper = editorWrapperRef.current;
      if (!wrapper) return;
      setIsEditorFocused(wrapper.contains(document.activeElement));
    };
    document.addEventListener('focusin', checkFocus);
    document.addEventListener('focusout', checkFocus);
    checkFocus();
    return () => { document.removeEventListener('focusin', checkFocus); document.removeEventListener('focusout', checkFocus); };
  }, [editorWrapperRef]);

  useEffect(() => { draftContentRef.current = draftContent; }, [draftContent]);

  // Sync from external plan updates
  useEffect(() => {
    const prevContent = lastPlanContentRef.current;
    const newContent = plan?.content;
    lastPlanContentRef.current = newContent;
    if (newContent !== prevContent) {
      const resolved = newContent ?? '';
      if (resolved === draftContentRef.current) return;
      isExternalUpdateRef.current = true;
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing external plan data to local editor state
      setDraftContent(resolved);
      setEditorKey(k => k + 1);
    }
  }, [plan?.content]);

  // Auto-save with debounce
  useEffect(() => {
    if (isExternalUpdateRef.current) { isExternalUpdateRef.current = false; return; }
    const hasChanges = plan ? draftContent !== plan.content : draftContent.length > 0;
    if (!hasChanges || isSaving) return;
    if (autoSaveTimerRef.current) clearTimeout(autoSaveTimerRef.current);
    autoSaveTimerRef.current = setTimeout(() => {
      autoSaveTimerRef.current = null;
      savePlan(draftContent, plan?.title);
    }, AUTO_SAVE_DELAY);
    return () => { if (autoSaveTimerRef.current) { clearTimeout(autoSaveTimerRef.current); autoSaveTimerRef.current = null; } };
  }, [draftContent, plan, isSaving, savePlan]);

  const hasUnsavedChanges = plan ? draftContent !== plan.content : draftContent.length > 0;
  return { draftContent, setDraftContent, editorKey, isEditorFocused, handleEmptyStateClick, hasUnsavedChanges };
}

/** Text selection state for comment popover */
function usePlanSelection(
  activeSessionId: string | null | undefined,
  commentState: ReturnType<typeof usePlanComments>,
) {
  const [textSelection, setTextSelection] = useState<TextSelection | null>(null);

  const handleCommentHighlightClick = useCallback(
    (id: string, position: { x: number; y: number }) => {
      const comment = commentState.comments.find((c) => c.id === id);
      if (comment) {
        commentState.setEditingCommentId(id);
        setTextSelection({ text: comment.selectedText, from: comment.from, to: comment.to, position });
      }
    },
    [commentState]
  );

  const handleSelectionClose = useCallback(() => {
    setTextSelection(null);
    commentState.setEditingCommentId(null);
    window.getSelection()?.removeAllRanges();
  }, [commentState]);

  return { textSelection, setTextSelection, handleCommentHighlightClick, handleSelectionClose };
}

/** Ctrl+S save shortcut */
function useSaveShortcut(
  hasUnsavedChanges: boolean,
  isSaving: boolean,
  savePlan: (content: string, title?: string) => Promise<unknown>,
  draftContent: string,
  title?: string,
) {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        if (hasUnsavedChanges && !isSaving) savePlan(draftContent, title);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [hasUnsavedChanges, isSaving, savePlan, draftContent, title]);
}

/** Rich empty state - shows when no content and editor not focused */
function PlanEmptyState({
  isLoading,
  draftContent,
  isEditorFocused,
  isAgentCreatingPlan,
  onClick,
}: {
  isLoading: boolean;
  draftContent: string;
  isEditorFocused: boolean;
  isAgentCreatingPlan: boolean;
  onClick: () => void;
}) {
  if (isLoading || draftContent.trim() !== '' || isEditorFocused || isAgentCreatingPlan) return null;
  return (
    <div
      className="absolute inset-0 flex items-center justify-center pointer-events-none"
      onClick={onClick}
    >
      <div className="flex flex-col items-center gap-6 max-w-md px-6">
        <div className="flex items-center justify-center w-12 h-12 rounded-xl bg-muted/50">
          <IconFileText className="h-6 w-6 text-muted-foreground" />
        </div>
        <div className="text-center">
          <h3 className="text-sm font-medium text-foreground mb-1">Plan your implementation</h3>
          <p className="text-xs text-muted-foreground">
            A shared document for you and the agent to collaborate on the approach
          </p>
        </div>
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
              Select text and press <kbd className="px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-mono text-[10px]">&#8984;&#8679;C</kbd> to comment
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2 text-xs text-muted-foreground/70">
          <IconClick className="h-3.5 w-3.5" />
          <span>Click anywhere to start writing</span>
        </div>
      </div>
    </div>
  );
}
