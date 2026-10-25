"use client";

import { useEffect, useId, useMemo, useRef, useState, type ReactNode } from "react";
import {
  DndContext,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
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
import { sortKeyLabelKey } from "./sort-picker";
import { SortChainRuleCard } from "./sort-chain-rule-card";
import { createSidebarListCollisionDetection } from "./sidebar-reorder-collision";
import {
  changeIdentifiedSortRule,
  moveIdentifiedSortRuleById,
  removeIdentifiedSortRule,
  type IdentifiedSortRule,
} from "./sort-chain-editor-model";
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

function useSortChainEditor(value: SortSpec, onChange: (sort: SortSpec) => void) {
  const { t } = useTranslation();
  const editorId = useId();
  const identitySequence = useRef(0);
  const createEntries = (rules: SortRule[]): IdentifiedSortRule[] =>
    rules.map((rule) => ({ id: `${editorId}${identitySequence.current++}`, rule }));
  const inputRules = useMemo(() => sidebarSortRules(value), [value]);
  const inputSignature = JSON.stringify(inputRules);
  const [entries, setEntries] = useState(() => createEntries(inputRules));
  const [announcement, setAnnouncement] = useState("");
  const observedSignature = useRef(inputSignature);
  const pendingSignature = useRef<{ previous: string; next: string } | null>(null);

  useEffect(() => {
    const pending = pendingSignature.current;
    if (pending?.next === inputSignature) {
      pendingSignature.current = null;
      observedSignature.current = inputSignature;
      return;
    }
    if (pending?.previous === inputSignature) return;
    if (observedSignature.current === inputSignature) return;
    pendingSignature.current = null;
    observedSignature.current = inputSignature;
    setEntries(createEntries(inputRules));
  }, [inputRules, inputSignature]);

  const rules = entries.map((entry) => entry.rule);
  const canAdd = rules.length < MAX_SIDEBAR_SORT_RULES && value.key !== "custom";
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const commitEntries = (updated: IdentifiedSortRule[]) => {
    const nextSort = withRules(updated.map((entry) => entry.rule));
    pendingSignature.current = {
      previous: observedSignature.current,
      next: JSON.stringify(sidebarSortRules(nextSort)),
    };
    setEntries(updated);
    onChange(nextSort);
  };
  const changeRule = (index: number, rule: SortRule) => {
    commitEntries(changeIdentifiedSortRule(entries, index, rule));
  };
  const labelFor = (id: string) => {
    const entry = entries.find((candidate) => candidate.id === id);
    if (!entry) return "";
    const label = t(sortKeyLabelKey(entry.rule.key));
    if (entry.rule.key !== "color") return label;
    const color = entry.rule.color ?? "red";
    return `${label} ${t(`task:color${color[0]!.toUpperCase()}${color.slice(1)}`)}`;
  };
  const moveRule = (id: string, offset: -1 | 1) => {
    const index = entries.findIndex((entry) => entry.id === id);
    const destination = index + offset;
    if (index < 0 || destination < 0 || destination >= entries.length) return;
    const next = moveIdentifiedSortRuleById(entries, id, entries[destination]!.id);
    if (next === entries) return;
    commitEntries(next);
    setAnnouncement(
      t("task:sidebarReorderMoved", {
        label: labelFor(id),
        position: destination + 1,
        count: entries.length,
      }),
    );
  };
  const handleDragEnd = (event: DragEndEvent) => {
    if (!event.over) return;
    const next = moveIdentifiedSortRuleById(
      entries,
      String(event.active.id),
      String(event.over.id),
    );
    if (next !== entries) commitEntries(next);
  };
  const addRule = () => {
    const rule = newRule(rules);
    if (rule) commitEntries([...entries, ...createEntries([rule])]);
  };

  return {
    t,
    entries,
    rules,
    canAdd,
    sensors,
    announcement,
    changeRule,
    labelFor,
    moveRule,
    handleDragEnd,
    addRule,
    commitEntries,
  };
}

function SortChainDndContext({
  reorderScopeKey,
  entries,
  sensors,
  labelFor,
  onDragEnd,
  children,
}: {
  reorderScopeKey: string;
  entries: IdentifiedSortRule[];
  sensors: ReturnType<typeof useSensors>;
  labelFor: (id: string) => string;
  onDragEnd: (event: DragEndEvent) => void;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const collision = useMemo(createSidebarListCollisionDetection, []);
  return (
    <DndContext
      key={reorderScopeKey}
      sensors={sensors}
      collisionDetection={collision.detect}
      onDragStart={collision.reset}
      onDragEnd={(event) => {
        const accepted = collision.isDropWithinCurrentVisibleBounds();
        const dropEvent = accepted ? event : { ...event, over: null };
        onDragEnd(dropEvent);
      }}
      onDragCancel={collision.reset}
      accessibility={{
        screenReaderInstructions: { draggable: t("task:sidebarReorderInstructions") },
        announcements: {
          onDragStart: ({ active }) => {
            const position = entries.findIndex((entry) => entry.id === String(active.id)) + 1;
            return t("task:sidebarReorderPickedUp", {
              label: labelFor(String(active.id)),
              position,
              count: entries.length,
            });
          },
          onDragOver: ({ active, over }) => {
            if (!over) return;
            return t("task:sidebarReorderMoved", {
              label: labelFor(String(active.id)),
              position: entries.findIndex((entry) => entry.id === String(over.id)) + 1,
              count: entries.length,
            });
          },
          onDragEnd: ({ active, over }) => {
            if (!over || !collision.isDropWithinCurrentVisibleBounds()) {
              return t("task:sidebarReorderCancelled", { label: labelFor(String(active.id)) });
            }
            return t("task:sidebarReorderDropped", {
              label: labelFor(String(active.id)),
              position: entries.findIndex((entry) => entry.id === String(over.id)) + 1,
            });
          },
          onDragCancel: ({ active }) =>
            t("task:sidebarReorderCancelled", { label: labelFor(String(active.id)) }),
        },
      }}
    >
      {children}
    </DndContext>
  );
}

export function SortChainEditor({
  value,
  onChange,
  isDrawerLayout,
  reorderScopeKey = "default",
  warningCount = 0,
}: {
  value: SortSpec;
  onChange: (sort: SortSpec) => void;
  isDrawerLayout: boolean;
  reorderScopeKey?: string;
  warningCount?: number;
}) {
  const {
    t,
    entries,
    rules,
    canAdd,
    sensors,
    announcement,
    changeRule,
    labelFor,
    moveRule,
    handleDragEnd,
    addRule,
    commitEntries,
  } = useSortChainEditor(value, onChange);

  return (
    <div className="space-y-1.5" data-testid="sidebar-sort-chain-editor">
      <SortChainDndContext
        reorderScopeKey={reorderScopeKey}
        entries={entries}
        sensors={sensors}
        labelFor={labelFor}
        onDragEnd={handleDragEnd}
      >
        <SortableContext
          items={entries.map((entry) => entry.id)}
          strategy={verticalListSortingStrategy}
        >
          <div data-sidebar-reorder-list="">
            {entries.map(({ id, rule }, index) => (
              <SortChainRuleCard
                key={id}
                id={id}
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
                onMove={(offset) => moveRule(id, offset)}
                onRemove={() => commitEntries(removeIdentifiedSortRule(entries, index))}
              />
            ))}
          </div>
        </SortableContext>
      </SortChainDndContext>
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
      <div aria-live="polite" className="sr-only">
        {announcement}
      </div>
    </div>
  );
}
