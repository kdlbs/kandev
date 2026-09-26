"use client";

import { IconLayoutRows } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import type { TaskGroupBy } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";

// `labelKey`, not `label` — module scope freezes a `t()` at the boot locale.
// The `value`s are the persisted grouping ids and stay untranslated.
const GROUP_OPTIONS: { value: TaskGroupBy; labelKey: string }[] = [
  { value: "none", labelKey: "office:noGrouping" },
  { value: "status", labelKey: "common:status" },
  { value: "priority", labelKey: "office:priority" },
  { value: "assignee", labelKey: "office:assignee" },
  { value: "project", labelKey: "office:project" },
  { value: "parent", labelKey: "office:parent" },
];

type IssueGroupProps = {
  groupBy: TaskGroupBy;
  onGroupByChange: (groupBy: TaskGroupBy) => void;
};

export function TaskGroup({ groupBy, onGroupByChange }: IssueGroupProps) {
  const { t } = useTranslation();
  return (
    <Popover>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              variant={groupBy !== "none" ? "secondary" : "ghost"}
              size="icon-sm"
              className="cursor-pointer"
            >
              <IconLayoutRows className="h-4 w-4" />
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{t("office:groupBy")}</TooltipContent>
      </Tooltip>
      <PopoverContent className="w-44 p-2" align="end">
        <p className="text-xs font-medium px-2 mb-1">{t("office:groupBy")}</p>
        <div className="flex flex-col gap-0.5">
          {GROUP_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              onClick={() => onGroupByChange(opt.value)}
              className={cn(
                "flex items-center gap-2 rounded-md border border-transparent px-2 py-1.5 text-left text-sm cursor-pointer",
                groupBy === opt.value
                  ? "border-primary/50 bg-card text-foreground"
                  : "hover:bg-muted",
              )}
            >
              {t(opt.labelKey)}
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}
