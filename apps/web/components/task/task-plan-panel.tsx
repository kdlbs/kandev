'use client';

import { memo, useState, useCallback, useEffect, useRef } from 'react';
import dynamic from 'next/dynamic';
import { IconLoader2 } from '@tabler/icons-react';
import { useTaskPlan } from '@/hooks/domains/session/use-task-plan';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import { TaskPlanToolbar } from './task-plan-toolbar';
import { TaskPlanDialogs } from './task-plan-dialogs';

// Dynamic import to avoid SSR issues with Milkdown
const MarkdownEditor = dynamic(
  () => import('./markdown-editor').then((mod) => mod.MarkdownEditor),
  { ssr: false, loading: () => <div className="flex h-full items-center justify-center text-muted-foreground text-sm">Loading editor...</div> }
);

type TaskPlanPanelProps = {
  taskId: string | null;
  showApproveButton?: boolean;
  onApprove?: () => void;
  visible?: boolean;
};

export const TaskPlanPanel = memo(function TaskPlanPanel({ taskId, showApproveButton, onApprove, visible = true }: TaskPlanPanelProps) {
  const { plan, isLoading, isSaving, savePlan, deletePlan } = useTaskPlan(taskId, { visible });
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId] ?? null : null
  );
  const isAgentBusy = activeSession?.state === 'STARTING' || activeSession?.state === 'RUNNING';
  const [draftContent, setDraftContent] = useState('');
  const [showDiscardDialog, setShowDiscardDialog] = useState(false);
  const [showClearDialog, setShowClearDialog] = useState(false);
  const [isReanalyzing, setIsReanalyzing] = useState(false);
  // Key to force editor remount on external content changes
  const [editorKey, setEditorKey] = useState(0);
  // Track the last known plan content to detect external updates
  const lastPlanContentRef = useRef<string | undefined>(plan?.content);

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

  return (
    <div className="flex flex-col h-full">
      {/* Content - Markdown Editor */}
      <div className="flex-1 min-h-0">
        <MarkdownEditor
          key={`${taskId}-${editorKey}`}
          value={draftContent}
          onChange={setDraftContent}
          placeholder="Start typing your plan..."
        />
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
        showApproveButton={showApproveButton}
        onSave={handleSave}
        onDiscard={() => setShowDiscardDialog(true)}
        onCopy={handleCopy}
        onReanalyze={handleReanalyze}
        onClear={() => setShowClearDialog(true)}
        onApprove={onApprove}
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
    </div>
  );
});

