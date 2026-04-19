"use client";

import { useMemo } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import type {
  FilterClause,
  GroupKey,
  SidebarView,
  SidebarViewDraft,
  SortSpec,
} from "@/lib/state/slices/ui/sidebar-view-types";
import { DIMENSION_METAS } from "./filter-dimension-registry";
import { FilterClauseEditor } from "./filter-clause-editor";
import { SortPicker } from "./sort-picker";
import { GroupPicker } from "./group-picker";
import { ViewHeaderRow, ViewSaveAsRow, ViewRenameRow } from "./view-manager";

type Props = {
  trigger: React.ReactNode;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

function makeClauseId(): string {
  return `c-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`;
}

type Current = { filters: FilterClause[]; sort: SortSpec; group: GroupKey };

function resolveCurrent(
  activeView: SidebarView | undefined,
  storedDraft: SidebarViewDraft | null,
): Current {
  if (storedDraft && activeView && storedDraft.baseViewId === activeView.id) {
    return { filters: storedDraft.filters, sort: storedDraft.sort, group: storedDraft.group };
  }
  if (!activeView) {
    return { filters: [], sort: { key: "state", direction: "asc" }, group: "none" };
  }
  return { filters: activeView.filters, sort: activeView.sort, group: activeView.group };
}

export function SidebarFilterPopover({ trigger, open, onOpenChange }: Props) {
  const views = useAppStore((s) => s.sidebarViews.views);
  const activeViewId = useAppStore((s) => s.sidebarViews.activeViewId);
  const storedDraft = useAppStore((s) => s.sidebarViews.draft);
  const updateDraft = useAppStore((s) => s.updateSidebarDraft);
  const saveAs = useAppStore((s) => s.saveSidebarDraftAs);
  const saveOverwrite = useAppStore((s) => s.saveSidebarDraftOverwrite);
  const discard = useAppStore((s) => s.discardSidebarDraft);
  const deleteView = useAppStore((s) => s.deleteSidebarView);
  const renameView = useAppStore((s) => s.renameSidebarView);

  const activeView: SidebarView | undefined = useMemo(
    () => views.find((v) => v.id === activeViewId),
    [views, activeViewId],
  );
  const current = useMemo(() => resolveCurrent(activeView, storedDraft), [storedDraft, activeView]);
  const hasDraft = !!storedDraft && activeView?.id === storedDraft.baseViewId;

  function handleAddFilter() {
    const defaultDim = DIMENSION_METAS[0];
    const newClause: FilterClause = {
      id: makeClauseId(),
      dimension: defaultDim.dimension,
      op: defaultDim.defaultOp,
      value: defaultDim.defaultValue,
    };
    updateDraft({ filters: [...current.filters, newClause] });
  }

  function handleChangeClause(next: FilterClause) {
    updateDraft({
      filters: current.filters.map((c) => (c.id === next.id ? next : c)),
    });
  }

  function handleRemoveClause(id: string) {
    updateDraft({ filters: current.filters.filter((c) => c.id !== id) });
  }

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent className="w-[22rem] p-0" align="end" data-testid="sidebar-filter-popover">
        <div className="border-b p-2">
          <ViewHeaderRow
            activeView={activeView}
            hasDraft={hasDraft}
            canDelete={views.length > 1}
            onSaveOverwrite={saveOverwrite}
            onDiscard={discard}
            onDelete={() => activeView && deleteView(activeView.id)}
          />
          <ViewSaveAsRow onSubmit={saveAs} />
          {activeView && <ViewRenameRow view={activeView} onRename={renameView} />}
        </div>

        <FilterSection
          filters={current.filters}
          onAdd={handleAddFilter}
          onChange={handleChangeClause}
          onRemove={handleRemoveClause}
        />

        <div className="border-b p-2">
          <SectionLabel>Sort</SectionLabel>
          <SortPicker value={current.sort} onChange={(sort) => updateDraft({ sort })} />
        </div>

        <div className="p-2">
          <SectionLabel>Group by</SectionLabel>
          <GroupPicker value={current.group} onChange={(group) => updateDraft({ group })} />
        </div>
      </PopoverContent>
    </Popover>
  );
}

const SECTION_LABEL_CLASS = "text-[11px] font-medium uppercase tracking-wide text-muted-foreground";

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <span className={`mb-1 block ${SECTION_LABEL_CLASS}`}>{children}</span>;
}

function FilterSection({
  filters,
  onAdd,
  onChange,
  onRemove,
}: {
  filters: FilterClause[];
  onAdd: () => void;
  onChange: (next: FilterClause) => void;
  onRemove: (id: string) => void;
}) {
  return (
    <div className="border-b p-2">
      <div className="mb-1 flex items-center justify-between">
        <span className={SECTION_LABEL_CLASS}>Filters</span>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="-my-1 h-6 cursor-pointer text-xs"
          onClick={onAdd}
          data-testid="filter-add-button"
        >
          <IconPlus className="mr-1 h-3 w-3" />
          Add
        </Button>
      </div>
      {filters.length > 0 && (
        <div className="space-y-0.5">
          {filters.map((clause) => (
            <FilterClauseEditor
              key={clause.id}
              clause={clause}
              onChange={onChange}
              onRemove={() => onRemove(clause.id)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
