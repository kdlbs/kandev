"use client";

import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import {
  FIXED_AUTOMATIC_TASK_COLORS,
  type FixedAutomaticTaskColor,
} from "@/lib/task-color-automation-settings";
import type {
  SortDirection,
  SortKey,
  SortRule,
  SortSpec,
} from "@/lib/state/slices/ui/sidebar-view-types";
import { MAX_SIDEBAR_SORT_RULES, sidebarSortRules } from "@/lib/sidebar/sidebar-sort-chain";
import { SortChainRuleCard } from "./sort-chain-rule-card";
export { sortRuleDirectionLabelKey } from "./sort-chain-rule-card";

const EDITABLE_KEYS: SortKey[] = [
  "state",
  "updatedAt",
  "lastActivityAt",
  "createdAt",
  "title",
  "running",
  "color",
  "custom",
];

function defaultDirection(key: SortKey): SortDirection {
  return key === "running" || key === "color" || key === "updatedAt" || key === "lastActivityAt"
    ? "desc"
    : "asc";
}

function withRules(rules: SortRule[]): SortSpec {
  const [primary, ...thenBy] = rules;
  return {
    ...primary,
    ...(thenBy.length
      ? {
          thenBy: thenBy.filter(
            (rule): rule is Exclude<SortRule, { key: "custom" }> => rule.key !== "custom",
          ),
        }
      : {}),
  };
}

function availableKeys(rules: SortRule[], currentIndex: number): SortKey[] {
  return EDITABLE_KEYS.filter(
    (key) =>
      (key === "color" && availableColors(rules, currentIndex).length > 0) ||
      key === rules[currentIndex]?.key ||
      (key !== "custom" &&
        !rules.some((rule, index) => index !== currentIndex && rule.key === key)) ||
      (key === "custom" && rules.length === 1),
  );
}

function availableColors(rules: SortRule[], currentIndex: number): FixedAutomaticTaskColor[] {
  const currentColor = rules[currentIndex]?.key === "color" ? rules[currentIndex].color : undefined;
  return FIXED_AUTOMATIC_TASK_COLORS.filter(
    (color) =>
      color === currentColor ||
      !rules.some(
        (rule, index) => index !== currentIndex && rule.key === "color" && rule.color === color,
      ),
  );
}

function firstAvailableColor(rules: SortRule[], currentIndex = -1): FixedAutomaticTaskColor {
  return (
    FIXED_AUTOMATIC_TASK_COLORS.find(
      (color) =>
        !rules.some(
          (rule, index) => index !== currentIndex && rule.key === "color" && rule.color === color,
        ),
    ) ?? "red"
  );
}

function ruleForKey(current: SortRule, key: SortKey, rules: SortRule[], index: number): SortRule {
  if (key === "custom") return { key: "custom", direction: "asc" };
  if (key === "color") {
    return {
      key: "color",
      color: current.key === key ? (current.color ?? "red") : firstAvailableColor(rules, index),
      direction: current.key === key ? current.direction : "desc",
    };
  }
  return { key, direction: current.key === key ? current.direction : defaultDirection(key) };
}

function newRule(rules: SortRule[]): SortRule | null {
  const key = EDITABLE_KEYS.find(
    (candidate) =>
      candidate !== "custom" &&
      (candidate === "color"
        ? FIXED_AUTOMATIC_TASK_COLORS.some(
            (color) => !rules.some((rule) => rule.key === "color" && rule.color === color),
          )
        : !rules.some((rule) => rule.key === candidate)),
  );
  if (!key) return null;
  return key === "color"
    ? { key, color: firstAvailableColor(rules), direction: "desc" }
    : { key, direction: defaultDirection(key) };
}

export function SortChainEditor({
  value,
  onChange,
  isDrawerLayout,
  warningCount = 0,
}: {
  value: SortSpec;
  onChange: (sort: SortSpec) => void;
  isDrawerLayout: boolean;
  warningCount?: number;
}) {
  const { t } = useTranslation();
  const rules = sidebarSortRules(value);
  const canAdd = rules.length < MAX_SIDEBAR_SORT_RULES && value.key !== "custom";
  const changeRule = (index: number, rule: SortRule) => {
    const updated = [...rules];
    updated[index] = rule;
    onChange(withRules(updated));
  };
  const moveRule = (index: number, offset: number) => {
    const destination = index + offset;
    if (destination < 0 || destination >= rules.length) return;
    const updated = [...rules];
    [updated[index], updated[destination]] = [updated[destination], updated[index]];
    onChange(withRules(updated));
  };
  const addRule = () => {
    const rule = newRule(rules);
    if (rule) onChange(withRules([...rules, rule]));
  };

  return (
    <div className="space-y-1.5" data-testid="sidebar-sort-chain-editor">
      {rules.map((rule, index) => (
        <SortChainRuleCard
          key={`${rule.key}-${rule.key === "color" ? rule.color : ""}`}
          rule={rule}
          position={index + 1}
          ruleCount={rules.length}
          availableKeys={availableKeys(rules, index)}
          availableColors={availableColors(rules, index)}
          isDrawerLayout={isDrawerLayout}
          onFieldChange={(key) => changeRule(index, ruleForKey(rule, key, rules, index))}
          onColorChange={(color) => {
            if (rule.key === "color") changeRule(index, { ...rule, color });
          }}
          onDirectionChange={(direction) =>
            rule.key !== "custom" && changeRule(index, { ...rule, direction })
          }
          onMove={(offset) => moveRule(index, offset)}
          onRemove={() => onChange(withRules(rules.filter((_, current) => current !== index)))}
        />
      ))}
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className={`w-full justify-start cursor-pointer text-xs ${isDrawerLayout ? "min-h-11" : "h-7 [@media(pointer:coarse)]:min-h-11"}`}
        onClick={addRule}
        disabled={!canAdd}
        data-testid="sort-add-rule-button"
      >
        <IconPlus className="mr-1 size-3" aria-hidden="true" />
        {t("task:sortAddRule")}
      </Button>
      {rules.length === MAX_SIDEBAR_SORT_RULES && (
        <p className="px-2 text-[11px] text-muted-foreground">{t("task:sortRuleLimit")}</p>
      )}
      {warningCount > 0 && (
        <p role="status" className="px-2 text-[11px] text-muted-foreground">
          {t("task:sortRulesNormalized")}
        </p>
      )}
    </div>
  );
}
