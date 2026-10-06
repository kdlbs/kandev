"use client";

import { IconArrowDown, IconArrowUp, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useTranslation } from "react-i18next";
import { type FixedAutomaticTaskColor } from "@/lib/task-color-automation-settings";
import type { SortDirection, SortKey, SortRule } from "@/lib/state/slices/ui/sidebar-view-types";
import { taskColorPresentation } from "@/lib/task-color-presentation";
import { sortKeyLabelKey } from "./sort-picker";

export function sortRuleDirectionLabelKey(key: SortKey, direction: SortDirection): string {
  if (key === "running")
    return direction === "desc" ? "task:sortRunningFirst" : "task:sortOthersFirst";
  if (key === "color")
    return direction === "desc" ? "task:sortMatchingFirst" : "task:sortOthersFirst";
  if (["updatedAt", "lastActivityAt", "createdAt"].includes(key))
    return direction === "desc" ? "task:sortNewestFirst" : "task:sortOldestFirst";
  return direction === "desc" ? "task:sortDescending" : "task:sortAscending";
}

function controlHeight(isDrawerLayout: boolean): string {
  return isDrawerLayout ? "min-h-11" : "h-7 min-h-7 [@media(pointer:coarse)]:min-h-11";
}

function SortRuleFieldSelect({
  rule,
  availableKeys,
  position,
  isDrawerLayout,
  onChange,
}: {
  rule: SortRule;
  availableKeys: SortKey[];
  position: number;
  isDrawerLayout: boolean;
  onChange: (key: SortKey) => void;
}) {
  const { t } = useTranslation();
  return (
    <Select value={rule.key} onValueChange={(key) => onChange(key as SortKey)}>
      <SelectTrigger
        className={`min-w-0 flex-1 text-xs ${controlHeight(isDrawerLayout)}`}
        aria-label={t("task:sortRuleField", { position })}
        data-testid={position === 1 ? "sort-key-select" : `sort-rule-key-${position - 1}`}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {availableKeys.map((key) => (
          <SelectItem key={key} value={key} className="text-xs">
            {t(sortKeyLabelKey(key))}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function SortRuleColorSelect({
  color,
  availableColors,
  position,
  isDrawerLayout,
  onChange,
}: {
  color: FixedAutomaticTaskColor;
  availableColors: FixedAutomaticTaskColor[];
  position: number;
  isDrawerLayout: boolean;
  onChange: (color: FixedAutomaticTaskColor) => void;
}) {
  const { t } = useTranslation();
  return (
    <Select value={color} onValueChange={(next) => onChange(next as FixedAutomaticTaskColor)}>
      <SelectTrigger
        className={`min-w-0 flex-1 text-xs ${controlHeight(isDrawerLayout)}`}
        aria-label={t("task:sortRuleColor", { position })}
        data-testid={`sort-rule-color-${position - 1}`}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {availableColors.map((candidate) => {
          const presentation = taskColorPresentation(candidate);
          return (
            <SelectItem key={candidate} value={candidate} className="text-xs">
              <span
                className={`mr-2 inline-block size-2.5 rounded-full ${presentation.token === "custom" ? "" : presentation.className}`}
                aria-hidden="true"
              />
              {t(`task:color${candidate[0].toUpperCase()}${candidate.slice(1)}`)}
            </SelectItem>
          );
        })}
      </SelectContent>
    </Select>
  );
}

function SortRuleDirectionSelect({
  rule,
  position,
  isDrawerLayout,
  onChange,
}: {
  rule: Exclude<SortRule, { key: "custom" }>;
  position: number;
  isDrawerLayout: boolean;
  onChange: (direction: SortDirection) => void;
}) {
  const { t } = useTranslation();
  return (
    <Select value={rule.direction} onValueChange={(next) => onChange(next as SortDirection)}>
      <SelectTrigger
        className={`min-w-0 flex-1 text-xs ${controlHeight(isDrawerLayout)}`}
        aria-label={t("task:sortRuleOrder", { position })}
        data-testid={`sort-rule-direction-${position - 1}`}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {(["asc", "desc"] as const).map((direction) => (
          <SelectItem key={direction} value={direction} className="text-xs">
            {t(sortRuleDirectionLabelKey(rule.key, direction))}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function SortRuleActions({
  position,
  ruleCount,
  isDrawerLayout,
  onMove,
  onRemove,
}: {
  position: number;
  ruleCount: number;
  isDrawerLayout: boolean;
  onMove: (offset: number) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const touchClass = controlHeight(isDrawerLayout);
  const buttonClass = `${touchClass} w-9 cursor-pointer [@media(pointer:coarse)]:w-11`;
  return (
    <div className={`flex shrink-0 ${isDrawerLayout ? "justify-end" : ""}`}>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className={buttonClass}
        aria-label={t("task:sortMoveUp", { position })}
        disabled={position === 1}
        onClick={() => onMove(-1)}
        data-testid={`sort-rule-up-${position - 1}`}
      >
        <IconArrowUp className="size-4" aria-hidden="true" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className={buttonClass}
        aria-label={t("task:sortMoveDown", { position })}
        disabled={position === ruleCount}
        onClick={() => onMove(1)}
        data-testid={`sort-rule-down-${position - 1}`}
      >
        <IconArrowDown className="size-4" aria-hidden="true" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className={`${touchClass} w-9 cursor-pointer text-muted-foreground hover:text-destructive [@media(pointer:coarse)]:w-11`}
        aria-label={t("task:sortRemoveRule", { position })}
        disabled={ruleCount === 1}
        onClick={onRemove}
        data-testid={`sort-rule-remove-${position - 1}`}
      >
        <IconX className="size-4" aria-hidden="true" />
      </Button>
    </div>
  );
}

function SortRuleDescription({ rule, position }: { rule: SortRule; position: number }) {
  const { t } = useTranslation();
  const label = t(sortKeyLabelKey(rule.key));
  const direction =
    rule.key === "custom"
      ? t("task:sortCustomDescription")
      : t(sortRuleDirectionLabelKey(rule.key, rule.direction));
  const description =
    rule.key === "color"
      ? `${label}: ${t(`task:color${(rule.color ?? "red")[0].toUpperCase()}${(rule.color ?? "red").slice(1)}`)}, ${direction}`
      : `${label}, ${direction}`;
  return (
    <span className="sr-only" data-testid={`sort-rule-description-${position - 1}`}>
      {description}
    </span>
  );
}

export function SortChainRuleCard({
  rule,
  position,
  ruleCount,
  availableKeys,
  availableColors,
  isDrawerLayout,
  onFieldChange,
  onColorChange,
  onDirectionChange,
  onMove,
  onRemove,
}: {
  rule: SortRule;
  position: number;
  ruleCount: number;
  availableKeys: SortKey[];
  availableColors: FixedAutomaticTaskColor[];
  isDrawerLayout: boolean;
  onFieldChange: (key: SortKey) => void;
  onColorChange: (color: FixedAutomaticTaskColor) => void;
  onDirectionChange: (direction: SortDirection) => void;
  onMove: (offset: number) => void;
  onRemove: () => void;
}) {
  const layoutClass = isDrawerLayout ? "flex-col" : "items-center";
  return (
    <div
      className={`flex gap-1 rounded-md border border-border/50 p-1 ${layoutClass}`}
      data-testid={`sort-rule-card-${position - 1}`}
    >
      <span className="flex min-w-5 items-center justify-center text-[11px] text-muted-foreground">
        {position}
      </span>
      <SortRuleFieldSelect
        rule={rule}
        availableKeys={availableKeys}
        position={position}
        isDrawerLayout={isDrawerLayout}
        onChange={onFieldChange}
      />
      {rule.key === "color" && (
        <SortRuleColorSelect
          color={rule.color ?? "red"}
          availableColors={availableColors}
          position={position}
          isDrawerLayout={isDrawerLayout}
          onChange={onColorChange}
        />
      )}
      {rule.key !== "custom" && (
        <SortRuleDirectionSelect
          rule={rule}
          position={position}
          isDrawerLayout={isDrawerLayout}
          onChange={onDirectionChange}
        />
      )}
      <SortRuleActions
        position={position}
        ruleCount={ruleCount}
        isDrawerLayout={isDrawerLayout}
        onMove={onMove}
        onRemove={onRemove}
      />
      <SortRuleDescription rule={rule} position={position} />
    </div>
  );
}
