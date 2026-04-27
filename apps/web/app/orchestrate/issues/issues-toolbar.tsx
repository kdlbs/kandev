"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { IconPlus, IconSearch, IconList, IconColumns3, IconListTree } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Separator } from "@kandev/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type {
  IssueFilterState,
  IssueViewMode,
  IssueSortField,
  IssueSortDir,
  IssueGroupBy,
} from "@/lib/state/slices/orchestrate/types";
import { IssueFilters } from "./issue-filters";
import { IssueSort } from "./issue-sort";
import { IssueGroup } from "./issue-group";

type IssuesToolbarProps = {
  viewMode: IssueViewMode;
  nestingEnabled: boolean;
  filters: IssueFilterState;
  sortField: IssueSortField;
  sortDir: IssueSortDir;
  groupBy: IssueGroupBy;
  onViewModeChange: (mode: IssueViewMode) => void;
  onToggleNesting: () => void;
  onFilterChange: (filters: Partial<IssueFilterState>) => void;
  onSortFieldChange: (field: IssueSortField) => void;
  onSortDirChange: (dir: IssueSortDir) => void;
  onGroupByChange: (groupBy: IssueGroupBy) => void;
  onSearchChange: (search: string) => void;
  onNewIssue?: () => void;
};

function ViewModeToggles({
  viewMode,
  nestingEnabled,
  onViewModeChange,
  onToggleNesting,
}: {
  viewMode: IssueViewMode;
  nestingEnabled: boolean;
  onViewModeChange: (mode: IssueViewMode) => void;
  onToggleNesting: () => void;
}) {
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant={viewMode === "list" ? "secondary" : "ghost"}
            size="icon-sm"
            className="cursor-pointer"
            onClick={() => onViewModeChange("list")}
          >
            <IconList className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>List view</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant={viewMode === "board" ? "secondary" : "ghost"}
            size="icon-sm"
            className="cursor-pointer"
            onClick={() => onViewModeChange("board")}
          >
            <IconColumns3 className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Board view</TooltipContent>
      </Tooltip>
      <Separator orientation="vertical" className="h-6 mx-1" />
      {viewMode === "list" && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant={nestingEnabled ? "secondary" : "ghost"}
              size="icon-sm"
              className="cursor-pointer"
              onClick={onToggleNesting}
            >
              <IconListTree className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Toggle nesting</TooltipContent>
        </Tooltip>
      )}
    </>
  );
}

export function IssuesToolbar({
  viewMode,
  nestingEnabled,
  filters,
  sortField,
  sortDir,
  groupBy,
  onViewModeChange,
  onToggleNesting,
  onFilterChange,
  onSortFieldChange,
  onSortDirChange,
  onGroupByChange,
  onSearchChange,
  onNewIssue,
}: IssuesToolbarProps) {
  const [searchValue, setSearchValue] = useState(filters.search);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleSearchInput = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const val = e.target.value;
      setSearchValue(val);
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(() => onSearchChange(val), 250);
    },
    [onSearchChange],
  );

  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, []);

  return (
    <div className="flex items-center gap-2">
      <Button size="sm" className="cursor-pointer" onClick={onNewIssue}>
        <IconPlus className="h-4 w-4 mr-1" />
        New Issue
      </Button>
      <div className="relative flex-1 max-w-[300px]">
        <IconSearch className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Search issues..."
          className="pl-8 h-9 text-sm"
          value={searchValue}
          onChange={handleSearchInput}
        />
      </div>
      <div className="ml-auto flex items-center gap-1">
        <ViewModeToggles
          viewMode={viewMode}
          nestingEnabled={nestingEnabled}
          onViewModeChange={onViewModeChange}
          onToggleNesting={onToggleNesting}
        />
        <IssueFilters filters={filters} onFilterChange={onFilterChange} />
        <IssueSort
          field={sortField}
          dir={sortDir}
          onFieldChange={onSortFieldChange}
          onDirChange={onSortDirChange}
        />
        <IssueGroup groupBy={groupBy} onGroupByChange={onGroupByChange} />
      </div>
    </div>
  );
}
