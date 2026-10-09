"use client";

import type { ComponentProps } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
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
import { sortKeyLabelKey } from "./sort-picker";
import { SortChainEditor, sortRuleDirectionLabelKey } from "./sort-chain-editor";
import { TaskRowSettings } from "./task-row-settings";
import { SidebarSettingsDisclosure } from "./sidebar-settings-disclosure";
import { AutomaticColorSettings } from "./automatic-color-settings";
import { ViewHeaderRow } from "./view-manager";
import { sidebarSortRules } from "@/lib/sidebar/sidebar-sort-chain";

export type SidebarViewEditorCurrent = {
  filters: FilterClause[];
  sort: SortSpec;
  sortWarningCount?: number;
  group: GroupKey;
  groupIndent: boolean;
  taskRow: NonNullable<SidebarViewDraft["taskRow"]>;
};

type Props = {
  current: SidebarViewEditorCurrent;
  isDrawerLayout: boolean;
  reorderScopeKey: string;
  headerProps: ComponentProps<typeof ViewHeaderRow>;
  onUpdate: (patch: Partial<SidebarViewEditorCurrent>) => void;
  onAddFilter: () => void;
  onChangeClause: (next: FilterClause) => void;
  onRemoveClause: (id: string) => void;
};

export function SidebarViewEditor({
  current,
  isDrawerLayout,
  reorderScopeKey,
  headerProps,
  onUpdate,
  onAddFilter,
  onChangeClause,
  onRemoveClause,
}: Props) {
  const { t } = useTranslation();
  const sortSummary = t("task:sortChainSummary", {
    rules: sidebarSortRules(current.sort)
      .map((rule) => {
        if (rule.key === "custom") return t(sortKeyLabelKey(rule.key));
        const field =
          rule.key === "color"
            ? `${t(sortKeyLabelKey(rule.key))}: ${t(`task:color${(rule.color ?? "red")[0].toUpperCase()}${(rule.color ?? "red").slice(1)}`)}`
            : t(sortKeyLabelKey(rule.key));
        return t("task:sortRuleSummary", {
          field,
          order: t(sortRuleDirectionLabelKey(rule.key, rule.direction)),
        });
      })
      .join(" · "),
  });
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
      <SidebarSettingsDisclosure
        title={t("task:sort")}
        summary={sortSummary}
        testId="sidebar-sort-settings"
        className="border-b"
        contentClassName="pt-1"
      >
        <SortChainEditor
          value={current.sort}
          onChange={(sort) => onUpdate({ sort })}
          isDrawerLayout={isDrawerLayout}
          reorderScopeKey={reorderScopeKey}
          warningCount={current.sortWarningCount ?? 0}
        />
      </SidebarSettingsDisclosure>
      <SidebarSettingsDisclosure
        title={t("task:groupBy")}
        summary={t(GROUP_SUMMARY_KEYS[current.group])}
        testId="sidebar-group-settings"
        className="border-b"
        contentClassName="pt-1"
      >
        <GroupPicker value={current.group} onChange={(group) => onUpdate({ group })} />
        <label
          htmlFor="sidebar-group-indent"
          className="flex min-h-11 cursor-pointer items-center justify-between gap-3 px-1 text-xs"
        >
          <span>{t("task:indentGroupedTasks")}</span>
          <Switch
            id="sidebar-group-indent"
            size="sm"
            checked={current.groupIndent}
            onCheckedChange={(groupIndent) => onUpdate({ groupIndent })}
            aria-label={t("task:indentGroupedTasks")}
          />
        </label>
      </SidebarSettingsDisclosure>
      <TaskRowSettings
        value={current.taskRow}
        sort={current.sort}
        isDrawerLayout={isDrawerLayout}
        reorderScopeKey={reorderScopeKey}
        onChange={(taskRow) => onUpdate({ taskRow })}
      />
      <AutomaticColorSettings isDrawerLayout={isDrawerLayout} />
    </>
  );
}

const SECTION_LABEL_CLASS =
  "text-[11px] font-medium uppercase leading-none tracking-wide text-muted-foreground";

const GROUP_SUMMARY_KEYS: Record<GroupKey, string> = {
  none: "task:groupNone",
  repository: "task:groupRepository",
  workflow: "task:groupWorkflow",
  workflowStep: "task:groupWorkflowStep",
  executorType: "task:groupExecutorType",
  state: "task:groupState",
};

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
    <div className="border-b px-2 pb-2 pt-2.5">
      <div className="mb-1 flex items-center justify-between">
        <span className={SECTION_LABEL_CLASS}>{t("task:filters")}</span>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className={`h-6 ${isDrawerLayout ? "min-h-11 " : ""}cursor-pointer text-xs`}
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
    ...(source?.sortWarningCount !== undefined
      ? { sortWarningCount: source.sortWarningCount }
      : {}),
    group: source?.group ?? "none",
    groupIndent: source?.groupIndent !== false,
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
