"use client";

import { useState } from "react";
import { IconCheck, IconChevronDown } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from "@kandev/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { cn } from "@/lib/utils";
import { levelBadgeClass, statusBadgeClass } from "./sentry-issue-common";
import { LEVEL_OPTIONS, STATUS_OPTIONS } from "./sentry-issue-watch-form";
import type { SentryLevel, SentryStatus } from "@/lib/types/sentry";
import { useTranslation } from "react-i18next";
import { resolveOptionLabel } from "@/lib/i18n/option-label";

// LevelMultiSelect / StatusMultiSelect are toggle-badge pickers: click a badge
// to add/remove it from the selection. A match means ANY selected value. Both
// are thin wrappers over the generic BadgeMultiSelect below.

// BadgeMultiSelect renders a toggle-badge picker for any string-union option
// set. `colorClass` resolves the active-state styling for a given option.
//
// `value` is the union member — the key, the selection identity, and what goes
// on the wire. Only `labelKey` is copy, resolved here at render.
function BadgeMultiSelect<T extends string>({
  options,
  selected,
  onToggle,
  colorClass,
}: {
  options: readonly { value: T; labelKey: string }[];
  selected: T[];
  onToggle: (value: T) => void;
  colorClass: (value: T) => string;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap gap-1.5">
      {options.map((option) => {
        const active = selected.includes(option.value);
        return (
          <button
            key={option.value}
            type="button"
            onClick={() => onToggle(option.value)}
            aria-pressed={active}
            className="cursor-pointer"
          >
            <Badge
              variant="outline"
              className={`uppercase ${active ? colorClass(option.value) : ""}`}
            >
              {resolveOptionLabel(t, option)}
            </Badge>
          </button>
        );
      })}
    </div>
  );
}

export function LevelMultiSelect({
  selected,
  onToggle,
}: {
  selected: SentryLevel[];
  onToggle: (level: SentryLevel) => void;
}) {
  return (
    <BadgeMultiSelect
      options={LEVEL_OPTIONS}
      selected={selected}
      onToggle={onToggle}
      colorClass={levelBadgeClass}
    />
  );
}

export function StatusMultiSelect({
  selected,
  onToggle,
}: {
  selected: SentryStatus[];
  onToggle: (status: SentryStatus) => void;
}) {
  return (
    <BadgeMultiSelect
      options={STATUS_OPTIONS}
      selected={selected}
      onToggle={onToggle}
      colorClass={statusBadgeClass}
    />
  );
}

export type ProjectMultiSelectOption = { id: string; label: string };

// ProjectMultiSelect is a dropdown checklist: click the trigger to open a
// searchable list, click a row to toggle it in/out of the selection. Unlike
// LevelMultiSelect/StatusMultiSelect's inline badges, a workspace can have
// dozens of Sentry projects, so a dropdown keeps the form compact. A match
// means ANY selected project (same semantics as levels/statuses).
export function ProjectMultiSelect({
  options,
  selected,
  onChange,
  placeholder,
  disabled = false,
}: {
  options: ProjectMultiSelectOption[];
  selected: string[];
  onChange: (next: string[]) => void;
  placeholder?: string;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const labelById = new Map(options.map((o) => [o.id, o.label]));

  function toggle(id: string) {
    onChange(selected.includes(id) ? selected.filter((s) => s !== id) : [...selected, id]);
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          data-testid="sentry-watch-project-trigger"
          className="flex h-9 w-full cursor-pointer items-center justify-between rounded-md border border-input bg-transparent px-3 text-sm disabled:cursor-not-allowed disabled:opacity-50"
        >
          <ProjectMultiSelectSummary
            selected={selected}
            labelById={labelById}
            placeholder={placeholder ?? t("sentry:selectProjects")}
          />
          <IconChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-[--radix-popover-trigger-width] p-0">
        <Command>
          <CommandInput placeholder={t("sentry:searchProjects")} />
          <CommandList>
            <CommandEmpty>{t("sentry:noProjectsAvailableSentence")}</CommandEmpty>
            {options.map((opt) => {
              const checked = selected.includes(opt.id);
              return (
                <CommandItem key={opt.id} value={opt.label} onSelect={() => toggle(opt.id)}>
                  <IconCheck
                    className={cn("mr-2 h-4 w-4", checked ? "opacity-100" : "opacity-0")}
                  />
                  <span className="truncate">{opt.label}</span>
                </CommandItem>
              );
            })}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

function ProjectMultiSelectSummary({
  selected,
  labelById,
  placeholder,
}: {
  selected: string[];
  labelById: Map<string, string>;
  placeholder: string;
}) {
  const { t } = useTranslation();
  if (selected.length === 0) {
    return <span className="truncate text-muted-foreground">{placeholder}</span>;
  }
  // One selection shows the project's own name — runtime data, never translated.
  if (selected.length === 1) {
    return <span className="truncate">{labelById.get(selected[0]) ?? selected[0]}</span>;
  }
  return (
    <span className="truncate">{t("sentry:projectsSelected", { count: selected.length })}</span>
  );
}
