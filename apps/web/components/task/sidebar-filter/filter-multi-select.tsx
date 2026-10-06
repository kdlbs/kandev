"use client";

import { useState } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Command, CommandEmpty, CommandInput, CommandList } from "@kandev/ui/command";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { cn } from "@/lib/utils";
import { FilterMultiSelectOptions } from "./filter-multi-select-options";
import { useTranslation } from "react-i18next";

export type MultiSelectOption = { value: string; label: string; color?: string; group?: string };

type Props = {
  options: MultiSelectOption[];
  selected: string[];
  onChange: (next: string[]) => void;
  placeholder?: string;
  searchPlaceholder?: string;
  className?: string;
};

export function FilterMultiSelect({
  options,
  selected,
  onChange,
  placeholder,
  searchPlaceholder,
  className,
}: Props) {
  const { t } = useTranslation();
  const resolvedPlaceholder = placeholder ?? t("task:selectValues");
  const resolvedSearchPlaceholder = searchPlaceholder ?? t("task:searchEllipsis");
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const selectedSet = new Set(selected);
  const labelByValue = new Map(options.map((o) => [o.value, o.label]));
  const colorByValue = new Map(options.map((o) => [o.value, o.color]));

  function toggle(value: string) {
    const next = new Set(selectedSet);
    if (next.has(value)) next.delete(value);
    else next.add(value);
    onChange([...next]);
  }

  return (
    <Popover
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        if (!nextOpen) setSearch("");
      }}
    >
      <PopoverTrigger asChild>
        <button
          type="button"
          data-testid="filter-value-multi"
          className={cn(
            `${controlSizingClassName("standard")} flex min-w-0 flex-1 cursor-pointer items-center gap-1 rounded-md border border-input bg-transparent px-2 text-xs transition-colors hover:bg-accent/40`,
            className,
          )}
        >
          <MultiSelectSummary
            selected={selected}
            labelByValue={labelByValue}
            colorByValue={colorByValue}
            placeholder={resolvedPlaceholder}
          />
          <IconChevronDown className="ml-auto h-3 w-3 shrink-0 opacity-50" />
        </button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[14rem] p-0"
        align="start"
        data-testid="filter-value-multi-popover"
      >
        <Command>
          <CommandInput
            placeholder={resolvedSearchPlaceholder}
            value={search}
            onValueChange={setSearch}
          />
          <CommandList>
            <CommandEmpty>{t("task:noOptions2")}</CommandEmpty>
            <FilterMultiSelectOptions
              options={options}
              selectedSet={selectedSet}
              search={search}
              onToggle={toggle}
            />
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

function MultiSelectSummary({
  selected,
  labelByValue,
  colorByValue,
  placeholder,
}: {
  selected: string[];
  labelByValue: Map<string, string>;
  colorByValue: Map<string, string | undefined>;
  placeholder: string;
}) {
  if (selected.length === 0) {
    return <span className="truncate text-muted-foreground">{placeholder}</span>;
  }
  if (selected.length === 1) {
    const value = selected[0];
    const color = colorByValue.get(value);
    return (
      <span className="flex min-w-0 items-center gap-1.5">
        {color && <span className={cn("block h-2 w-2 shrink-0 rounded-full", color)} />}
        <span className="truncate">{labelByValue.get(value) ?? value}</span>
      </span>
    );
  }
  return <span className="truncate">{selected.length} selected</span>;
}
