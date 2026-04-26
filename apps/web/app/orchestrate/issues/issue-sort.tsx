"use client";

import { IconArrowsSort, IconSortAscending, IconSortDescending } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import type { IssueSortField, IssueSortDir } from "@/lib/state/slices/orchestrate/types";

const SORT_FIELDS: { value: IssueSortField; label: string }[] = [
  { value: "updated", label: "Updated" },
  { value: "created", label: "Created" },
  { value: "status", label: "Status" },
  { value: "priority", label: "Priority" },
  { value: "title", label: "Title" },
];

type IssueSortProps = {
  field: IssueSortField;
  dir: IssueSortDir;
  onFieldChange: (field: IssueSortField) => void;
  onDirChange: (dir: IssueSortDir) => void;
};

export function IssueSort({ field, dir, onFieldChange, onDirChange }: IssueSortProps) {
  return (
    <Popover>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button variant="ghost" size="icon-sm" className="cursor-pointer">
              <IconArrowsSort className="h-4 w-4" />
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>Sort</TooltipContent>
      </Tooltip>
      <PopoverContent className="w-48 p-2" align="end">
        <p className="text-xs font-medium px-2 mb-1">Sort by</p>
        <div className="flex flex-col gap-0.5">
          {SORT_FIELDS.map((f) => (
            <button
              key={f.value}
              onClick={() => onFieldChange(f.value)}
              className={cn(
                "flex items-center gap-2 px-2 py-1.5 text-sm rounded-md cursor-pointer",
                field === f.value ? "bg-accent text-foreground" : "hover:bg-accent/50",
              )}
            >
              {f.label}
            </button>
          ))}
        </div>
        <div className="flex gap-1 mt-2 px-2">
          <Button
            variant={dir === "asc" ? "secondary" : "ghost"}
            size="sm"
            className="flex-1 cursor-pointer"
            onClick={() => onDirChange("asc")}
          >
            <IconSortAscending className="h-3.5 w-3.5 mr-1" />
            Asc
          </Button>
          <Button
            variant={dir === "desc" ? "secondary" : "ghost"}
            size="sm"
            className="flex-1 cursor-pointer"
            onClick={() => onDirChange("desc")}
          >
            <IconSortDescending className="h-3.5 w-3.5 mr-1" />
            Desc
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
