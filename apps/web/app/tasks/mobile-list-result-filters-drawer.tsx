"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconFilter } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { repositorySlug } from "@/lib/repository-slug";
import type { FilterClause } from "@/lib/state/slices/ui/sidebar-view-types";

type Dimension = "state" | "workflow" | "repository";

const DIMENSIONS: Array<{ key: Dimension; labelKey: string; testId: string }> = [
  { key: "state", labelKey: "task:filterDimensionState", testId: "mobile-list-filter-state" },
  {
    key: "workflow",
    labelKey: "task:filterDimensionWorkflow",
    testId: "mobile-list-filter-workflow",
  },
  {
    key: "repository",
    labelKey: "task:filterDimensionRepository",
    testId: "mobile-list-filter-repository",
  },
];

const STATE_OPTIONS = [
  { value: "review", labelKey: "task:filterStateReview" },
  { value: "in_progress", labelKey: "task:filterStateInProgress" },
  { value: "backlog", labelKey: "task:filterStateBacklog" },
];

export function MobileListResultFiltersDrawer({
  workspaceId,
  count,
  filters,
  onBeginEdit,
  onChange,
  onCancel,
  onApply,
  onClear,
}: {
  workspaceId: string | null;
  count: number;
  filters: FilterClause[];
  onBeginEdit: () => void;
  onChange: (filters: FilterClause[]) => void;
  onCancel: () => void;
  onApply: () => void;
  onClear: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const applyingRef = useRef(false);
  const allWorkflows = useAppStore((state) => state.workflows.items);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const workflows = useMemo(
    () =>
      allWorkflows
        .filter((workflow) => workflow.workspaceId === workspaceId)
        .map((workflow) => ({ value: workflow.id, label: workflow.name })),
    [allWorkflows, workspaceId],
  );
  const repositories = useMemo(
    () =>
      (workspaceId ? (repositoriesByWorkspace[workspaceId] ?? []) : []).map((repo) => {
        const slug = repositorySlug(repo);
        return { value: slug, label: slug };
      }),
    [repositoriesByWorkspace, workspaceId],
  );
  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (open) {
        applyingRef.current = false;
        onBeginEdit();
      } else {
        if (!applyingRef.current) onCancel();
        applyingRef.current = false;
      }
      setOpen(open);
    },
    [onBeginEdit, onCancel],
  );

  return (
    <Drawer open={open} onOpenChange={handleOpenChange}>
      <DrawerTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className="h-11 shrink-0 cursor-pointer gap-2"
          aria-haspopup="dialog"
          data-testid="mobile-list-result-filters"
        >
          <IconFilter className="size-4" aria-hidden="true" />
          {count ? t("tasks:filtersButton", { count }) : t("kanban:filters")}
        </Button>
      </DrawerTrigger>
      <DrawerContent
        data-testid="mobile-list-result-filters-drawer"
        aria-describedby={undefined}
        className="max-h-[min(70dvh,34rem)] overflow-hidden rounded-t-xl"
      >
        <DrawerHeader className="shrink-0 border-b px-4 pb-3 pt-5 text-left">
          <DrawerTitle>{t("kanban:filters")}</DrawerTitle>
        </DrawerHeader>
        <ResultFilterFields
          filters={filters}
          workflows={workflows}
          repositories={repositories}
          onChange={onChange}
        />
        <ResultFiltersFooter
          onClear={onClear}
          onApply={() => {
            applyingRef.current = true;
            onApply();
            setOpen(false);
          }}
        />
      </DrawerContent>
    </Drawer>
  );
}

function ResultFilterFields({
  filters,
  workflows,
  repositories,
  onChange,
}: {
  filters: FilterClause[];
  workflows: Array<{ value: string; label: string }>;
  repositories: Array<{ value: string; label: string }>;
  onChange: (filters: FilterClause[]) => void;
}) {
  const { t } = useTranslation();

  function optionsFor(dimension: Dimension) {
    if (dimension === "state") {
      return STATE_OPTIONS.map((option) => ({ value: option.value, label: t(option.labelKey) }));
    }
    return dimension === "workflow" ? workflows : repositories;
  }

  function selectedValue(dimension: Dimension) {
    const filter = filters.find((entry) => entry.dimension === dimension);
    if (!filter) return "all";
    if (Array.isArray(filter.value)) return filter.value.length === 1 ? filter.value[0] : "all";
    return String(filter.value);
  }

  function changeValue(dimension: Dimension, value: string) {
    const next = filters.filter((filter) => filter.dimension !== dimension);
    if (value !== "all") {
      next.push({
        id: `mobile-result-${dimension}`,
        dimension,
        op: "is",
        value,
      });
    }
    onChange(next);
  }

  return (
    <div className="min-h-0 max-h-[45dvh] space-y-3 overflow-y-auto overscroll-contain px-4 py-3">
      {DIMENSIONS.map((dimension) => (
        <ResultFilterSelect
          key={dimension.key}
          label={t(dimension.labelKey)}
          testId={dimension.testId}
          value={selectedValue(dimension.key)}
          options={optionsFor(dimension.key)}
          onChange={(value) => changeValue(dimension.key, value)}
        />
      ))}
    </div>
  );
}

function ResultFiltersFooter({ onClear, onApply }: { onClear: () => void; onApply: () => void }) {
  const { t } = useTranslation();
  return (
    <DrawerFooter className="shrink-0 flex-row justify-between border-t px-4 pb-[max(1rem,env(safe-area-inset-bottom))] pt-3">
      <Button
        type="button"
        variant="ghost"
        className="h-11 cursor-pointer"
        onClick={onClear}
        data-testid="mobile-list-filters-clear"
      >
        {t("tasks:clearFilters")}
      </Button>
      <Button
        type="button"
        className="h-11 cursor-pointer"
        onClick={onApply}
        data-testid="mobile-list-filters-apply"
      >
        {t("common:apply")}
      </Button>
    </DrawerFooter>
  );
}

function ResultFilterSelect({
  label,
  testId,
  value,
  options,
  onChange,
}: {
  label: string;
  testId: string;
  value: string;
  options: Array<{ value: string; label: string }>;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <label className="text-xs font-medium text-muted-foreground" id={`${testId}-label`}>
        {label}
      </label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger
          className="h-11 w-full cursor-pointer"
          aria-labelledby={`${testId}-label`}
          data-testid={testId}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("tasks:allTasks")}</SelectItem>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
