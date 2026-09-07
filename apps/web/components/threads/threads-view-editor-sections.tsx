"use client";

import { IconPlus } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { ThreadCandidate } from "@/lib/threads/thread-view-query";
import type {
  ThreadFilterClause,
  ThreadSortSpec,
  ThreadTaskScope,
  ThreadView,
  ThreadViewDraft,
} from "@/lib/state/slices/ui/thread-view-types";
import { TypedSortPicker } from "@/components/task/sidebar-filter/sort-picker-primitive";
import { ThreadsViewFilterRow } from "./threads-view-filter-row";
import { EditorActions, SectionLabel } from "./threads-view-editor-actions";
import {
  MAX_THREAD_VIEW_FILTERS,
  THREAD_SORT_OPTIONS,
  createThreadFilterClause,
} from "./threads-view-filter-registry";
import { MAX_THREAD_VIEW_COLUMNS } from "./threads-view-editor-utils";
import { threadViewName } from "@/lib/state/slices/ui/thread-view-builtins";

type ThreadViewDraftUpdate = (
  patch: Partial<Pick<ThreadView, "taskScope" | "filters" | "sort" | "maxColumns">>,
) => void;

export type EditorBodyProps = {
  activeView: ThreadView;
  current: ThreadView | ThreadViewDraft;
  candidates: ThreadCandidate[];
  repositoryNames: ReadonlyMap<string, string>;
  mobile: boolean;
  showHeader: boolean;
  nameMode: "rename" | "saveAs" | null;
  name: string;
  maxColumnsInput: string;
  maxColumnsInvalid: boolean;
  invalidSelectedScope: boolean;
  invalidDraft: boolean;
  hasDraft: boolean;
  viewCount: number;
  canDelete: boolean;
  onNameChange: (name: string) => void;
  onNameModeChange: (mode: "rename" | "saveAs" | null) => void;
  onUpdate: ThreadViewDraftUpdate;
  onSave: () => void;
  onSaveAs: (name: string) => void;
  onRename: (name: string) => void;
  onDiscard: () => void;
  onDelete: () => void;
  onDuplicate: () => void;
  onReapplySort: () => void;
  onSetMaxColumns: (value: string, badInput?: boolean) => void;
  onOpenPicker: () => void;
};

export function EditorBody({
  activeView,
  current,
  candidates,
  repositoryNames,
  mobile,
  showHeader,
  nameMode,
  name,
  maxColumnsInput,
  maxColumnsInvalid,
  invalidSelectedScope,
  invalidDraft,
  hasDraft,
  viewCount,
  canDelete,
  onNameChange,
  onNameModeChange,
  onUpdate,
  onSave,
  onSaveAs,
  onRename,
  onDiscard,
  onDelete,
  onDuplicate,
  onReapplySort,
  onSetMaxColumns,
  onOpenPicker,
}: EditorBodyProps) {
  return (
    <div
      className={`flex min-h-0 flex-col${mobile ? " [&_button]:min-h-11 [&_input]:min-h-11 [&_[role=combobox]]:min-h-11" : ""}`}
      data-testid="threads-view-editor"
    >
      {showHeader && (
        <EditorHeader
          activeView={activeView}
          nameMode={nameMode}
          name={name}
          onNameChange={onNameChange}
          onNameModeChange={onNameModeChange}
          onSubmit={(value) => {
            if (nameMode === "rename") onRename(value);
            if (nameMode === "saveAs") onSaveAs(value);
            onNameModeChange(null);
            onNameChange("");
          }}
          onDuplicate={onDuplicate}
        />
      )}
      <EditorSections
        current={current}
        candidates={candidates}
        repositoryNames={repositoryNames}
        mobile={mobile}
        invalidSelectedScope={invalidSelectedScope}
        invalidDraft={invalidDraft}
        hasDraft={hasDraft}
        viewCount={viewCount}
        canDelete={canDelete}
        maxColumnsInput={maxColumnsInput}
        maxColumnsInvalid={maxColumnsInvalid}
        onUpdate={onUpdate}
        onSave={onSave}
        onDiscard={onDiscard}
        onDelete={onDelete}
        onNameChange={onNameChange}
        onNameModeChange={onNameModeChange}
        onReapplySort={onReapplySort}
        onSetMaxColumns={onSetMaxColumns}
        onOpenPicker={onOpenPicker}
      />
    </div>
  );
}

type EditorSectionsProps = Pick<
  EditorBodyProps,
  | "current"
  | "candidates"
  | "repositoryNames"
  | "mobile"
  | "invalidSelectedScope"
  | "invalidDraft"
  | "hasDraft"
  | "viewCount"
  | "canDelete"
  | "maxColumnsInput"
  | "maxColumnsInvalid"
  | "onUpdate"
  | "onSave"
  | "onDiscard"
  | "onDelete"
  | "onNameChange"
  | "onNameModeChange"
  | "onReapplySort"
  | "onSetMaxColumns"
  | "onOpenPicker"
>;

