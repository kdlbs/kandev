"use client";

import { useRef, useState, type RefObject } from "react";
import {
  IconArrowsSort,
  IconBookmark,
  IconCheck,
  IconCode,
  IconPlus,
  IconRefresh,
  IconSearch,
  IconTrash,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { savedViewLabel, type SavedView } from "./use-saved-views";
import type { SortKey } from "./filter-model";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { isActionConfirmationTarget } from "@/components/confirmation/action-confirm-popover";
import { SavedTaskViewDeleteConfirmation } from "@/components/confirmation/saved-task-view-delete-confirmation";
import {
  useSavedTaskViewDeleteConfirmation,
  type SavedTaskViewDeleteTarget,
} from "@/components/confirmation/use-saved-task-view-delete-confirmation";

/** `value` is the persisted SortKey; only the catalog key is copy. */
const SORT_OPTIONS: { value: SortKey; labelKey: string }[] = [
  { value: "updated", labelKey: "jira:sortUpdated" },
  { value: "created", labelKey: "jira:sortCreated" },
  { value: "priority", labelKey: "jira:sortPriority" },
];

type ListToolbarProps = {
  searchText: string;
  onSearchChange: (text: string) => void;
  views: SavedView[];
  activeViewId: string | null;
  onSelectView: (id: string) => void;
  onDeleteView: (id: string) => void;
  onSaveView: (name: string) => void;
  count: number;
  loading: boolean;
  sort: SortKey;
  onSortChange: (sort: SortKey) => void;
  onRefresh: () => void;
  showJqlEditor: boolean;
  onToggleJqlEditor: () => void;
};

export function ListToolbar({
  searchText,
  onSearchChange,
  views,
  activeViewId,
  onSelectView,
  onDeleteView,
  onSaveView,
  count,
  loading,
  sort,
  onSortChange,
  onRefresh,
  showJqlEditor,
  onToggleJqlEditor,
}: ListToolbarProps) {
  const { t } = useTranslation();
  const activeView = views.find((v) => v.id === activeViewId);
  const sortOption = SORT_OPTIONS.find((o) => o.value === sort);
  const sortLabel = t(sortOption?.labelKey ?? "jira:sortUpdated");
  return (
    <div className="flex items-center gap-2 px-6 py-2.5 border-b shrink-0 flex-wrap">
      <SearchInput value={searchText} onChange={onSearchChange} />
      <ViewsDropdown
        views={views}
        activeViewId={activeViewId}
        onSelect={onSelectView}
        onDelete={onDeleteView}
        activeName={activeView ? savedViewLabel(t, activeView) : undefined}
      />
      <SaveViewButton onSave={onSaveView} />
      <div className="ml-auto flex items-center gap-1">
        <span className="text-xs text-muted-foreground tabular-nums mr-2">
          {loading ? t("jira:loading2") : t("jira:ticketOnThisPage", { count })}
        </span>
        <SortDropdown sort={sort} sortLabel={sortLabel} onSortChange={onSortChange} />
        <Button
          variant="ghost"
          size="icon"
          onClick={onRefresh}
          disabled={loading}
          className="cursor-pointer"
          title={t("jira:refresh")}
        >
          <IconRefresh className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
        </Button>
        <Button
          variant={showJqlEditor ? "default" : "ghost"}
          onClick={onToggleJqlEditor}
          className="cursor-pointer text-xs gap-1.5"
          title={t("jira:toggleRawJqlEditor")}
        >
          <IconCode className="h-3.5 w-3.5" />
          JQL
        </Button>
      </div>
    </div>
  );
}

function SortDropdown({
  sort,
  sortLabel,
  onSortChange,
}: {
  sort: SortKey;
  sortLabel: string;
  onSortChange: (sort: SortKey) => void;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" className="cursor-pointer text-xs gap-1.5">
          <IconArrowsSort className="h-3.5 w-3.5" />
          {t("jira:sortLabelled", { label: sortLabel })}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40">
        {SORT_OPTIONS.map((o) => (
          <DropdownMenuCheckboxItem
            key={o.value}
            checked={sort === o.value}
            onCheckedChange={() => onSortChange(o.value)}
            className="cursor-pointer"
          >
            {t(o.labelKey)}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function SearchInput({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { t } = useTranslation();
  return (
    <div className="relative flex-1 max-w-md min-w-[200px]">
      <IconSearch className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t("jira:searchTicketKeyOrText")}
        className="text-xs pl-8"
      />
    </div>
  );
}

function ViewsDropdown({
  views,
  activeViewId,
  onSelect,
  onDelete,
  activeName,
}: {
  views: SavedView[];
  activeViewId: string | null;
  onSelect: (id: string) => void;
  onDelete: (id: string) => void;
  activeName: string | undefined;
}) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const [open, setOpen] = useState(false);
  const contentRef = useRef<HTMLDivElement>(null);
  const builtin = views.filter((v) => v.builtin);
  const custom = views.filter((v) => !v.builtin);
  const deletion = useSavedTaskViewDeleteConfirmation<HTMLButtonElement>(custom);

  const deleteProps: ViewDeletionProps = {
    isFinePointer,
    deleteTarget: deletion.target,
    deleteAnchorRef: deletion.anchorRef,
    onDeleteOpenChange: (nextOpen) => {
      if (!nextOpen) deletion.close();
    },
    onRequestDelete: (view) => deletion.request({ id: view.id, label: savedViewLabel(t, view) }),
    onConfirmDelete: onDelete,
    onRegisterDeleteAnchor: deletion.registerAnchor,
  };

  return (
    <Popover
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        if (!nextOpen) deletion.close();
      }}
    >
      <PopoverTrigger asChild>
        <Button variant="outline" className="cursor-pointer text-xs gap-1.5">
          <IconBookmark className="h-3.5 w-3.5" />
          {activeName ?? t("jira:noView")}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        ref={contentRef}
        align="start"
        className="w-60 max-w-[calc(100vw-1rem)] overflow-x-hidden p-0"
        onFocusOutside={(event) => {
          if (isActionConfirmationTarget(event.target)) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (isActionConfirmationTarget(event.target)) event.preventDefault();
        }}
      >
        <ViewsGroup
          label={t("jira:builtIn")}
          views={builtin}
          activeViewId={activeViewId}
          onSelect={(id) => {
            onSelect(id);
            setOpen(false);
          }}
          {...deleteProps}
        />
        {custom.length > 0 && (
          <>
            <div className="border-t" />
            <ViewsGroup
              label={t("jira:saved")}
              views={custom}
              activeViewId={activeViewId}
              onSelect={(id) => {
                onSelect(id);
                setOpen(false);
              }}
              {...deleteProps}
            />
          </>
        )}
      </PopoverContent>
      {isFinePointer && deletion.target ? (
        <SavedTaskViewDeleteConfirmation
          target={deletion.target}
          presentation="popover"
          open
          anchorRef={deletion.anchorRef}
          focusBoundaryRef={contentRef}
          onOpenChange={(nextOpen) => {
            if (!nextOpen) deletion.close();
          }}
          onConfirm={onDelete}
        />
      ) : null}
    </Popover>
  );
}

type ViewDeletionProps = {
  isFinePointer: boolean;
  deleteTarget: SavedTaskViewDeleteTarget | null;
  deleteAnchorRef: RefObject<HTMLElement | null>;
  onDeleteOpenChange: (open: boolean) => void;
  onRequestDelete: (view: SavedView) => void;
  onConfirmDelete: (id: string) => void;
  onRegisterDeleteAnchor: (id: string, element: HTMLButtonElement | null) => void;
};

function ViewsGroup({
  label,
  views,
  activeViewId,
  onSelect,
  isFinePointer,
  deleteTarget,
  deleteAnchorRef,
  onDeleteOpenChange,
  onRequestDelete,
  onConfirmDelete,
  onRegisterDeleteAnchor,
}: {
  label: string;
  views: SavedView[];
  activeViewId: string | null;
  onSelect: (id: string) => void;
} & ViewDeletionProps) {
  return (
    <div className="py-1">
      <div className="px-3 py-1 text-[10px] uppercase tracking-wider text-muted-foreground font-semibold">
        {label}
      </div>
      {views.map((v) => (
        <ViewRow
          key={v.id}
          view={v}
          active={v.id === activeViewId}
          onSelect={onSelect}
          isFinePointer={isFinePointer}
          deleteTarget={deleteTarget}
          deleteAnchorRef={deleteAnchorRef}
          onDeleteOpenChange={onDeleteOpenChange}
          onRequestDelete={onRequestDelete}
          onConfirmDelete={onConfirmDelete}
          onRegisterDeleteAnchor={onRegisterDeleteAnchor}
        />
      ))}
    </div>
  );
}

function ViewRow({
  view,
  active,
  onSelect,
  isFinePointer,
  deleteTarget,
  deleteAnchorRef,
  onDeleteOpenChange,
  onRequestDelete,
  onConfirmDelete,
  onRegisterDeleteAnchor,
}: {
  view: SavedView;
  active: boolean;
  onSelect: (id: string) => void;
} & ViewDeletionProps) {
  const { t } = useTranslation();
  const label = savedViewLabel(t, view);

  if (!isFinePointer && deleteTarget?.id === view.id) {
    return (
      <div className="min-w-0 px-2 py-1">
        <SavedTaskViewDeleteConfirmation
          target={deleteTarget}
          presentation="inline"
          open
          anchorRef={deleteAnchorRef}
          onOpenChange={onDeleteOpenChange}
          onConfirm={onConfirmDelete}
        />
      </div>
    );
  }

  return (
    <div className="group flex min-w-0 items-center px-2">
      <button
        type="button"
        onClick={() => onSelect(view.id)}
        className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-muted/50"
      >
        <IconCheck className={`h-3.5 w-3.5 ${active ? "opacity-100" : "opacity-0"}`} />
        <span className="min-w-0 flex-1 truncate">{label}</span>
      </button>
      {!view.builtin && (
        <button
          ref={(element) => onRegisterDeleteAnchor(view.id, element)}
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onRequestDelete(view);
          }}
          className={cn(
            "flex shrink-0 cursor-pointer items-center justify-center rounded hover:bg-muted",
            isFinePointer
              ? "h-7 w-7 opacity-0 group-hover:opacity-100 focus:opacity-100"
              : "h-12 w-12 opacity-100",
          )}
          title={t("jira:deleteView")}
          aria-label={t("common:deleteSavedTaskViewAction", { name: label })}
        >
          <IconTrash className="h-3.5 w-3.5 text-muted-foreground" />
        </button>
      )}
    </div>
  );
}

function SaveViewButton({ onSave }: { onSave: (name: string) => void }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const submit = () => {
    const trimmed = name.trim();
    if (!trimmed) return;
    onSave(trimmed);
    setName("");
    setOpen(false);
  };
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          className="cursor-pointer text-xs gap-1.5"
          title={t("jira:saveCurrentFiltersAsAView")}
        >
          <IconPlus className="h-3.5 w-3.5" />
          {t("jira:saveView")}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-64 p-3 space-y-2">
        <div className="text-xs font-semibold">{t("jira:saveCurrentFiltersAs")}</div>
        <Input
          autoFocus
          value={name}
          onChange={(e) => setName(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") submit();
          }}
          placeholder={t("jira:myOpenBugs")}
          className="text-xs"
        />
        <div className="flex justify-end gap-1">
          <Button variant="ghost" onClick={() => setOpen(false)} className="cursor-pointer text-xs">
            {t("common:cancel")}
          </Button>
          <Button onClick={submit} disabled={!name.trim()} className="cursor-pointer text-xs">
            {t("common:save")}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
