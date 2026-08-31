"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import type {
  SidebarTaskColorDimension,
  SidebarTaskColorRule,
  FixedAutomaticTaskColor,
} from "@/lib/task-color-automation-settings";
import {
  FIXED_AUTOMATIC_TASK_COLORS,
  AUTOMATIC_TASK_COLOR_DIMENSIONS,
} from "@/lib/task-color-automation-settings";
import {
  fixedAutomaticTaskColor,
  type TaskMarkerPresentation,
} from "@/lib/task-color-presentation";
import { cn } from "@/lib/utils";
import type { RepositoryRuleCatalogOption } from "@/lib/sidebar/repository-rule-catalog";
import { RepositoryConditionField, type Translate } from "./automatic-color-repository-picker";
import { taskColorDimensionLabelKey, type TaskColorRuleOption } from "./task-color-rule-options";

type AutomaticColorLabelKey = Record<FixedAutomaticTaskColor, string>;

export const AUTOMATIC_COLOR_LABEL_KEYS: AutomaticColorLabelKey = {
  gray: "task:colorGray",
  red: "task:colorRed",
  orange: "task:colorOrange",
  yellow: "task:colorYellow",
  green: "task:colorGreen",
  cyan: "task:colorCyan",
  blue: "task:colorBlue",
  indigo: "task:colorIndigo",
  purple: "task:colorPurple",
  pink: "task:colorPink",
};

export function AutomaticColorConditionFields({
  rule,
  options,
  repositoryOptions,
  selectedOption,
  isDrawerLayout,
  repositoryQuery,
  repositoryLoading,
  repositoryError,
  onRepositoryQueryChange,
  onRefreshRepositories,
  onOpenRepository,
  onChange,
  onConditionChange,
  onTargetChange,
  t,
}: {
  rule: SidebarTaskColorRule;
  options: readonly TaskColorRuleOption[];
  repositoryOptions: readonly RepositoryRuleCatalogOption[];
  selectedOption: TaskColorRuleOption | undefined;
  isDrawerLayout: boolean;
  repositoryQuery: string;
  repositoryLoading: boolean;
  repositoryError: Error | null;
  onRepositoryQueryChange: (query: string) => void;
  onRefreshRepositories: () => void;
  onOpenRepository: () => void;
  onChange: (rule: SidebarTaskColorRule) => void;
  onConditionChange: (dimension: SidebarTaskColorDimension) => void;
  onTargetChange: (key: string) => void;
  t: Translate;
}) {
  return (
    <div className="grid min-w-0 gap-2 md:grid-cols-2">
      <AutomaticColorDimensionField
        ruleId={rule.id}
        dimension={rule.condition.dimension}
        onChange={onConditionChange}
        t={t}
      />
      {rule.condition.dimension === "repository" ? (
        <RepositoryConditionField
          rule={rule}
          selectedOption={selectedOption}
          isDrawerLayout={isDrawerLayout}
          options={repositoryOptions}
          query={repositoryQuery}
          loading={repositoryLoading}
          error={repositoryError}
          onQueryChange={onRepositoryQueryChange}
          onRefresh={onRefreshRepositories}
          onOpen={onOpenRepository}
          onSelect={(option) =>
            onChange({
              ...rule,
              condition: { ...rule.condition, value: option.target, label: option.label },
            })
          }
          t={t}
        />
      ) : (
        <AutomaticColorTargetField
          ruleId={rule.id}
          options={options}
          selectedOption={selectedOption}
          onChange={onTargetChange}
          t={t}
        />
      )}
    </div>
  );
}

