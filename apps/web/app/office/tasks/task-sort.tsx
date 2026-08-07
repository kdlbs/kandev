"use client";

import { IconArrowsSort, IconSortAscending, IconSortDescending } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import type { TaskSortField, TaskSortDir } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";

// `labelKey`, not `label` — module scope freezes a `t()` at the boot locale.
// The `value`s are the persisted sort-field ids and stay untranslated.
const SORT_FIELDS: { value: TaskSortField; labelKey: string }[] = [
  { value: "updated", labelKey: "office:sortUpdated" },
  { value: "created", labelKey: "office:sortCreated" },
  { value: "status", labelKey: "common:status" },
  { value: "priority", labelKey: "office:priority" },
  { value: "title", labelKey: "office:title" },
];

type IssueSortProps = {
  field: TaskSortField;
  dir: TaskSortDir;
  onFieldChange: (field: TaskSortField) => void;
  onDirChange: (dir: TaskSortDir) => void;
};

export function TaskSort({ field, dir, onFieldChange, onDirChange }: IssueSortProps) {
  const { t } = useTranslation();
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
        <TooltipContent>{t("office:sort")}</TooltipContent>
      </Tooltip>
      <PopoverContent className="w-48 p-2" align="end">
        <p className="text-xs font-medium px-2 mb-1">{t("office:sortBy")}</p>
        <div className="flex flex-col gap-0.5">
          {SORT_FIELDS.map((f) => (
            <button
              key={f.value}
              onClick={() => onFieldChange(f.value)}
              className={cn(
                "flex items-center gap-2 rounded-md border border-transparent px-2 py-1.5 text-sm cursor-pointer",
                field === f.value ? "border-primary/50 bg-card text-foreground" : "hover:bg-muted",
              )}
            >
              {t(f.labelKey)}
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
            {t("office:asc")}
          </Button>
          <Button
            variant={dir === "desc" ? "secondary" : "ghost"}
            size="sm"
            className="flex-1 cursor-pointer"
            onClick={() => onDirChange("desc")}
          >
            <IconSortDescending className="h-3.5 w-3.5 mr-1" />
            {t("office:desc")}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
