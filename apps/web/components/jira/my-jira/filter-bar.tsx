"use client";

import { Button } from "@kandev/ui/button";
import type { JiraProject } from "@/lib/types/jira";
import { AssigneePill, ProjectPill, StatusPill } from "./filter-pills";
import type { FilterState } from "./filter-model";
import { DEFAULT_FILTERS } from "./filter-model";

type FilterBarProps = {
  filters: FilterState;
  onChange: (filters: FilterState) => void;
  projects: JiraProject[];
  hasActiveFilters: boolean;
  onClear: () => void;
};

export function FilterBar({
  filters,
  onChange,
  projects,
  hasActiveFilters,
  onClear,
}: FilterBarProps) {
  return (
    <div className="flex items-center gap-2 flex-wrap px-6 py-2.5 border-b shrink-0 bg-muted/20">
      <ProjectPill
        projects={projects}
        value={filters.projectKeys}
        onChange={(projectKeys) => onChange({ ...filters, projectKeys })}
      />
      <StatusPill
        value={filters.statusCategories}
        onChange={(statusCategories) => onChange({ ...filters, statusCategories })}
      />
      <AssigneePill
        value={filters.assignee}
        onChange={(assignee) => onChange({ ...filters, assignee })}
      />
      {hasActiveFilters && (
        <Button
          variant="ghost"
          size="sm"
          onClick={onClear}
          className="cursor-pointer h-7 text-xs ml-1"
        >
          Clear filters
        </Button>
      )}
    </div>
  );
}

export function hasActiveFilters(f: FilterState): boolean {
  return (
    f.projectKeys.length > 0 ||
    f.statusCategories.length > 0 ||
    f.assignee !== DEFAULT_FILTERS.assignee ||
    f.searchText.trim().length > 0
  );
}