function EditorSections({
  current,
  candidates,
  repositoryNames,
  mobile,
  invalidSelectedScope,
  invalidDraft,
  hasDraft,
  viewCount,
  canDelete,
  maxColumnsInput,
  maxColumnsInvalid,
  onUpdate,
  onSave,
  onDiscard,
  onDelete,
  onNameChange,
  onNameModeChange,
  onReapplySort,
  onSetMaxColumns,
  onOpenPicker,
}: EditorSectionsProps) {
  return (
    <>
      <TaskScopeSection
        current={current}
        invalid={invalidSelectedScope}
        mobile={mobile}
        onChange={(scope) => onUpdate({ taskScope: scope })}
        onOpenPicker={onOpenPicker}
      />
      <ThreadFilterSection
        filters={current.filters}
        candidates={candidates}
        repositoryNames={repositoryNames}
        mobile={mobile}
        onAdd={() => {
          if (current.filters.length < MAX_THREAD_VIEW_FILTERS) {
            onUpdate({ filters: [...current.filters, createThreadFilterClause()] });
          }
        }}
        onChange={(next) =>
          onUpdate({
            filters: current.filters.map((filter) => (filter.id === next.id ? next : filter)),
          })
        }
        onRemove={(id) =>
          onUpdate({ filters: current.filters.filter((filter) => filter.id !== id) })
        }
      />
      <ThreadSortSection
        sort={current.sort}
        mobile={mobile}
        onChange={(sort) => onUpdate({ sort })}
        onReapplySort={onReapplySort}
      />
      <MaxColumnsSection
        value={maxColumnsInput}
        invalid={maxColumnsInvalid}
        mobile={mobile}
        onChange={onSetMaxColumns}
      />
      <EditorActions
        hasDraft={hasDraft}
        canDelete={canDelete}
        viewCount={viewCount}
        invalidDraft={invalidDraft}
        onSave={onSave}
        onSaveAs={() => {
          onNameChange("");
          onNameModeChange("saveAs");
        }}
        onDiscard={onDiscard}
        onDelete={onDelete}
      />
    </>
  );
}

function EditorHeader({
  activeView,
  nameMode,
  name,
  onNameChange,
  onNameModeChange,
  onSubmit,
  onDuplicate,
}: {
  activeView: ThreadView;
  nameMode: "rename" | "saveAs" | null;
  name: string;
  onNameChange: (name: string) => void;
  onNameModeChange: (mode: "rename" | "saveAs" | null) => void;
  onSubmit: (name: string) => void;
  onDuplicate: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-2 border-b p-2">
      <div className="min-w-0">
        <p className="text-sm font-medium">{t("threads:viewSettings")}</p>
        <p className="truncate text-xs text-muted-foreground">{threadViewName(activeView, t)}</p>
      </div>
      {nameMode ? (
        <NameEditor
          name={name}
          onChange={onNameChange}
          onCancel={() => onNameModeChange(null)}
          onSubmit={onSubmit}
        />
      ) : (
        <div className="flex shrink-0 items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-8 cursor-pointer px-2 text-xs"
            onClick={() => {
              onNameChange(activeView.name);
              onNameModeChange("rename");
            }}
            data-testid="threads-view-rename"
          >
            {t("common:rename")}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-8 cursor-pointer px-2 text-xs"
            onClick={onDuplicate}
            data-testid="threads-view-duplicate"
          >
            {t("threads:saveAs")}
          </Button>
        </div>
      )}
    </div>
  );
}

function NameEditor({
  name,
  onChange,
  onCancel,
  onSubmit,
}: {
  name: string;
  onChange: (name: string) => void;
  onCancel: () => void;
  onSubmit: (name: string) => void;
}) {
  const { t } = useTranslation();
  const submit = () => {
    const trimmed = name.trim();
    if (trimmed) onSubmit(trimmed);
  };
  return (
    <div className="flex min-w-0 items-center gap-1">
      <Input
        autoFocus
        value={name}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") submit();
          if (event.key === "Escape") onCancel();
        }}
        placeholder={t("threads:viewName")}
        aria-label={t("threads:viewName")}
        data-testid="threads-view-name-input"
      />
      <Button
        type="button"
        size="sm"
        className="h-8 cursor-pointer"
        onClick={submit}
        disabled={!name.trim()}
      >
        {t("common:save")}
      </Button>
    </div>
  );
}

