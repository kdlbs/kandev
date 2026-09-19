"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { KeyboardEvent, MouseEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { useCommitTaskDescription } from "@/hooks/use-optimistic-task-mutation";
import { closeFieldEditor, openFieldEditor } from "@/lib/state/office-task-content-sync";
import type { TaskSession } from "@/app/office/tasks/[id]/types";

type TaskDescriptionEditableProps = {
  taskId: string;
  description: string | undefined;
  sessions: TaskSession[];
};

type DescriptionReadViewProps = {
  description: string | undefined;
  onActivate: () => void;
};

function DescriptionReadView({ description, onActivate }: DescriptionReadViewProps) {
  const { t } = useTranslation();
  const trimmedDescription = (description ?? "").trim();

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLDivElement>) => {
      if (e.key === "Enter") {
        e.preventDefault();
        onActivate();
      }
    },
    [onActivate],
  );

  const handleClick = useCallback(() => {
    const selection = window.getSelection();
    if (selection && selection.toString().length > 0) return; // AC-34
    onActivate();
  }, [onActivate]);

  const stopSelectionMouseDown = useCallback((e: MouseEvent) => {
    // Allow text selection inside the read view without triggering a click
    // activation once the mouseup fires with a non-empty selection.
    e.stopPropagation();
  }, []);

  if (!trimmedDescription) {
    return (
      <div
        data-testid="office-task-description-empty"
        tabIndex={0}
        role="button"
        className="prose prose-sm mt-4 max-w-none text-sm text-muted-foreground cursor-text rounded-sm outline-none focus-visible:ring-[2px] focus-visible:ring-ring/35"
        onClick={onActivate}
        onKeyDown={handleKeyDown}
      >
        {t("task:addDescription")}
      </div>
    );
  }

  return (
    <div
      data-testid="office-task-description"
      tabIndex={0}
      role="button"
      className="prose prose-sm mt-4 max-w-none text-sm whitespace-pre-wrap cursor-text rounded-sm outline-none focus-visible:ring-[2px] focus-visible:ring-ring/35"
      onMouseDown={stopSelectionMouseDown}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
    >
      {description}
    </div>
  );
}

type DescriptionDiscardDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

