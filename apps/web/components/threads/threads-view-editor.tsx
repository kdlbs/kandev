"use client";

import { useEffect, useMemo, useState } from "react";
import type { ThreadCandidate } from "@/lib/threads/thread-view-query";
import type { ThreadView, ThreadViewDraft } from "@/lib/state/slices/ui/thread-view-types";
import type { Repository } from "@/lib/types/http";
import { ThreadsViewTaskPicker } from "./threads-view-task-picker";
import { EditorBody, type EditorBodyProps } from "./threads-view-editor-sections";
import { parseThreadMaxColumns } from "./threads-view-editor-utils";

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
  onDelete: () => void;
  onDuplicate: () => void;
  onReapplySort: () => void;
  showHeader?: boolean;
  mobile?: boolean;
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
}: EditorProps) {
  const [pickerOpen, setPickerOpen] = useState(false);
  const [nameMode, setNameMode] = useState<"rename" | "saveAs" | null>(null);
  const [name, setName] = useState("");
  const current = draft && draft.baseViewId === activeView.id ? draft : activeView;
  const [maxColumnsInput, setMaxColumnsInput] = useState(() =>
    formatMaxColumns(current.maxColumns),
  );
  const [maxColumnsInvalid, setMaxColumnsInvalid] = useState(false);
  const invalidSelectedScope =
    current.taskScope.mode === "selected" && current.taskScope.taskIds.length === 0;
  const hasDraft = !!draft && draft.baseViewId === activeView.id;
  const repositoryNames = useMemo(
    () =>
      new Map<string, string>(
        repositories.map((repository) => [String(repository.id), repository.name]),
      ),
    [repositories],
  );
  const invalidDraft = invalidSelectedScope || maxColumnsInvalid;

  useEffect(() => {
    setMaxColumnsInput(formatMaxColumns(current.maxColumns));
    setMaxColumnsInvalid(false);
  }, [activeView.id, current.maxColumns]);

  if (pickerOpen) {
    return (
      <div className="flex min-h-0 flex-col" data-testid="threads-view-editor">
        <ThreadsViewTaskPicker
          candidates={candidates}
          selectedTaskIds={current.taskScope.mode === "selected" ? current.taskScope.taskIds : []}
          onChange={(taskIds) => onUpdate({ taskScope: { mode: "selected", taskIds } })}
          onBack={() => setPickerOpen(false)}
        />
      </div>
    );
  }

  return (
    <EditorBody
      activeView={activeView}
      current={current}
      candidates={candidates}
      repositoryNames={repositoryNames}
      mobile={mobile}
      showHeader={showHeader}
      nameMode={nameMode}
      name={name}
      maxColumnsInput={maxColumnsInput}
      maxColumnsInvalid={maxColumnsInvalid}
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
      onDelete={onDelete}
      onDuplicate={onDuplicate}
      onReapplySort={onReapplySort}
      onSetMaxColumns={(value, badInput = false) => {
        setMaxColumnsInput(value);
        const parsed = parseThreadMaxColumns(value, badInput);
        setMaxColumnsInvalid(parsed === undefined);
        if (parsed !== undefined) onUpdate({ maxColumns: parsed });
      }}
      onOpenPicker={() => setPickerOpen(true)}
    />
  );
}

function formatMaxColumns(value: number | null): string {
  return value === null ? "" : String(value);
}
