"use client";

import { useState } from "react";
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
          size="icon-sm"
          onClick={onRefresh}
          disabled={loading}
          className="cursor-pointer h-7 w-7"
          title={t("jira:refresh")}
        >
          <IconRefresh className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
        </Button>
        <Button
          variant={showJqlEditor ? "default" : "ghost"}
          size="sm"
          onClick={onToggleJqlEditor}
          className="cursor-pointer h-7 text-xs gap-1.5"
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
        <Button variant="ghost" size="sm" className="cursor-pointer h-7 text-xs gap-1.5">
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
        className="h-8 text-xs pl-8"
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
  const [open, setOpen] = useState(false);
  const builtin = views.filter((v) => v.builtin);
  const custom = views.filter((v) => !v.builtin);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="cursor-pointer h-8 text-xs gap-1.5">
          <IconBookmark className="h-3.5 w-3.5" />
          {activeName ?? t("jira:noView")}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-60 p-0">
        <ViewsGroup
          label={t("jira:builtIn")}
          views={builtin}
          activeViewId={activeViewId}
          onSelect={(id) => {
            onSelect(id);
            setOpen(false);
          }}
          onDelete={onDelete}
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
              onDelete={onDelete}
            />
          </>
        )}
      </PopoverContent>
    </Popover>
  );
}

function ViewsGroup({
  label,
  views,
  activeViewId,
  onSelect,
  onDelete,
}: {
  label: string;
  views: SavedView[];
  activeViewId: string | null;
  onSelect: (id: string) => void;
  onDelete: (id: string) => void;
}) {
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
          onDelete={onDelete}
        />
      ))}
    </div>
  );
}

function ViewRow({
  view,
  active,
  onSelect,
  onDelete,
}: {
  view: SavedView;
  active: boolean;
  onSelect: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="group flex items-center px-2">
      <button
        type="button"
        onClick={() => onSelect(view.id)}
        className="flex-1 flex items-center gap-2 px-2 py-1.5 text-sm cursor-pointer rounded hover:bg-muted/50"
      >
        <IconCheck className={`h-3.5 w-3.5 ${active ? "opacity-100" : "opacity-0"}`} />
        <span className="truncate">{savedViewLabel(t, view)}</span>
      </button>
      {!view.builtin && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onDelete(view.id);
          }}
          className="cursor-pointer opacity-0 group-hover:opacity-100 p-1 rounded hover:bg-muted"
          title={t("jira:deleteView")}
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
          size="sm"
          className="cursor-pointer h-8 text-xs gap-1.5"
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
          className="h-8 text-xs"
        />
        <div className="flex justify-end gap-1">
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setOpen(false)}
            className="cursor-pointer h-7 text-xs"
          >
            {t("common:cancel")}
          </Button>
          <Button
            size="sm"
            onClick={submit}
            disabled={!name.trim()}
            className="cursor-pointer h-7 text-xs"
          >
            {t("common:save")}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
