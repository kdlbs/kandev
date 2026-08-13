"use client";

import type { ComponentType } from "react";
import { useTranslation } from "react-i18next";
import { IconChecklist, IconChevronDown, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import {
  iconForIntegrationPreset,
  type IntegrationPresetIconName,
} from "./integration-preset-icons";

export type IntegrationTaskPreset = {
  id: string;
  label: string;
  hint: string;
  icon?: ComponentType<{ className?: string }>;
  iconName?: IntegrationPresetIconName;
};

export type IntegrationStartTaskMenuProps<T extends IntegrationTaskPreset = IntegrationTaskPreset> =
  {
    presets: T[];
    onSelect: (preset: T) => void;
    triggerLabel?: string;
    triggerAriaLabel?: string;
    triggerTestId?: string;
    itemTestId?: string;
  };

export function IntegrationStartTaskMenu<T extends IntegrationTaskPreset>({
  presets,
  onSelect,
  triggerLabel,
  triggerAriaLabel,
  triggerTestId,
  itemTestId,
}: IntegrationStartTaskMenuProps<T>) {
  const { t } = useTranslation();
  if (presets.length === 0) return null;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          size="sm"
          variant="outline"
          className="h-11 cursor-pointer gap-1 sm:h-7"
          aria-label={triggerAriaLabel ?? t("github:task")}
          data-testid={triggerTestId}
        >
          <IconPlus className="h-3.5 w-3.5" />
          {triggerLabel ?? t("github:task")}
          <IconChevronDown className="h-3 w-3" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-52">
        {presets.map((preset) => {
          const PresetIcon =
            preset.icon ??
            (preset.iconName ? iconForIntegrationPreset(preset.iconName) : IconChecklist);
          return (
            <DropdownMenuItem
              key={preset.id}
              className="min-h-11 cursor-pointer gap-2 sm:min-h-8"
              onSelect={() => onSelect(preset)}
              data-testid={itemTestId}
              data-preset-id={preset.id}
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
