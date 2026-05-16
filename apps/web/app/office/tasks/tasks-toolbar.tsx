"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { IconPlus, IconSearch, IconList, IconColumns3, IconListTree } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Separator } from "@kandev/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import type {
  TaskFilterState,
  TaskViewMode,
  TaskSortField,
  TaskSortDir,
  TaskGroupBy,
} from "@/lib/state/slices/office/types";
import { TaskFilters } from "./task-filters";
import { TaskSort } from "./task-sort";
import { TaskGroup } from "./task-group";

type IssuesToolbarProps = {
  viewMode: TaskViewMode;
  nestingEnabled: boolean;
  filters: TaskFilterState;
  sortField: TaskSortField;
  sortDir: TaskSortDir;
  groupBy: TaskGroupBy;
  showSystem: boolean;
  onViewModeChange: (mode: TaskViewMode) => void;
  onToggleNesting: () => void;
  onFilterChange: (filters: Partial<TaskFilterState>) => void;
  onSortFieldChange: (field: TaskSortField) => void;
  onSortDirChange: (dir: TaskSortDir) => void;
  onGroupByChange: (groupBy: TaskGroupBy) => void;
  onSearchChange: (search: string) => void;
  onShowSystemChange: (next: boolean) => void;
  onNewIssue?: () => void;
};

function ViewModeToggles({
  viewMode,
  nestingEnabled,
  onViewModeChange,
  onToggleNesting,
}: {
  viewMode: TaskViewMode;
  nestingEnabled: boolean;
  onViewModeChange: (mode: TaskViewMode) => void;
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

export function TasksToolbar({
  viewMode,
  nestingEnabled,
  filters,
  sortField,
  sortDir,
  groupBy,
  showSystem,
  onViewModeChange,
  onToggleNesting,
  onFilterChange,
  onSortFieldChange,
  onSortDirChange,
  onGroupByChange,
  onSearchChange,
  onShowSystemChange,
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
      <Button className="cursor-pointer" onClick={onNewIssue}>
        <IconPlus className="h-4 w-4 mr-1" />
        New Task
      </Button>
      <div className="relative flex-1 max-w-[300px]">
        <IconSearch className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Search tasks..."
          className="pl-8"
          value={searchValue}
          onChange={handleSearchInput}
        />
      </div>
      <Label className="flex items-center gap-2 text-xs text-muted-foreground cursor-pointer select-none">
        <Checkbox
          checked={showSystem}
          onCheckedChange={(v) => onShowSystemChange(v === true)}
          className="cursor-pointer"
        />
        Show system tasks
      </Label>
      <div className="ml-auto flex items-center gap-1">
        <ViewModeToggles
          viewMode={viewMode}
          nestingEnabled={nestingEnabled}
          onViewModeChange={onViewModeChange}
          onToggleNesting={onToggleNesting}
        />
        <TaskFilters filters={filters} onFilterChange={onFilterChange} />
        <TaskSort
          field={sortField}
          dir={sortDir}
          onFieldChange={onSortFieldChange}
          onDirChange={onSortDirChange}
        />
        <TaskGroup groupBy={groupBy} onGroupByChange={onGroupByChange} />
      </div>
    </div>
  );
}
