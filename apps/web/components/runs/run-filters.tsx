"use client";

import { useTranslation } from "react-i18next";
import { IconCheck, IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { ALL_STATUSES, STATUS_FILTER_OPTIONS, type RunStatusFilter } from "./run-status";

export const ANY_AUTOMATION = "any";

export type AutomationOption = { id: string; name: string };

type FilterMenuProps = {
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
  testId: string;
};

function FilterMenu({ value, options, onChange, testId }: FilterMenuProps) {
  const selected = options.find((option) => option.value === value) ?? options[0];
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="h-8 cursor-pointer gap-1.5 text-xs font-normal"
          data-testid={testId}
        >
          <span className="max-w-[16rem] truncate">{selected?.label}</span>
          <IconChevronDown className="h-3.5 w-3.5 opacity-50" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-80 overflow-y-auto">
        {options.map((option) => (
          <DropdownMenuItem
            key={option.value}
            className="cursor-pointer gap-2 text-xs"
            data-testid={`${testId}-option-${option.value}`}
            onSelect={() => onChange(option.value)}
          >
            <IconCheck
              className={cn("h-3.5 w-3.5", option.value === value ? "opacity-100" : "opacity-0")}
            />
            <span className="truncate">{option.label}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

type RunFiltersProps = {
  status: RunStatusFilter;
  onStatusChange: (status: RunStatusFilter) => void;
  automationId: string;
  onAutomationChange: (automationId: string) => void;
  automations: AutomationOption[];
};

export function RunFilters({
  status,
  onStatusChange,
  automationId,
  onAutomationChange,
  automations,
}: RunFiltersProps) {
  const { t } = useTranslation();
  // Both filters are driven off the loaded feed, so the automation list only
  // ever offers automations that have actually produced a run.
  const automationOptions = [
    { value: ANY_AUTOMATION, label: t("automations:anyAutomation") },
    ...automations.map((automation) => ({ value: automation.id, label: automation.name })),
  ];

  return (
    <div className="flex items-center gap-2">
      <FilterMenu
        value={status}
        options={STATUS_FILTER_OPTIONS.map((option) => ({
          value: option.value,
          label: t(option.labelKey),
        }))}
        onChange={(next) => onStatusChange(next as RunStatusFilter)}
        testId="run-status-filter"
      />
      <FilterMenu
        value={automationId}
        options={automationOptions}
        onChange={onAutomationChange}
        testId="run-automation-filter"
      />
    </div>
  );
}

export function isDefaultFilters(status: RunStatusFilter, automationId: string): boolean {
  return status === ALL_STATUSES && automationId === ANY_AUTOMATION;
}
