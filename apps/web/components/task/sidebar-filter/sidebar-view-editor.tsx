"use client";

import type { ComponentProps } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type {
  FilterClause,
  GroupKey,
  SidebarView,
  SidebarViewDraft,
  SortSpec,
} from "@/lib/state/slices/ui/sidebar-view-types";
import { cloneSidebarTaskRowPresentation } from "@/lib/state/slices/ui/sidebar-task-row-presentation";
import { DIMENSION_METAS } from "./filter-dimension-registry";
import { FilterClauseEditor } from "./filter-clause-editor";
import { GroupPicker } from "./group-picker";
import { SortPicker } from "./sort-picker";
import { TaskRowSettings } from "./task-row-settings";
import { ViewHeaderRow } from "./view-manager";

export type SidebarViewEditorCurrent = {
  filters: FilterClause[];
  sort: SortSpec;
  group: GroupKey;
  taskRow: NonNullable<SidebarViewDraft["taskRow"]>;
};

type Props = {
  current: SidebarViewEditorCurrent;
  isDrawerLayout: boolean;
  headerProps: ComponentProps<typeof ViewHeaderRow>;
  onUpdate: (patch: Partial<SidebarViewEditorCurrent>) => void;
  onAddFilter: () => void;
  onChangeClause: (next: FilterClause) => void;
  onRemoveClause: (id: string) => void;
};

export function SidebarViewEditor({
  current,
  isDrawerLayout,
  headerProps,
  onUpdate,
  onAddFilter,
  onChangeClause,
  onRemoveClause,
}: Props) {
  const { t } = useTranslation();
  return (
    <>
      <div className="border-b p-2">
        <ViewHeaderRow {...headerProps} />
      </div>
      <FilterSection
        filters={current.filters}
        isDrawerLayout={isDrawerLayout}
        onAdd={onAddFilter}
        onChange={onChangeClause}
        onRemove={onRemoveClause}
      />
      <div className={`border-b px-2 pb-2 ${isDrawerLayout ? "pt-2" : "pt-0"}`}>
        <SectionLabel isDrawerLayout={isDrawerLayout}>{t("task:sort")}</SectionLabel>
        <SortPicker value={current.sort} onChange={(sort) => onUpdate({ sort })} />
      </div>
      <div className={`px-2 pb-2 ${isDrawerLayout ? "pt-2" : "pt-0"}`}>
        <SectionLabel isDrawerLayout={isDrawerLayout}>{t("task:groupBy")}</SectionLabel>
        <GroupPicker value={current.group} onChange={(group) => onUpdate({ group })} />
      </div>
      <TaskRowSettings
        value={current.taskRow}
        sort={current.sort}
        onChange={(taskRow) => onUpdate({ taskRow })}
      />
    </>
  );
}

const SECTION_LABEL_CLASS =
  "text-[11px] font-medium uppercase leading-none tracking-wide text-muted-foreground";

function SectionLabel({
  children,
  isDrawerLayout,
}: {
  children: React.ReactNode;
  isDrawerLayout: boolean;
}) {
  return (
    <span className={`${isDrawerLayout ? "" : "-mt-1 "}mb-1 block ${SECTION_LABEL_CLASS}`}>
      {children}
    </span>
  );
}

function FilterSection({
  filters,
  isDrawerLayout,
  onAdd,
  onChange,
  onRemove,
}: {
  filters: FilterClause[];
  isDrawerLayout: boolean;
  onAdd: () => void;
  onChange: (next: FilterClause) => void;
  onRemove: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className={`border-b px-2 pb-2 ${isDrawerLayout ? "pt-2" : "pt-0"}`}>
      <div className={`${isDrawerLayout ? "" : "-mt-1 "}mb-1 flex items-center justify-between`}>
        <span className={SECTION_LABEL_CLASS}>{t("task:filters")}</span>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className={`${isDrawerLayout ? "" : "-my-1 "}h-6 cursor-pointer text-xs`}
          onClick={onAdd}
          data-testid="filter-add-button"
        >
          <IconPlus className="mr-1 h-3 w-3" />
          {t("task:add")}
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

export function createSidebarViewEditorCurrent(
  activeView: SidebarView | undefined,
  storedDraft: SidebarViewDraft | null,
): SidebarViewEditorCurrent {
  const source =
    storedDraft && activeView && storedDraft.baseViewId === activeView.id
      ? storedDraft
      : activeView;
  return {
    filters: source?.filters ?? [],
    sort: source?.sort ?? { key: "state", direction: "asc" },
    group: source?.group ?? "none",
    taskRow: cloneSidebarTaskRowPresentation(source?.taskRow),
  };
}

export function createSidebarFilterClause(): FilterClause {
  const defaultDim = DIMENSION_METAS[0];
  return {
    id: `c-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`,
    dimension: defaultDim.dimension,
    op: defaultDim.defaultOp,
    value: defaultDim.defaultValue,
  };
}
