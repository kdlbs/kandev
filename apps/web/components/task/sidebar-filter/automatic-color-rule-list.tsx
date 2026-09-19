"use client";

import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  TouchSensor,
  closestCenter,
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
import { AutomaticColorRuleCard } from "./automatic-color-rule-card";
import type { Translate } from "./automatic-color-repository-picker";

export function AutomaticColorRuleList({
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
  onReorder,
  t,
}: {
  rules: SidebarTaskColorRule[];
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
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  function handleDragEnd(event: DragEndEvent) {
    if (event.over) onReorder(String(event.active.id), String(event.over.id));
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={rules.map((rule) => rule.id)} strategy={verticalListSortingStrategy}>
        <div className="space-y-2" data-testid="automatic-color-rule-list">
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
              onMove={(direction) => {
                const destination = index + direction;
                if (destination >= 0 && destination < rules.length) {
                  onReorder(rule.id, rules[destination]!.id);
                }
              }}
              t={t}
            />
          ))}
        </div>
      </SortableContext>
    </DndContext>
  );
}
