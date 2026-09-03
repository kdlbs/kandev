"use client";

import { memo, useState, useCallback, useEffect, useRef, useMemo } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { PlanPanelHeader } from "./task-plan-panel-header";
import dynamic from "@/lib/routing/client-dynamic";
import { IconLoader2, IconFileText, IconRobot, IconMessage, IconClick } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { useTaskPlan } from "@/hooks/domains/session/use-task-plan";
import { usePlanDraft } from "@/hooks/domains/session/use-plan-draft";
import { TaskPlanSaveErrorBanner } from "./task-plan-save-error-banner";
import { useAppStore } from "@/components/state-provider";
import { PlanSelectionPopover } from "./plan-selection-popover";
import { usePlanComments } from "@/hooks/domains/comments/use-plan-comments";
import {
  resolvePlanCommentRunAvailability,
  useRunComment,
} from "@/hooks/domains/comments/use-run-comment";
import type { PlanComment } from "@/lib/state/slices/comments";
import type {
  TextSelection,
  CommentForEditor,
} from "@/components/editors/tiptap/tiptap-plan-editor";
import type { Editor } from "@tiptap/core";
import { PanelSearchBar } from "@/components/search/panel-search-bar";
import { usePlanFindShortcut } from "./use-plan-find-shortcut";
import { usePlanSelection } from "./use-plan-selection";
import { Trans, useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import { usePlanCommentMigration } from "@/hooks/domains/comments/use-plan-comment-migration";
import { PlanCommentMigrationNotice } from "./plan-comment-migration-notice";

// Dynamic import to avoid SSR issues with TipTap
const PlanEditor = dynamic(
  () =>
    import("@/components/editors/tiptap/tiptap-plan-editor").then((mod) => mod.TipTapPlanEditor),
  {
    ssr: false,
    loading: () => (
      <div className="flex h-full items-center justify-center text-muted-foreground text-sm">
        {t("task:loadingEditor")}
      </div>
    ),
  },
);

type TaskPlanPanelProps = {
  taskId: string | null;
  visible?: boolean;
  /** Internal mobile layout offset for the docked formatting strip. */
  mobileBottomOffset?: string;
};

function useTaskPlanPanelState(taskId: string | null, visible: boolean) {
  const {
    plan,
    isLoading,
    isSaving,
    saveError,
    savePlan,
    revisions,
    isLoadingRevisions,
    loadRevisions,
    loadRevisionContent,
    revertTo,
    previewRevisionId,
    setPreviewRevision,
    comparePair,
    toggleCompareSelection,
    clearComparePair,
  } = useTaskPlan(taskId, { visible });
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? (state.taskSessions.items[activeSessionId] ?? null) : null,
  );
  const sessionState = activeSession?.state;
  const isAgentBusy = sessionState === "STARTING" || sessionState === "RUNNING";

  const editorWrapperRef = useRef<HTMLDivElement>(null);
  const editorInstanceRef = useRef<Editor | null>(null);
  const [editorInstance, setEditorInstance] = useState<Editor | null>(null);
  const {
    draftContent,
    setDraftContent,
    editorKey,
    isEditorFocused,
    handleEmptyStateClick,
    hasUnsavedChanges,
    attemptSave,
  } = usePlanDraft({ plan, isSaving, savePlan, editorWrapperRef, taskId, saveError });
  const commentState = usePlanComments(taskId);
  const planCommentMigration = usePlanCommentMigration(taskId);
  const selectionState = usePlanSelection(taskId, commentState);

  const handleEditorReady = useCallback((editor: Editor) => {
    editorInstanceRef.current = editor;
    setEditorInstance(editor);
  }, []);

  const handleCommentDeleted = useCallback(
    (ids: string[]) => {
      for (const id of ids) {
        void commentState.handleDeleteComment(id);
      }
    },
    [commentState],
  );

  const commentHighlights: CommentForEditor[] = useMemo(
    () =>
      commentState.comments.map((c) => ({
        id: c.id,
        selectedText: c.selectedText,
        from: c.from,
        to: c.to,
      })),
    [commentState.comments],
  );

  const isAgentCreatingPlan = isAgentBusy && !plan && draftContent.trim() === "";

  return {
    plan,
    isLoading,
    isSaving,
    saveError,
    savePlan,
    activeSessionId,
    draftContent,
    setDraftContent,
    editorKey,
    isEditorFocused,
    handleEmptyStateClick,
    hasUnsavedChanges,
    attemptSave,
    commentState,
    planCommentMigration,
    selectionState,
    handleEditorReady,
    handleCommentDeleted,
    commentHighlights,
    isAgentBusy,
    isAgentCreatingPlan,
    editorWrapperRef,
    editorInstanceRef,
    editorInstance,
    revisions,
    isLoadingRevisions,
    loadRevisions,
    loadRevisionContent,
    revertTo,
    previewRevisionId,
    setPreviewRevision,
    comparePair,
    toggleCompareSelection,
    clearComparePair,
  };
}

