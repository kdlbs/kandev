"use client";

import { IconGripVertical, IconX } from "@tabler/icons-react";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useTranslation } from "react-i18next";
import { type FixedAutomaticTaskColor } from "@/lib/task-color-automation-settings";
import type { SortDirection, SortKey, SortRule } from "@/lib/state/slices/ui/sidebar-view-types";
import { taskColorPresentation } from "@/lib/task-color-presentation";
import { sortKeyDescriptionKey, sortKeyLabelKey } from "./sort-picker";
import { SidebarReorderMenu } from "./sidebar-reorder-menu";

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
        {availableKeys.map((key) => {
          const descriptionKey = sortKeyDescriptionKey(key);
          return (
            <SelectItem
              key={key}
              value={key}
              className="text-xs"
              description={descriptionKey ? t(descriptionKey) : undefined}
            >
              {t(sortKeyLabelKey(key))}
            </SelectItem>
          );
        })}
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
        data-direction={rule.direction}
        data-testid={`sort-rule-direction-${position - 1}`}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {(["asc", "desc"] as const).map((direction) => (
          <SelectItem
            key={direction}
            value={direction}
            className="text-xs"
            data-testid={`sort-rule-direction-option-${position - 1}-${direction}`}
          >
            {t(sortRuleDirectionLabelKey(rule.key, direction))}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function SortRuleActions({
  label,
  position,
  ruleCount,
  isDrawerLayout,
  onMove,
  onRemove,
}: {
  label: string;
  position: number;
  ruleCount: number;
  isDrawerLayout: boolean;
  onMove: (offset: -1 | 1) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const buttonClass = isDrawerLayout
    ? "size-11"
    : "size-7 max-md:size-11 [@media(pointer:coarse)]:size-11";
  return (
    <div className={`flex shrink-0 items-center ${isDrawerLayout ? "ml-auto" : ""}`}>
      <SidebarReorderMenu
        label={label}
        position={position}
        count={ruleCount}
        onMove={onMove}
        isDrawerLayout={isDrawerLayout}
        testId={`sort-rule-more-${position - 1}`}
        moveUpLabelKey="task:sortMoveUp"
        moveDownLabelKey="task:sortMoveDown"
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className={`${buttonClass} cursor-pointer text-muted-foreground hover:text-destructive`}
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

function SortRuleFields({
  rule,
  availableKeys,
  availableColors,
  position,
  isDrawerLayout,
  onFieldChange,
  onColorChange,
  onDirectionChange,
}: {
  rule: SortRule;
  availableKeys: SortKey[];
  availableColors: FixedAutomaticTaskColor[];
  position: number;
  isDrawerLayout: boolean;
  onFieldChange: (key: SortKey) => void;
  onColorChange: (color: FixedAutomaticTaskColor) => void;
  onDirectionChange: (direction: SortDirection) => void;
}) {
  return (
    <div className={`flex min-w-0 gap-1 ${isDrawerLayout ? "flex-col" : "flex-1 items-center"}`}>
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
    </div>
  );
}

export function SortChainRuleCard({
  id,
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
  id: string;
  rule: SortRule;
  position: number;
  ruleCount: number;
  availableKeys: SortKey[];
  availableColors: FixedAutomaticTaskColor[];
  isDrawerLayout: boolean;
  onFieldChange: (key: SortKey) => void;
  onColorChange: (color: FixedAutomaticTaskColor) => void;
  onDirectionChange: (direction: SortDirection) => void;
  onMove: (offset: -1 | 1) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const sortable = useSortable({ id, disabled: ruleCount <= 1 });
  const layoutClass = isDrawerLayout ? "flex-col" : "items-center";
  const label = t(sortKeyLabelKey(rule.key));
  const itemLabel =
    rule.key === "color"
      ? `${label} ${t(`task:color${(rule.color ?? "red")[0]!.toUpperCase()}${(rule.color ?? "red").slice(1)}`)}`
      : label;
  const handleClass = isDrawerLayout
    ? "size-11"
    : "size-7 max-md:size-11 [@media(pointer:coarse)]:size-11";
  const handle = (
    <button
      ref={sortable.setActivatorNodeRef}
      type="button"
      className={`flex shrink-0 cursor-grab touch-none items-center justify-center text-muted-foreground/60 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed ${handleClass}`}
      aria-label={t("task:sortRuleReorderHandle", { position })}
      disabled={ruleCount <= 1}
      data-testid={`sort-rule-handle-${position - 1}`}
      data-vaul-no-drag={isDrawerLayout ? "" : undefined}
      {...sortable.attributes}
      {...sortable.listeners}
      aria-roledescription={t("task:sortRuleReorderable")}
    >
      <IconGripVertical className="size-4" aria-hidden="true" />
    </button>
  );
  const actions = (
    <SortRuleActions
      label={itemLabel}
      position={position}
      ruleCount={ruleCount}
      isDrawerLayout={isDrawerLayout}
      onMove={onMove}
      onRemove={onRemove}
    />
  );
  return (
    <div
      ref={sortable.setNodeRef}
      style={{
        transform: CSS.Transform.toString(sortable.transform),
        transition: sortable.transition,
      }}
      className={`flex min-w-0 gap-1 rounded-md border border-border/50 p-1 ${layoutClass}`}
      data-testid={`sort-rule-card-${position - 1}`}
      data-dragging={sortable.isDragging ? "true" : undefined}
    >
      {isDrawerLayout && (
        <div className="flex min-w-0 items-center gap-1">
          {handle}
          <span className="min-w-0 flex-1 truncate text-xs font-medium">
            {t("task:sortRuleField", { position })}
          </span>
          {actions}
        </div>
      )}
      {!isDrawerLayout && handle}
      <SortRuleFields
        rule={rule}
        availableKeys={availableKeys}
        availableColors={availableColors}
        position={position}
        isDrawerLayout={isDrawerLayout}
        onFieldChange={onFieldChange}
        onColorChange={onColorChange}
        onDirectionChange={onDirectionChange}
      />
      {!isDrawerLayout && actions}
      <SortRuleDescription rule={rule} position={position} />
    </div>
  );
}
