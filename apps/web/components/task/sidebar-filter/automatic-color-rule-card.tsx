"use client";

import { IconArrowDown, IconArrowUp, IconGripVertical, IconTrash } from "@tabler/icons-react";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { Button } from "@kandev/ui/button";
import type {
  SidebarTaskColorDimension,
  SidebarTaskColorRule,
} from "@/lib/task-color-automation-settings";
import type { RepositoryRuleCatalogOption } from "@/lib/sidebar/repository-rule-catalog";
import type { TaskColorRuleOption, TaskColorRuleOptionMap } from "./task-color-rule-options";
import { taskColorRuleOptionKey } from "./task-color-rule-options";
import {
  AutomaticColorConditionFields,
  AutomaticColorEnableField,
  AutomaticColorOutputField,
  isCompleteRule,
  optionWithCurrentValue,
} from "./automatic-color-rule-fields";
import type { Translate } from "./automatic-color-repository-picker";

export function AutomaticColorRuleCard({
  rule,
  index,
  total,
  scalarOptions,
  repositoryOptions,
  repositoryQuery,
  repositoryLoading,
  repositoryError,
  onRepositoryQueryChange,
  onRefreshRepositories,
  isDrawerLayout,
  onChange,
  onRemove,
  onOpenRepository,
  onMove,
  t,
}: {
  rule: SidebarTaskColorRule;
  index: number;
  total: number;
  scalarOptions: TaskColorRuleOptionMap;
  repositoryOptions: readonly RepositoryRuleCatalogOption[];
  repositoryQuery: string;
  repositoryLoading: boolean;
  repositoryError: Error | null;
  onRepositoryQueryChange: (query: string) => void;
  onRefreshRepositories: () => void;
  isDrawerLayout: boolean;
  onChange: (rule: SidebarTaskColorRule) => void;
  onRemove: () => void;
  onOpenRepository: () => void;
  onMove: (direction: -1 | 1) => void;
  t: Translate;
}) {
  const sortable = useSortable({ id: rule.id });
  const rawOptions = getRuleOptions(rule, scalarOptions, repositoryOptions);
  const options = optionWithCurrentValue(rawOptions, rule, t);
  const selectedOption = options.find(
    (option) => option.key === taskColorRuleOptionKey(rule.condition.value),
  );
  const complete = isCompleteRule(rule, selectedOption);

  return (
    <div
      ref={sortable.setNodeRef}
      style={{
        transform: CSS.Transform.toString(sortable.transform),
        transition: sortable.transition,
      }}
      className="min-w-0 rounded-md border border-border/60 p-2"
      data-testid={`automatic-color-rule-${rule.id}`}
      data-dragging={sortable.isDragging ? "true" : undefined}
    >
      <AutomaticColorRuleHeader
        ruleId={rule.id}
        index={index}
        total={total}
        sortable={sortable}
        onRemove={onRemove}
        onMove={onMove}
        t={t}
      />
      <div className="space-y-2 pl-0 md:pl-11">
        <AutomaticColorConditionFields
          rule={rule}
          options={options}
          repositoryOptions={repositoryOptions}
          selectedOption={selectedOption}
          isDrawerLayout={isDrawerLayout}
          repositoryQuery={repositoryQuery}
          repositoryLoading={repositoryLoading}
          repositoryError={repositoryError}
          onRepositoryQueryChange={onRepositoryQueryChange}
          onRefreshRepositories={onRefreshRepositories}
          onOpenRepository={onOpenRepository}
          onChange={onChange}
          onConditionChange={(dimension) => onChange(ruleWithCondition(rule, dimension))}
          onTargetChange={(key) => {
            const next = ruleWithTarget(rule, options, key);
            if (next) onChange(next);
          }}
          t={t}
        />
        <AutomaticColorOutputField rule={rule} onChange={onChange} t={t} />
        <AutomaticColorEnableField
          rule={rule}
          complete={complete}
          onChange={(enabled) => onChange({ ...rule, enabled })}
          t={t}
        />
      </div>
    </div>
  );
}

function getRuleOptions(
  rule: SidebarTaskColorRule,
  scalarOptions: TaskColorRuleOptionMap,
  repositoryOptions: readonly RepositoryRuleCatalogOption[],
): readonly TaskColorRuleOption[] {
  if (rule.condition.dimension !== "repository") {
    return scalarOptions[rule.condition.dimension];
  }
  return repositoryOptions.map((option) => ({
    key: option.key,
    value: option.target,
    label: option.label,
    secondaryLabel: option.secondaryLabel,
    available: option.available,
  }));
}

function ruleWithCondition(
  rule: SidebarTaskColorRule,
  dimension: SidebarTaskColorDimension,
): SidebarTaskColorRule {
  const output =
    dimension === "workflow_step" && rule.output.kind === "workflow_step"
      ? rule.output
      : { kind: "fixed" as const, color: "gray" as const };
  return {
    ...rule,
    enabled: false,
    condition: { dimension, value: null, label: "" },
    output,
  };
}

function ruleWithTarget(
  rule: SidebarTaskColorRule,
  options: readonly TaskColorRuleOption[],
  key: string,
): SidebarTaskColorRule | null {
  const selected = options.find((option) => option.key === key);
  if (!selected?.available) return null;
  return {
    ...rule,
    condition: { ...rule.condition, value: selected.value, label: selected.label },
  };
}

function AutomaticColorRuleHeader({
  ruleId,
  index,
  total,
  sortable,
  onRemove,
  onMove,
  t,
}: {
  ruleId: string;
  index: number;
  total: number;
  sortable: ReturnType<typeof useSortable>;
  onRemove: () => void;
  onMove: (direction: -1 | 1) => void;
  t: Translate;
}) {
  return (
    <div className="flex min-h-11 items-center gap-1">
      <button
        type="button"
        className="flex min-h-11 min-w-11 shrink-0 cursor-grab touch-none items-center justify-center text-muted-foreground/60 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-label={t("task:automaticColorsReorderHandle", { number: index + 1 })}
        data-testid={`automatic-color-rule-handle-${ruleId}`}
        {...sortable.attributes}
        {...sortable.listeners}
        aria-roledescription={t("task:automaticColorsReorderable")}
      >
        <IconGripVertical className="size-4" aria-hidden="true" />
      </button>
      <span className="min-w-0 flex-1 text-xs font-medium">
        {t("task:automaticColorsRule", { number: index + 1 })}
      </span>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-11 cursor-pointer md:size-7"
        onClick={() => onMove(-1)}
        disabled={index === 0}
        aria-label={t("task:automaticColorsMoveUp")}
        data-testid={`automatic-color-rule-up-${ruleId}`}
      >
        <IconArrowUp className="size-3.5" aria-hidden="true" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-11 cursor-pointer md:size-7"
        onClick={() => onMove(1)}
        disabled={index === total - 1}
        aria-label={t("task:automaticColorsMoveDown")}
        data-testid={`automatic-color-rule-down-${ruleId}`}
      >
        <IconArrowDown className="size-3.5" aria-hidden="true" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-11 cursor-pointer text-muted-foreground hover:text-destructive md:size-7"
        onClick={onRemove}
        aria-label={t("task:automaticColorsRemoveRule")}
        data-testid={`automatic-color-rule-remove-${ruleId}`}
      >
        <IconTrash className="size-3.5" aria-hidden="true" />
      </Button>
    </div>
  );
}