export const TaskPlanPanel = memo(function TaskPlanPanel({
  taskId,
  visible = true,
  mobileBottomOffset,
}: TaskPlanPanelProps) {
  const { t } = useTranslation();
  const state = useTaskPlanPanelState(taskId, visible);
  // Ctrl+S to save immediately
  useSaveShortcut({
    hasUnsavedChanges: state.hasUnsavedChanges,
    isSaving: state.isSaving,
    attemptSave: state.attemptSave,
    draftContent: state.draftContent,
    title: state.plan?.title,
  });

  if (state.isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <IconLoader2 className="h-5 w-5 animate-spin mr-2" />
        <span className="text-sm">{t("task:loadingPlan")}</span>
      </div>
    );
  }

  if (!taskId) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <span className="text-sm">{t("task:noTaskSelected")}</span>
      </div>
    );
  }

  return <PlanPanelContent taskId={taskId} state={state} mobileBottomOffset={mobileBottomOffset} />;
});

function PlanPanelContent({
  taskId,
  state,
  mobileBottomOffset,
}: {
  taskId: string;
  state: ReturnType<typeof useTaskPlanPanelState>;
  mobileBottomOffset?: string;
}) {
  const { t } = useTranslation();
  const { editorWrapperRef, editorInstanceRef, editorInstance, selectionState } = state;
  const { textSelection, setTextSelection } = selectionState;
  // Ctrl+F in-document find (registers a keydown listener on the editor wrapper)
  const planSearch = usePlanFindShortcut(editorWrapperRef, editorInstance);
  return (
    <PanelRoot data-testid="plan-panel">
      <PlanPanelHeader
        taskId={taskId}
        plan={state.plan}
        draftContent={state.draftContent}
        hasUnsavedChanges={state.hasUnsavedChanges}
        activeSessionId={state.activeSessionId ?? null}
        revisions={state.revisions}
        isLoadingRevisions={state.isLoadingRevisions}
        isSaving={state.isSaving}
        isAgentBusy={state.isAgentBusy}
        attemptSave={state.attemptSave}
        onOpenRevisions={state.loadRevisions}
        onRevert={state.revertTo}
        loadRevisionContent={state.loadRevisionContent}
        previewRevisionId={state.previewRevisionId}
        setPreviewRevision={state.setPreviewRevision}
        comparePair={state.comparePair}
        toggleCompareSelection={state.toggleCompareSelection}
        clearComparePair={state.clearComparePair}
      />
      {state.saveError && <TaskPlanSaveErrorBanner saveError={state.saveError} />}
      <PlanCommentMigrationNotice {...state.planCommentMigration} />
      <PanelBody
        padding={false}
        scroll={false}
        className={cn(
          "relative transition-colors cursor-text",
          state.isAgentBusy && "bg-background",
        )}
        ref={editorWrapperRef}
        onClick={state.handleEmptyStateClick}
        data-panel-kind="plan"
      >
        <PlanEditor
          key={`${taskId}-${state.editorKey}`}
          taskId={taskId}
          value={state.draftContent}
          onChange={state.setDraftContent}
          placeholder={t("task:startTypingYourPlan")}
          mobileBottomOffset={mobileBottomOffset}
          onSelectionChange={setTextSelection}
          comments={state.commentHighlights}
          onCommentClick={selectionState.handleCommentHighlightClick}
          onCommentDeleted={state.handleCommentDeleted}
          onEditorReady={state.handleEditorReady}
        />
        <PlanEmptyState
          isLoading={state.isLoading}
          draftContent={state.draftContent}
          isEditorFocused={state.isEditorFocused}
          isAgentCreatingPlan={state.isAgentCreatingPlan}
          onClick={state.handleEmptyStateClick}
        />
        {planSearch.isOpen && (
          <PanelSearchBar
            value={planSearch.query}
            onChange={planSearch.setQuery}
            onNext={planSearch.findNext}
            onPrev={planSearch.findPrev}
            onClose={planSearch.close}
            matchInfo={planSearch.matchInfo}
          />
        )}
      </PanelBody>

      <PlanSelectionPopoverWrapper
        textSelection={textSelection}
        taskId={taskId}
        commentState={state.commentState}
        editorRef={editorInstanceRef}
        onClose={selectionState.handleSelectionClose}
      />
    </PanelRoot>
  );
}

