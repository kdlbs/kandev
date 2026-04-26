"use client";

import { IconLayoutRows } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { cn } from "@/lib/utils";
import type { IssueGroupBy } from "@/lib/state/slices/orchestrate/types";

const GROUP_OPTIONS: { value: IssueGroupBy; label: string }[] = [
  { value: "none", label: "No grouping" },
  { value: "status", label: "Status" },
  { value: "priority", label: "Priority" },
  { value: "assignee", label: "Assignee" },
  { value: "project", label: "Project" },
  { value: "parent", label: "Parent" },
];

type IssueGroupProps = {
  groupBy: IssueGroupBy;
  onGroupByChange: (groupBy: IssueGroupBy) => void;
};

export function IssueGroup({ groupBy, onGroupByChange }: IssueGroupProps) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant={groupBy !== "none" ? "secondary" : "ghost"}
          size="icon-sm"
          className="cursor-pointer"
        >
          <IconLayoutRows className="h-4 w-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-44 p-2" align="end">
        <p className="text-xs font-medium px-2 mb-1">Group by</p>
        <div className="flex flex-col gap-0.5">
          {GROUP_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              onClick={() => onGroupByChange(opt.value)}
              className={cn(
                "flex items-center gap-2 px-2 py-1.5 text-sm rounded-md cursor-pointer text-left",
                groupBy === opt.value ? "bg-accent text-foreground" : "hover:bg-accent/50",
              )}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}
