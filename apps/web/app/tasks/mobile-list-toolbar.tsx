"use client";

import { Button } from "@kandev/ui/button";
import { IconX } from "@tabler/icons-react";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { TaskSearchInput } from "@/components/kanban/task-search-input";
import type { FilterClause } from "@/lib/state/slices/ui/sidebar-view-types";
import { repositorySlug } from "@/lib/repository-slug";
import { MobileListResultFiltersDrawer } from "./mobile-list-result-filters-drawer";

type NamedOption = { value: string; label: string };

type Props = {
  open: boolean;
  searchQuery: string;
  onSearchChange: (query: string) => void;
  isSearchLoading: boolean;
  resultFilters: FilterClause[];
  hasLinkScope: boolean;
  onClearLinkScope: () => void;
  onRemoveResultFilter: (dimension: FilterClause["dimension"]) => void;
  filterDraft: FilterClause[];
  onBeginFilterEdit: () => void;
  onSetFilterDraft: (filters: FilterClause[]) => void;
  onCancelFilterEdit: () => void;
  onApplyFilterEdit: () => void;
  onClearFilterDraft: () => void;
  workspaceId: string | null;
};

export function MobileListToolbar(props: Props) {
  const { t } = useTranslation();
  const workflows = useAppStore((state) => state.workflows.items);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const workflowOptions = useMemo(
    () =>
      workflows
        .filter((workflow) => workflow.workspaceId === props.workspaceId)
        .map((workflow) => ({ value: workflow.id, label: workflow.name })),
    [props.workspaceId, workflows],
  );
  const repositoryOptions = useMemo(
    () =>
      (props.workspaceId ? (repositoriesByWorkspace[props.workspaceId] ?? []) : []).map((repo) => {
        const slug = repositorySlug(repo);
        return { value: slug, label: slug };
      }),
    [props.workspaceId, repositoriesByWorkspace],
  );
  if (!props.open) return null;
  return (
    <div
      id="mobile-list-search-row"
      className="shrink-0 space-y-2 px-4 pb-2"
      data-testid="mobile-list-search-row"
    >
      <div className="flex min-w-0 items-center gap-2">
        <TaskSearchInput
          value={props.searchQuery}
          onChange={props.onSearchChange}
          placeholder={t("kanban:searchTasksPlaceholder")}
          isLoading={props.isSearchLoading}
          autoFocus
          inputTestId="mobile-list-search-input"
          clearButtonLabel={t("tasks:clearSearch")}
          className="min-w-0 flex-1 [&_[data-slot=input]]:h-11 [&_[data-slot=input]]:text-base"
          clearButtonClassName="right-0 size-11"
        />
        <MobileListResultFiltersDrawer
          workspaceId={props.workspaceId}
          count={props.resultFilters.length}
          filters={props.filterDraft}
          onBeginEdit={props.onBeginFilterEdit}
          onChange={props.onSetFilterDraft}
          onCancel={props.onCancelFilterEdit}
          onApply={props.onApplyFilterEdit}
          onClear={props.onClearFilterDraft}
        />
      </div>
      {(props.resultFilters.length > 0 || props.hasLinkScope) && (
        <div
          className="flex flex-wrap items-center gap-1.5"
          data-testid="mobile-list-result-summary"
        >
          {props.resultFilters.length > 0 && (
            <div
              className="flex flex-wrap gap-1.5"
              data-testid="mobile-list-temporary-filter-summary"
            >
              {props.resultFilters.map((filter) => (
                <ResultFilterSummary
                  key={filter.id}
                  filter={filter}
                  workflows={workflowOptions}
                  repositories={repositoryOptions}
                  onRemove={() => props.onRemoveResultFilter(filter.dimension)}
                />
              ))}
            </div>
          )}
          {props.hasLinkScope && (
            <div
              className="flex min-h-7 items-center gap-1 rounded-full border border-border px-2 text-xs text-muted-foreground"
              data-testid="mobile-list-link-scope"
            >
              <span>{t("tasks:linkedResultScope")}</span>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-6 cursor-pointer"
                aria-label={t("tasks:clearFilters")}
                onClick={props.onClearLinkScope}
                data-testid="mobile-list-clear-link-scope"
              >
                <IconX className="size-3.5" aria-hidden="true" />
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function ResultFilterSummary({
  filter,
  workflows,
  repositories,
  onRemove,
}: {
  filter: FilterClause;
  workflows: NamedOption[];
  repositories: NamedOption[];
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const dimensionLabel = t(`task:filterDimension${capitalize(filter.dimension)}`);
  const option = [...workflows, ...repositories].find(
    (item) => item.value === String(filter.value),
  );
  const stateLabel: Record<string, string> = {
    backlog: t("task:filterStateBacklog"),
    in_progress: t("task:filterStateInProgress"),
    review: t("task:filterStateReview"),
  };
  const value = Array.isArray(filter.value)
    ? filter.value.map((item) => stateLabel[item] ?? item).join(", ")
    : (stateLabel[String(filter.value)] ?? option?.label ?? String(filter.value));
  return (
    <span className="flex min-h-7 items-center gap-1 rounded-full border border-border px-2 text-xs">
      <span className="text-muted-foreground">{dimensionLabel}: </span>
      <span>{value}</span>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-6 cursor-pointer"
        aria-label={t("task:removeFilter")}
        onClick={onRemove}
        data-testid={`mobile-list-remove-filter-${filter.dimension}`}
      >
        <IconX className="size-3.5" aria-hidden="true" />
      </Button>
    </span>
  );
}

function capitalize(value: string) {
  return value.charAt(0).toUpperCase() + value.slice(1);
}