function removeCommentMark(editor: Editor | null, commentId: string) {
  if (!editor) return;
  const markType = editor.state.schema.marks.commentMark;
  if (!markType) return;
  const { tr } = editor.state;
  tr.removeMark(0, editor.state.doc.content.size, markType.create({ commentId }));
  editor.view.dispatch(tr);
}

function planCommentRunDisabledReason(
  migrationReady: boolean,
  reason: ReturnType<typeof resolvePlanCommentRunAvailability>["reason"],
) {
  if (!migrationReady) return t("task:planCommentMigrationPending");
  if (reason === "no-primary-session") return t("task:noPrimarySessionForPlanComment");
  if (reason === "primary-session-unavailable") {
    return t("task:primarySessionUnavailableForPlanComment");
  }
  return null;
}

function usePlanSelectionCommentActions({
  textSelection,
  commentState,
  editorRef,
  runComment,
}: {
  textSelection: TextSelection | null;
  commentState: ReturnType<typeof usePlanComments>;
  editorRef: React.RefObject<Editor | null>;
  runComment: ReturnType<typeof useRunComment>["runComment"];
}) {
  const [runError, setRunError] = useState<string | null>(null);
  const savedRunCommentRef = useRef<{ selection: string; comment: PlanComment } | null>(null);
  const selectionIdentity = textSelection
    ? `${textSelection.from ?? ""}:${textSelection.to ?? ""}:${textSelection.text}`
    : "";

  const addCommentAndApplyMark = useCallback(
    async (comment: string, selectedText: string) => {
      const from = textSelection?.from;
      const to = textSelection?.to;
      const saved = await commentState.handleAddComment(comment, selectedText, from, to);
      const editor = editorRef.current;
      if (saved && editor && from != null && to != null) {
        editor
          .chain()
          .setTextSelection({ from, to })
          .setMark("commentMark", { commentId: saved.id })
          .run();
      }
      return saved;
    },
    [commentState, textSelection, editorRef],
  );

  const handleAdd = useCallback(
    async (comment: string, selectedText: string) => {
      setRunError(null);
      return Boolean(await addCommentAndApplyMark(comment, selectedText));
    },
    [addCommentAndApplyMark],
  );

  const handleAddAndRun = useCallback(
    async (comment: string, selectedText: string) => {
      setRunError(null);
      const previous = savedRunCommentRef.current;
      let saved = previous?.selection === selectionIdentity ? previous.comment : null;
      if (!saved) {
        saved = await addCommentAndApplyMark(comment, selectedText);
        if (!saved) return false;
        savedRunCommentRef.current = { selection: selectionIdentity, comment: saved };
      }
      try {
        await runComment(saved);
        savedRunCommentRef.current = null;
        return true;
      } catch (error) {
        setRunError(error instanceof Error ? error.message : t("task:failedToRunPlanComment"));
        return false;
      }
    },
    [addCommentAndApplyMark, runComment, selectionIdentity],
  );

  return { handleAdd, handleAddAndRun, runError };
}

