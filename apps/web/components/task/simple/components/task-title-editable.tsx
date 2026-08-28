"use client";

import { useCallback, useEffect, useState } from "react";
import type { KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { useCommitTaskTitle } from "@/hooks/use-optimistic-task-mutation";
import { useTaskTitleSelectionRestore } from "@/hooks/use-task-title-selection-restore";
import { closeFieldEditor, openFieldEditor } from "@/lib/state/office-task-content-sync";

type TaskTitleEditableProps = {
  taskId: string;
  title: string;
};

/**
 * Inline title editor (docs/specs/office-task-content-editing/spec.md §B).
 * Double-click or Enter-on-focus opens a single-line input; Enter commits,
 * Escape/blur cancel. No archived-task or status gate (AC-30, AC-47).
 */
export function TaskTitleEditable({ taskId, title }: TaskTitleEditableProps) {
  const { t } = useTranslation();
  const commitTitle = useCommitTaskTitle();
  const [isEditing, setIsEditing] = useState(false);
  const [draft, setDraft] = useState("");
  // Frozen at editor-open time (AC-51): the equality test in AC-8 must never
  // compare against a canonical value that arrives while the field is
  // guarded, even though nothing else can normally change `title` while
  // guarded — an unrelated in-flight write's failure restore can (see the
  // task plan's R5 addendum), so a live read of `title` is not safe here.
  const [baseline, setBaseline] = useState("");
  const { inputRef, clampChange } = useTaskTitleSelectionRestore(draft);

  // AC-68/COR-002: TaskBody swaps simple/advanced panes on a URL search
  // param rather than the route id, which unmounts this component without
  // running any id-keyed cleanup elsewhere. closeFieldEditor is a no-op if
  // the guard was already released via commit/cancel/blur. This component
  // instance can also be reused for a different taskId without unmounting
  // (no key={taskId} upstream); setIsEditing(false) closes a still-open
  // editor so the previous task's draft never lingers over the new task.
  useEffect(() => {
    return () => {
      closeFieldEditor(taskId, "title");
      setIsEditing(false);
    };
  }, [taskId]);

  const closeEditor = useCallback(() => {
    closeFieldEditor(taskId, "title");
    setIsEditing(false);
  }, [taskId]);

  const activate = useCallback(() => {
    setBaseline(title);
    setDraft(title);
    setIsEditing(true);
    openFieldEditor(taskId, "title");
  }, [taskId, title]);

  const handleHeadingKeyDown = useCallback(
    (e: KeyboardEvent<HTMLHeadingElement>) => {
      if (e.key === "Enter") {
        e.preventDefault();
        activate();
      }
    },
    [activate],
  );

  const commit = useCallback(
    (trimmed: string) => {
      // Begin the write's guard contribution before releasing the editor's,
      // so the field is never observably unguarded between the two (R5-F3
      // ordering pattern, applied here to avoid a needless intermediate
      // deferred-apply notification).
      void commitTitle(taskId, trimmed);
      closeFieldEditor(taskId, "title");
      setIsEditing(false);
    },
    [commitTitle, taskId],
  );

  const handleInputKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      // IME composition uses Enter to accept a candidate (AC-10).
      if (e.nativeEvent.isComposing || e.nativeEvent.keyCode === 229) return;
      if (e.key === "Enter") {
        e.preventDefault();
        const trimmed = draft.trim();
        if (!trimmed) return; // AC-7: invalid, stays open, no request
        if (trimmed === baseline) {
          closeEditor(); // AC-8
          return;
        }
        commit(trimmed); // AC-3
      } else if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        closeEditor(); // AC-4
      }
    },
    [baseline, closeEditor, commit, draft],
  );

  if (!isEditing) {
    return (
      <h1
        data-testid="office-task-title"
        tabIndex={0}
        className="text-xl font-semibold mt-4 rounded-sm outline-none focus-visible:ring-[2px] focus-visible:ring-ring/35"
        onDoubleClick={activate}
        onKeyDown={handleHeadingKeyDown}
      >
        {title}
      </h1>
    );
  }

  const trimmedDraft = draft.trim();
  const isInvalid = trimmedDraft.length === 0;

  return (
    <div className="mt-4">
      <Input
        ref={inputRef}
        data-testid="office-task-title-input"
        aria-label={t("task:taskTitle")}
        aria-invalid={isInvalid}
        autoFocus
        value={draft}
        onChange={(e) => setDraft(clampChange(e))}
        onFocus={(e) => e.target.select()}
        onBlur={closeEditor}
        onKeyDown={handleInputKeyDown}
        className="text-xl font-semibold h-auto py-1"
      />
      {isInvalid && (
        <p data-testid="office-task-title-error" className="mt-1 text-xs text-destructive">
          {t("task:titleRequired")}
        </p>
      )}
    </div>
  );
}
