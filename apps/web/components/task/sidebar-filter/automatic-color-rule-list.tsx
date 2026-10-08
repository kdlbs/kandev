"use client";

import { useMemo, useState, type ReactNode } from "react";
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
import type { SidebarTaskColorRule } from "@/lib/task-color-automation-settings";
import type { RepositoryRuleCatalogOption } from "@/lib/sidebar/repository-rule-catalog";
import type { TaskColorRuleOptionMap } from "./task-color-rule-options";
import { taskColorDimensionLabelKey } from "./task-color-rule-options";
import { AutomaticColorRuleCard } from "./automatic-color-rule-card";
import { createSidebarListCollisionDetection } from "./sidebar-reorder-collision";
import type { Translate } from "./automatic-color-repository-picker";

function labelForAutomaticColorRule(rules: SidebarTaskColorRule[], id: string, t: Translate) {
  const index = rules.findIndex((rule) => rule.id === id);
  const rule = rules[index];
  if (!rule) return "";
  return `${t("task:automaticColorsRule", { number: index + 1 })} ${t(taskColorDimensionLabelKey(rule.condition.dimension))}`;
}

function AutomaticColorRuleRows({
  rules,
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
}: Pick<
  Parameters<typeof AutomaticColorRuleList>[0],
  | "rules"
  | "scalarOptions"
  | "repositoryOptions"
  | "repositoryQuery"
  | "repositoryLoading"
  | "repositoryError"
  | "onRepositoryQueryChange"
  | "onRefreshRepositories"
  | "isDrawerLayout"
  | "onChange"
  | "onRemove"
  | "onOpenRepository"
  | "t"
> & { onMove: (ruleId: string, direction: -1 | 1) => void }) {
  return (
    <SortableContext items={rules.map((rule) => rule.id)} strategy={verticalListSortingStrategy}>
      <div
        className="space-y-2"
        data-testid="automatic-color-rule-list"
        data-sidebar-reorder-list=""
      >
        {rules.map((rule, index) => (
          <AutomaticColorRuleCard
            key={rule.id}
            rule={rule}
            index={index}
            total={rules.length}
            scalarOptions={scalarOptions}
            repositoryOptions={repositoryOptions}
            repositoryQuery={repositoryQuery}
            repositoryLoading={repositoryLoading}
            repositoryError={repositoryError}
            onRepositoryQueryChange={onRepositoryQueryChange}
            onRefreshRepositories={onRefreshRepositories}
            isDrawerLayout={isDrawerLayout}
            onChange={(next) => onChange(rule.id, next)}
            onRemove={() => onRemove(rule.id)}
            onOpenRepository={() => onOpenRepository(rule.id)}
            onMove={(direction) => onMove(rule.id, direction)}
            t={t}
          />
        ))}
      </div>
    </SortableContext>
  );
}

function AutomaticColorRuleDndContext({
  scopeKey,
  rules,
  sensors,
  t,
  onDragStart,
  onDragEnd,
  children,
}: {
  scopeKey: string;
  rules: SidebarTaskColorRule[];
  sensors: ReturnType<typeof useSensors>;
  t: Translate;
  onDragStart: () => void;
  onDragEnd: (event: DragEndEvent) => void;
  children: ReactNode;
}) {
  const collision = useMemo(createSidebarListCollisionDetection, []);
  return (
    <DndContext
      key={scopeKey}
      sensors={sensors}
      collisionDetection={collision.detect}
      onDragStart={() => {
        collision.reset();
        onDragStart();
      }}
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
            const position = rules.findIndex((rule) => rule.id === String(active.id)) + 1;
            return t("task:sidebarReorderPickedUp", {
              label: labelForAutomaticColorRule(rules, String(active.id), t),
              position,
              count: rules.length,
            });
          },
          onDragOver: ({ active, over }) => {
            if (!over) return;
            return t("task:sidebarReorderMoved", {
              label: labelForAutomaticColorRule(rules, String(active.id), t),
              position: rules.findIndex((rule) => rule.id === String(over.id)) + 1,
              count: rules.length,
            });
          },
          onDragEnd: ({ active, over }) => {
            if (!over || !collision.isDropWithinCurrentVisibleBounds()) {
              return t("task:sidebarReorderCancelled", {
                label: labelForAutomaticColorRule(rules, String(active.id), t),
              });
            }
            return t("task:sidebarReorderDropped", {
              label: labelForAutomaticColorRule(rules, String(active.id), t),
              position: rules.findIndex((rule) => rule.id === String(over.id)) + 1,
            });
          },
          onDragCancel: ({ active }) =>
            t("task:sidebarReorderCancelled", {
              label: labelForAutomaticColorRule(rules, String(active.id), t),
            }),
        },
      }}
    >
      {children}
    </DndContext>
  );
}

export function AutomaticColorRuleList({
  rules,
  scopeKey,
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
  onReorder,
  t,
}: {
  rules: SidebarTaskColorRule[];
  scopeKey: string;
  scalarOptions: TaskColorRuleOptionMap;
  repositoryOptions: readonly RepositoryRuleCatalogOption[];
  repositoryQuery: string;
  repositoryLoading: boolean;
  repositoryError: Error | null;
  onRepositoryQueryChange: (query: string) => void;
  onRefreshRepositories: () => void;
  isDrawerLayout: boolean;
  onChange: (ruleId: string, rule: SidebarTaskColorRule) => void;
  onRemove: (ruleId: string) => void;
  onOpenRepository: (ruleId: string) => void;
  onReorder: (activeId: string, overId: string) => void;
  t: Translate;
}) {
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const [announcement, setAnnouncement] = useState("");

  function moveRule(ruleId: string, offset: -1 | 1) {
    const index = rules.findIndex((rule) => rule.id === ruleId);
    const destination = index + offset;
    const target = rules[destination];
    if (index < 0 || !target) return;
    onReorder(ruleId, target.id);
    setAnnouncement(
      t("task:sidebarReorderMoved", {
        label: labelForAutomaticColorRule(rules, ruleId, t),
        position: destination + 1,
        count: rules.length,
      }),
    );
  }

  function handleDragEnd(event: DragEndEvent) {
    if (!event.over) return;
    onReorder(String(event.active.id), String(event.over.id));
  }

  return (
    <div>
      <AutomaticColorRuleDndContext
        scopeKey={scopeKey}
        rules={rules}
        sensors={sensors}
        t={t}
        onDragStart={() => setAnnouncement("")}
        onDragEnd={handleDragEnd}
      >
        <AutomaticColorRuleRows
          rules={rules}
          scalarOptions={scalarOptions}
          repositoryOptions={repositoryOptions}
          repositoryQuery={repositoryQuery}
          repositoryLoading={repositoryLoading}
          repositoryError={repositoryError}
          onRepositoryQueryChange={onRepositoryQueryChange}
          onRefreshRepositories={onRefreshRepositories}
          isDrawerLayout={isDrawerLayout}
          onChange={onChange}
          onRemove={onRemove}
          onOpenRepository={onOpenRepository}
          onMove={moveRule}
          t={t}
        />
      </AutomaticColorRuleDndContext>
      <div aria-live="polite" className="sr-only">
        {announcement}
      </div>
    </div>
  );
}
