"use client";

import { IconChevronDown, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import type { GitLabTaskPreset } from "./quick-task-launcher";
import { useTranslation } from "react-i18next";

export function StartTaskMenu({
  presets,
  onSelect,
}: {
  presets: GitLabTaskPreset[];
  onSelect: (preset: GitLabTaskPreset) => void;
}) {
  const { t } = useTranslation();
  if (presets.length === 0) return null;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          size="sm"
          variant="outline"
          className="h-11 sm:h-7 gap-1 cursor-pointer"
          aria-label={t("gitlab:createTask")}
        >
          <IconPlus className="h-3.5 w-3.5" />
          {t("gitlab:task")}
          <IconChevronDown className="h-3 w-3" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-52">
        {presets.map((preset) => {
          const PresetIcon = preset.icon;
          return (
            <DropdownMenuItem
              key={preset.id}
              className="min-h-11 sm:min-h-8 cursor-pointer gap-2"
              onSelect={() => onSelect(preset)}
            >
              <PresetIcon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              <div className="min-w-0">
                <div className="text-xs font-medium">{preset.label}</div>
                <div className="truncate text-[11px] text-muted-foreground">{preset.hint}</div>
              </div>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