function AutomaticColorDimensionField({
  ruleId,
  dimension,
  onChange,
  t,
}: {
  ruleId: string;
  dimension: SidebarTaskColorDimension;
  onChange: (dimension: SidebarTaskColorDimension) => void;
  t: Translate;
}) {
  return (
    <label className="min-w-0">
      <span className="mb-1 block text-[11px] font-medium text-muted-foreground">
        {t("task:automaticColorsCondition")}
      </span>
      <Select
        value={dimension}
        onValueChange={(value) => onChange(value as SidebarTaskColorDimension)}
      >
        <SelectTrigger
          size="sm"
          className="min-h-11 w-full text-xs md:min-h-0 md:h-7"
          data-testid={`automatic-color-dimension-${ruleId}`}
          aria-label={t("task:automaticColorsCondition")}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {AUTOMATIC_TASK_COLOR_DIMENSIONS.map((option) => (
            <SelectItem key={option} value={option} className="text-xs">
              {t(taskColorDimensionLabelKey(option))}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </label>
  );
}

function AutomaticColorTargetField({
  ruleId,
  options,
  selectedOption,
  onChange,
  t,
}: {
  ruleId: string;
  options: readonly TaskColorRuleOption[];
  selectedOption: TaskColorRuleOption | undefined;
  onChange: (key: string) => void;
  t: Translate;
}) {
  return (
    <label className="min-w-0">
      <span className="mb-1 block text-[11px] font-medium text-muted-foreground">
        {t("task:automaticColorsSelectTarget")}
      </span>
      <Select value={selectedOption?.key} onValueChange={onChange}>
        <SelectTrigger
          size="sm"
          className="min-h-11 w-full text-xs md:min-h-0 md:h-7"
          data-testid={`automatic-color-target-${ruleId}`}
          aria-label={t("task:automaticColorsSelectTarget")}
        >
          <SelectValue placeholder={t("task:automaticColorsSelectTarget")} />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem
              key={option.key}
              value={option.key}
              disabled={!option.available}
              className="text-xs"
            >
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </label>
  );
}

export function AutomaticColorOutputField({
  rule,
  onChange,
  t,
}: {
  rule: SidebarTaskColorRule;
  onChange: (rule: SidebarTaskColorRule) => void;
  t: Translate;
}) {
  const marker = outputMarker(rule);
  return (
    <label className="block min-w-0">
      <span className="mb-1 block text-[11px] font-medium text-muted-foreground">
        {t("task:automaticColorsOutput")}
      </span>
      <Select
        value={rule.output.kind === "workflow_step" ? "workflow_step" : rule.output.color}
        onValueChange={(output) => {
          if (output === "workflow_step") {
            if (rule.condition.dimension === "workflow_step") {
              onChange({ ...rule, output: { kind: "workflow_step" } });
            }
            return;
          }
          const color = output as FixedAutomaticTaskColor;
          if (FIXED_AUTOMATIC_TASK_COLORS.includes(color)) {
            onChange({ ...rule, output: { kind: "fixed", color } });
          }
        }}
      >
        <SelectTrigger
          size="sm"
          className="min-h-11 w-full text-xs md:min-h-0 md:h-7"
          data-testid={`automatic-color-output-${rule.id}`}
          aria-label={t("task:automaticColorsOutput")}
        >
          <span className="flex min-w-0 items-center gap-2">
            <ColorSwatch marker={marker} />
            <SelectValue />
          </span>
        </SelectTrigger>
        <SelectContent>
          {rule.condition.dimension === "workflow_step" && (
            <SelectItem value="workflow_step" className="text-xs">
              {t("task:automaticColorsUseStepColor")}
            </SelectItem>
          )}
          {FIXED_AUTOMATIC_TASK_COLORS.map((color) => (
            <SelectItem key={color} value={color} className="text-xs">
              <span className="flex items-center gap-2">
                <ColorSwatch marker={fixedAutomaticTaskColor(color)} />
                {t(AUTOMATIC_COLOR_LABEL_KEYS[color])}
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </label>
  );
}

export function AutomaticColorEnableField({
  rule,
  complete,
  onChange,
  t,
}: {
  rule: SidebarTaskColorRule;
  complete: boolean;
  onChange: (enabled: boolean) => void;
  t: Translate;
}) {
  return (
    <div className="flex min-h-11 items-center justify-between gap-2 rounded-md bg-muted/30 px-2">
      <div className="min-w-0">
        <span className="block text-xs font-medium">{t("task:automaticColorsEnable")}</span>
        {!complete && (
          <span className="block text-[11px] text-muted-foreground">
            {t("task:automaticColorsCompleteRule")}
          </span>
        )}
      </div>
      <label className="flex size-11 shrink-0 cursor-pointer items-center justify-center">
        <Switch
          checked={rule.enabled}
          disabled={!complete}
          onCheckedChange={onChange}
          aria-label={t("task:automaticColorsEnable")}
          data-testid={`automatic-color-rule-enabled-${rule.id}`}
        />
      </label>
    </div>
  );
}

export function optionWithCurrentValue(
  options: readonly TaskColorRuleOption[],
  rule: SidebarTaskColorRule,
  t: Translate,
): TaskColorRuleOption[] {
  if (rule.condition.value === null || rule.condition.value === undefined) return [...options];
  const key = JSON.stringify(rule.condition.value);
  if (options.some((option) => option.key === key)) return [...options];
  return [
    ...options,
    {
      key,
      value: rule.condition.value,
      label: rule.condition.label || t("task:automaticColorsUnavailable"),
      available: false,
    },
  ];
}

export function isCompleteRule(
  rule: SidebarTaskColorRule,
  selectedOption: TaskColorRuleOption | undefined,
): boolean {
  if (rule.condition.value === null || rule.condition.value === undefined) return false;
  if (!selectedOption?.available) return false;
  return rule.output.kind === "workflow_step"
    ? rule.condition.dimension === "workflow_step"
    : FIXED_AUTOMATIC_TASK_COLORS.includes(rule.output.color);
}

function outputMarker(rule: SidebarTaskColorRule): TaskMarkerPresentation {
  return rule.output.kind === "workflow_step"
    ? fixedAutomaticTaskColor("gray")
    : fixedAutomaticTaskColor(rule.output.color);
}

function ColorSwatch({ marker }: { marker: TaskMarkerPresentation }) {
  return (
    <span
      className={cn(
        "inline-block size-2.5 shrink-0 rounded-full",
        marker.token === "custom" ? undefined : marker.className,
      )}
      style={marker.token === "custom" ? marker.style : undefined}
      aria-hidden="true"
    />
  );
}