function DescriptionDiscardDialog({
  open,
  onOpenChange,
  onConfirm,
}: DescriptionDiscardDialogProps) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent data-testid="office-task-description-discard-confirm">
        <AlertDialogHeader>
          <AlertDialogTitle>{t("task:discardDescriptionTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{t("task:discardDescriptionBody")}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("task:keepEditing")}</AlertDialogCancel>
          <AlertDialogAction className="cursor-pointer" onClick={onConfirm}>
            {t("task:discard")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

type DescriptionEditViewProps = {
  draft: string;
  isDirty: boolean;
  isSaving: boolean;
  hasRunningSession: boolean;
  showDiscardConfirm: boolean;
  onChange: (value: string) => void;
  onKeyDown: (e: KeyboardEvent<HTMLTextAreaElement>) => void;
  onSave: () => void;
  onCancel: () => void;
  onDiscardOpenChange: (open: boolean) => void;
  onDiscardConfirm: () => void;
};

function DescriptionEditView({
  draft,
  isDirty,
  isSaving,
  hasRunningSession,
  showDiscardConfirm,
  onChange,
  onKeyDown,
  onSave,
  onCancel,
  onDiscardOpenChange,
  onDiscardConfirm,
}: DescriptionEditViewProps) {
  const { t } = useTranslation();
  return (
    <div className="mt-4 space-y-2">
      <Textarea
        data-testid="office-task-description-textarea"
        aria-label={t("task:description")}
        autoFocus
        rows={8}
        value={draft}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={onKeyDown}
      />
      {hasRunningSession && (
        <p
          data-testid="office-task-description-running-note"
          className="text-xs text-muted-foreground"
        >
          {t("task:descriptionRunningNote")}
        </p>
      )}
      <div className="flex gap-2">
        <Button
          data-testid="office-task-description-save"
          size="sm"
          className="cursor-pointer"
          disabled={!isDirty || isSaving}
          onClick={onSave}
        >
          {t("common:save")}
        </Button>
        <Button
          data-testid="office-task-description-cancel"
          size="sm"
          variant="outline"
          className="cursor-pointer"
          disabled={isSaving}
          onClick={onCancel}
        >
          {t("common:cancel")}
        </Button>
      </div>
      <DescriptionDiscardDialog
        open={showDiscardConfirm}
        onOpenChange={onDiscardOpenChange}
        onConfirm={onDiscardConfirm}
      />
    </div>
  );
}

/**
 * Click-to-edit description (docs/specs/office-task-content-editing/spec.md
 * §C). No blur-cancel (AC-21): only Escape (when clean), Save, or Cancel
 * leave the editor. Escape/Cancel with a dirty draft confirms discard first.
 */
export function TaskDescriptionEditable({
  taskId,
  description,
  sessions,
}: TaskDescriptionEditableProps) {
  const commitDescription = useCommitTaskDescription();
  const [isEditing, setIsEditing] = useState(false);
  const [draft, setDraft] = useState("");
  // Frozen at editor-open time, trimmed (AC-51): dirty/discard comparisons
  // never re-derive from a live canonical/displayed value while guarded.
  const [baseline, setBaseline] = useState("");
  const [isSaving, setIsSaving] = useState(false);
  const [showDiscardConfirm, setShowDiscardConfirm] = useState(false);
  // Mirrors `draft` so the async Save handler can read the *live* draft
  // after its `await` without closing over a stale value typed during the
  // in-flight request (AC-17 vs AC-42).
  const draftRef = useRef("");
  // Invalidated when taskId changes so an old request cannot update the
  // replacement task's editor state.
  const saveGenerationRef = useRef(0);

  const isDirty = draft.trim() !== baseline;
  const hasRunningSession = sessions.some((s) => s.state === "RUNNING");

  // AC-68/COR-002: TaskBody swaps simple/advanced panes on a URL search
  // param rather than the route id, which unmounts this component without
  // running any id-keyed cleanup elsewhere. Description never closes on blur
  // (AC-21), so an open editor at toggle time has no other safety net.
  // closeFieldEditor is a no-op if the guard was already released via
  // save/cancel/discard. This component instance can also be reused for a
  // different taskId without unmounting (no key={taskId} upstream); resetting
  // the local editor state closes a still-open editor so the previous task's
  // draft never lingers over the new task.
  useEffect(() => {
    return () => {
      saveGenerationRef.current += 1;
      closeFieldEditor(taskId, "description");
      setIsEditing(false);
      setIsSaving(false);
      setShowDiscardConfirm(false);
    };
  }, [taskId]);

  const closeEditor = useCallback(() => {
    closeFieldEditor(taskId, "description");
    setIsEditing(false);
    setShowDiscardConfirm(false);
  }, [taskId]);

  const activate = useCallback(() => {
    const value = description ?? "";
    setBaseline(value.trim());
    setDraft(value);
    draftRef.current = value;
    setIsEditing(true);
    openFieldEditor(taskId, "description");
  }, [description, taskId]);

  const handleChange = useCallback((value: string) => {
    setDraft(value);
    draftRef.current = value;
  }, []);

  const handleCancel = useCallback(() => {
    if (isSaving) return;
    if (isDirty) {
      setShowDiscardConfirm(true);
      return;
    }
    closeEditor();
  }, [closeEditor, isDirty, isSaving]);

  const handleTextareaKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key !== "Escape") return;
      if (isSaving) return;
      if (isDirty) return; // AC-22: Escape does nothing while dirty
      e.preventDefault();
      closeEditor(); // AC-23
    },
    [closeEditor, isDirty, isSaving],
  );

  const handleSave = useCallback(async () => {
    if (isSaving || !isDirty) return;
    setIsSaving(true);
    const trimmed = draft.trim();
    const saveGeneration = saveGenerationRef.current;
    const result = await commitDescription(taskId, trimmed);
    if (saveGeneration !== saveGenerationRef.current) return;
    setIsSaving(false);
    if (!result.ok) return; // stays open, draft intact (AC-18)
    if (draftRef.current.trim() === trimmed) {
      closeEditor(); // AC-16
    } else {
      setBaseline(result.value.trim()); // AC-42: draft changed mid-save
    }
  }, [closeEditor, commitDescription, draft, isDirty, isSaving, taskId]);

  if (!isEditing) {
    return <DescriptionReadView description={description} onActivate={activate} />;
  }

  return (
    <DescriptionEditView
      draft={draft}
      isDirty={isDirty}
      isSaving={isSaving}
      hasRunningSession={hasRunningSession}
      showDiscardConfirm={showDiscardConfirm}
      onChange={handleChange}
      onKeyDown={handleTextareaKeyDown}
      onSave={handleSave}
      onCancel={handleCancel}
      onDiscardOpenChange={(open) => {
        if (!open) setShowDiscardConfirm(false);
      }}
      onDiscardConfirm={closeEditor}
    />
  );
}