/** Conditional selection popover for adding/editing comments */
function PlanSelectionPopoverWrapper({
  textSelection,
  taskId,
  commentState,
  editorRef,
  onClose,
}: {
  textSelection: TextSelection | null;
  taskId: string | null;
  commentState: ReturnType<typeof usePlanComments>;
  editorRef: React.RefObject<Editor | null>;
  onClose: () => void;
}) {
  const { runComment } = useRunComment({
    sessionId: null,
    taskId,
  });
  const runUnavailableReason = useAppStore(
    (state) => resolvePlanCommentRunAvailability(state, taskId).reason,
  );
  const migrationReady = useAppStore((state) =>
    taskId ? state.taskPlans.commentsMigrationStatusByTaskId[taskId] === "complete" : false,
  );
  const runDisabledReason = planCommentRunDisabledReason(migrationReady, runUnavailableReason);
  const { handleAdd, handleAddAndRun, runError } = usePlanSelectionCommentActions({
    textSelection,
    commentState,
    editorRef,
    runComment,
  });

  if (!textSelection) return null;
  const editingComment = commentState.editingCommentId
    ? commentState.comments.find((c) => c.id === commentState.editingCommentId)?.text
    : undefined;
  const onDelete = commentState.editingCommentId
    ? async () => {
        const id = commentState.editingCommentId!;
        const deleted = await commentState.handleDeleteComment(id);
        if (deleted) removeCommentMark(editorRef.current, id);
        return deleted;
      }
    : undefined;
  return (
    <PlanSelectionPopover
      selectedText={textSelection.text}
      position={textSelection.position}
      onAdd={handleAdd}
      onAddAndRun={editingComment ? undefined : handleAddAndRun}
      onClose={onClose}
      editingComment={editingComment}
      onDelete={onDelete}
      errorMessage={runError || commentState.mutationError}
      runDisabledReason={runDisabledReason}
    />
  );
}

type SaveShortcutOptions = {
  hasUnsavedChanges: boolean;
  isSaving: boolean;
  attemptSave: (content: string, title?: string) => Promise<unknown>;
  draftContent: string;
  title: string | undefined;
};

/** Ctrl+S save shortcut */
function useSaveShortcut({
  hasUnsavedChanges,
  isSaving,
  attemptSave,
  draftContent,
  title,
}: SaveShortcutOptions) {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "s") {
        e.preventDefault();
        if (hasUnsavedChanges && !isSaving) {
          // An explicit save is the user's escape hatch from a suppressed
          // autosave: it proceeds unconditionally, even resubmitting
          // unchanged content.
          attemptSave(draftContent, title);
        }
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [hasUnsavedChanges, isSaving, attemptSave, draftContent, title]);
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
  const { t } = useTranslation();
  if (isLoading || draftContent.trim() !== "" || isEditorFocused || isAgentCreatingPlan)
    return null;
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
          <h3 className="text-sm font-medium text-foreground mb-1">
            {t("task:planYourImplementation")}
          </h3>
          <p className="text-xs text-muted-foreground">{t("task:aSharedDocumentForYouAnd")}</p>
        </div>
        <div className="flex flex-col gap-3 w-full">
          <div className="flex items-start gap-3">
            <IconRobot className="h-4 w-4 text-muted-foreground mt-0.5 shrink-0" />
            <p className="text-xs text-muted-foreground">{t("task:theAgentCanWriteAndUpdate")}</p>
          </div>
          <div className="flex items-start gap-3">
            <IconMessage className="h-4 w-4 text-muted-foreground mt-0.5 shrink-0" />
            <p className="text-xs text-muted-foreground">
              <Trans i18nKey="task:selectTextAndPressToComment">
                Select text and press{" "}
                <kbd className="px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-mono text-[10px]">
                  &#8984;&#8679;C
                </kbd>{" "}
                to comment
              </Trans>
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2 text-xs text-muted-foreground/70">
          <IconClick className="h-3.5 w-3.5" />
          <span>{t("task:clickAnywhereToStartWriting")}</span>
        </div>
      </div>
    </div>
  );
}
