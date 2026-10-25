"use client";

import { useEffect, useMemo, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { SavedTaskViewDeleteConfirmation } from "@/components/confirmation/saved-task-view-delete-confirmation";
import {
  useSavedTaskViewDeleteConfirmation,
  type SavedTaskViewDeleteTarget,
} from "@/components/confirmation/use-saved-task-view-delete-confirmation";
import type { ThreadCandidate } from "@/lib/threads/thread-view-query";
import type { ThreadView, ThreadViewDraft } from "@/lib/state/slices/ui/thread-view-types";
import type { Repository } from "@/lib/types/http";
import { ThreadsViewTaskPicker } from "./threads-view-task-picker";
import { EditorBody, type EditorBodyProps } from "./threads-view-editor-sections";
import { parseThreadMaxColumns } from "./threads-view-editor-utils";
import { threadViewName } from "@/lib/state/slices/ui/thread-view-builtins";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

type EditorProps = {
  activeView: ThreadView;
  draft: ThreadViewDraft | null;
  candidates: ThreadCandidate[];
  repositories: ReadonlyArray<Pick<Repository, "id" | "name">>;
  viewCount: number;
  canDelete: boolean;
  onUpdate: EditorBodyProps["onUpdate"];
  onSave: () => void;
  onSaveAs: (name: string) => void;
  onDiscard: () => void;
  onRename: (name: string) => void;
  onDelete: (viewId: string) => void;
  onDuplicate: () => void;
  onReapplySort: () => void;
  showHeader?: boolean;
  mobile?: boolean;
  gridHeightFallback?: boolean;
  deleteFocusBoundaryRef?: RefObject<HTMLElement | null>;
};

export function ThreadsViewEditor({
  activeView,
  draft,
  candidates,
  repositories,
  viewCount,
  canDelete,
  onUpdate,
  onSave,
  onSaveAs,
  onDiscard,
  onRename,
  onDelete,
  onDuplicate,
  onReapplySort,
  showHeader = true,
  mobile = false,
  gridHeightFallback = false,
  deleteFocusBoundaryRef,
}: EditorProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [pickerOpen, setPickerOpen] = useState(false);
  const [nameMode, setNameMode] = useState<"rename" | "saveAs" | null>(null);
  const [name, setName] = useState("");
  const current = draft && draft.baseViewId === activeView.id ? draft : activeView;
  const maxColumns = useMaxColumnsInput(activeView.id, current.maxColumns, onUpdate);
  const deletion = useSavedTaskViewDeleteConfirmation<HTMLButtonElement>([activeView]);
  const invalidSelectedScope =
    current.taskScope.mode === "selected" && current.taskScope.taskIds.length === 0;
  const hasDraft = !!draft && draft.baseViewId === activeView.id;
  const repositoryNames = useMemo(() => mapRepositoryNames(repositories), [repositories]);
  const invalidDraft = invalidSelectedScope || maxColumns.invalid;
  const deleteConfirmation = deletion.target ? (
    <ThreadViewDeleteConfirmation
      deletion={deletion}
      mobile={mobile}
      focusBoundaryRef={deleteFocusBoundaryRef}
      onConfirm={onDelete}
    />
  ) : null;

  if (pickerOpen) {
    return (
      <TaskPickerEditor
        candidates={candidates}
        current={current}
        onUpdate={onUpdate}
        onBack={() => setPickerOpen(false)}
      />
    );
  }

  return (
    <>
      <EditorBody
        activeView={activeView}
        current={current}
        candidates={candidates}
        repositoryNames={repositoryNames}
        mobile={mobile}
        gridHeightFallback={gridHeightFallback}
        showHeader={showHeader}
        nameMode={nameMode}
        name={name}
        maxColumnsInput={maxColumns.input}
        maxColumnsInvalid={maxColumns.invalid}
        invalidSelectedScope={invalidSelectedScope}
        invalidDraft={invalidDraft}
        hasDraft={hasDraft}
        viewCount={viewCount}
        canDelete={canDelete}
        onNameChange={setName}
        onNameModeChange={setNameMode}
        onUpdate={onUpdate}
        onSave={onSave}
        onSaveAs={onSaveAs}
        onRename={onRename}
        onDiscard={onDiscard}
        onDelete={() =>
          deletion.request({ id: activeView.id, label: threadViewName(activeView, t) })
        }
        deleteAnchorRef={deletion.anchorRef}
        deleteConfirmation={mobile && !isMobile ? deleteConfirmation : undefined}
        onDuplicate={onDuplicate}
        onReapplySort={onReapplySort}
        onSetMaxColumns={maxColumns.onChange}
        onOpenPicker={() => setPickerOpen(true)}
      />
      {!mobile || isMobile ? deleteConfirmation : null}
    </>
  );
}

function ThreadViewDeleteConfirmation({
  deletion,
  mobile,
  focusBoundaryRef,
  onConfirm,
}: {
  deletion: {
    target: SavedTaskViewDeleteTarget | null;
    anchorRef: RefObject<HTMLButtonElement | null>;
    close: () => void;
  };
  mobile: boolean;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  onConfirm: (id: string) => void;
}) {
  if (!deletion.target) return null;
  return (
    <SavedTaskViewDeleteConfirmation
      target={deletion.target}
      presentation={mobile ? "inline" : "popover"}
      open
      anchorRef={deletion.anchorRef}
      focusBoundaryRef={focusBoundaryRef}
      onOpenChange={(open) => {
        if (!open) deletion.close();
      }}
      onConfirm={onConfirm}
    />
  );
}

function TaskPickerEditor({
  candidates,
  current,
  onUpdate,
  onBack,
}: {
  candidates: ThreadCandidate[];
  current: ThreadView | ThreadViewDraft;
  onUpdate: EditorBodyProps["onUpdate"];
  onBack: () => void;
}) {
  return (
    <div className="flex min-h-0 flex-col" data-testid="threads-view-editor">
      <ThreadsViewTaskPicker
        candidates={candidates}
        selectedTaskIds={current.taskScope.mode === "selected" ? current.taskScope.taskIds : []}
        onChange={(taskIds) => onUpdate({ taskScope: { mode: "selected", taskIds } })}
        onBack={onBack}
      />
    </div>
  );
}

function mapRepositoryNames(
  repositories: ReadonlyArray<Pick<Repository, "id" | "name">>,
): Map<string, string> {
  return new Map(repositories.map((repository) => [String(repository.id), repository.name]));
}

function formatMaxColumns(value: number | null): string {
  return value === null ? "" : String(value);
}

function useMaxColumnsInput(
  viewId: string,
  value: number | null,
  onUpdate: EditorProps["onUpdate"],
) {
  const [input, setInput] = useState(() => formatMaxColumns(value));
  const [invalid, setInvalid] = useState(false);
  useEffect(() => {
    setInput(formatMaxColumns(value));
    setInvalid(false);
  }, [viewId, value]);
  return {
    input,
    invalid,
    onChange: (next: string, badInput = false) => {
      setInput(next);
      const parsed = parseThreadMaxColumns(next, badInput);
      setInvalid(parsed === undefined);
      if (parsed !== undefined) onUpdate({ maxColumns: parsed });
    },
  };
}