function TaskScopeSection({
  current,
  invalid,
  mobile,
  onChange,
  onOpenPicker,
}: {
  current: Pick<ThreadView, "taskScope">;
  invalid: boolean;
  mobile: boolean;
  onChange: (scope: ThreadTaskScope) => void;
  onOpenPicker: () => void;
}) {
  const { t } = useTranslation();
  const setMode = (mode: ThreadTaskScope["mode"]) => {
    onChange(
      mode === "all"
        ? { mode: "all", taskIds: [] }
        : {
            mode: "selected",
            taskIds: current.taskScope.mode === "selected" ? [...current.taskScope.taskIds] : [],
          },
    );
  };
  return (
    <div className="space-y-2 border-b p-2">
      <SectionLabel>{t("threads:taskScope")}</SectionLabel>
      <Select
        value={current.taskScope.mode}
        onValueChange={(value) => setMode(value as ThreadTaskScope["mode"])}
      >
        <SelectTrigger
          className={`${mobile ? "h-11" : "h-9"} w-full text-xs`}
          data-testid="threads-scope-select"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("threads:allEligibleTasks")}</SelectItem>
          <SelectItem value="selected">{t("threads:selectedTasks")}</SelectItem>
        </SelectContent>
      </Select>
      {current.taskScope.mode === "selected" && (
        <>
          <Button
            type="button"
            variant="outline"
            className={`${mobile ? "h-11" : "h-9"} w-full cursor-pointer justify-between text-xs`}
            onClick={onOpenPicker}
            data-testid="threads-open-task-picker"
          >
            <span>{t("threads:chooseTasks")}</span>
            <span className="text-muted-foreground">{current.taskScope.taskIds.length}</span>
          </Button>
          {invalid && (
            <p className="text-xs text-destructive" role="alert">
              {t("threads:selectAtLeastOneTask")}
            </p>
          )}
        </>
      )}
    </div>
  );
}

function ThreadFilterSection({
  filters,
  candidates,
  repositoryNames,
  mobile,
  onAdd,
  onChange,
  onRemove,
}: {
  filters: ThreadFilterClause[];
  candidates: ThreadCandidate[];
  repositoryNames: ReadonlyMap<string, string>;
  mobile: boolean;
  onAdd: () => void;
  onChange: (next: ThreadFilterClause) => void;
  onRemove: (id: string) => void;
}) {
  const { t } = useTranslation();
  const atLimit = filters.length >= MAX_THREAD_VIEW_FILTERS;
  return (
    <div className="space-y-1 border-b p-2">
      <div className="flex items-center justify-between gap-2">
        <SectionLabel>{t("threads:filters")}</SectionLabel>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className={`${mobile ? "h-11" : "h-8"} cursor-pointer px-2 text-xs`}
          onClick={onAdd}
          disabled={atLimit}
          title={
            atLimit
              ? t("threads:filterLimitReached", { count: MAX_THREAD_VIEW_FILTERS })
              : undefined
          }
          data-testid="threads-filter-add"
        >
          <IconPlus className="mr-1 h-3.5 w-3.5" />
          {t("task:add")}
        </Button>
      </div>
      {atLimit && (
        <p className="text-xs text-muted-foreground">
          {t("threads:filterLimitReached", { count: MAX_THREAD_VIEW_FILTERS })}
        </p>
      )}
      {filters.map((filter) => (
        <ThreadsViewFilterRow
          key={filter.id}
          clause={filter}
          candidates={candidates}
          repositoryNames={repositoryNames}
          mobile={mobile}
          onChange={onChange}
          onRemove={() => onRemove(filter.id)}
        />
      ))}
    </div>
  );
}

function ThreadSortSection({
  sort,
  mobile,
  onChange,
  onReapplySort,
}: {
  sort: ThreadSortSpec;
  mobile: boolean;
  onChange: (sort: ThreadSortSpec) => void;
  onReapplySort: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 border-b p-2">
      <SectionLabel>{t("threads:sort")}</SectionLabel>
      <div className="flex items-center gap-1">
        <TypedSortPicker
          value={sort}
          options={THREAD_SORT_OPTIONS.map((option) => ({
            key: option.key,
            label: t(option.labelKey),
            description: t(option.descriptionKey),
          }))}
          onChange={onChange}
          mobile={mobile}
          directionLabel={t("threads:toggleSortDirection")}
          testIds={{ key: "threads-sort-select", direction: "threads-sort-direction" }}
        />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className={`${mobile ? "h-11" : "h-9"} cursor-pointer px-2 text-xs`}
          onClick={onReapplySort}
          data-testid="threads-reapply-sort"
        >
          {t("threads:reapplySort")}
        </Button>
      </div>
    </div>
  );
}

function MaxColumnsSection({
  value,
  invalid,
  mobile,
  onChange,
}: {
  value: string;
  invalid: boolean;
  mobile: boolean;
  onChange: (value: string, badInput?: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 border-b p-2">
      <label
        className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground"
        htmlFor="threads-max-columns"
      >
        {t("threads:maxColumns")}
      </label>
      <Input
        id="threads-max-columns"
        type="number"
        min={1}
        max={MAX_THREAD_VIEW_COLUMNS}
        step={1}
        value={value}
        onChange={(event) => onChange(event.target.value, event.target.validity.badInput)}
        className={`${mobile ? "h-11" : "h-9"} text-xs`}
        placeholder={t("threads:noColumnLimit")}
        data-testid="threads-max-columns"
        aria-invalid={invalid}
        aria-describedby={invalid ? "threads-max-columns-error" : undefined}
      />
      {invalid && (
        <p id="threads-max-columns-error" className="text-xs text-destructive" role="alert">
          {t("threads:maxColumnsInvalid")}
        </p>
      )}
    </div>
  );
}
