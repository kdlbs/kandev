"use client";

import { Fragment, useState } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@kandev/ui/command";
import { cn } from "@/lib/utils";
import { buildOptionGroups, hasGroupedOptions } from "./filter-option-groups";

export type MultiSelectOption = { value: string; label: string; color?: string; group?: string };

type Props = {
  options: MultiSelectOption[];
  selected: string[];
  onChange: (next: string[]) => void;
  placeholder?: string;
  searchPlaceholder?: string;
};

export function FilterMultiSelect({
  options,
  selected,
  onChange,
  placeholder = "Select values",
  searchPlaceholder = "Search…",
}: Props) {
  const [open, setOpen] = useState(false);
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
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          data-testid="filter-value-multi"
          className="flex h-7 min-w-0 flex-1 cursor-pointer items-center gap-1 rounded-md border border-input bg-transparent px-2 text-xs transition-colors hover:bg-accent/40"
        >
          <MultiSelectSummary
            selected={selected}
            labelByValue={labelByValue}
            colorByValue={colorByValue}
            placeholder={placeholder}
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
          <CommandInput placeholder={searchPlaceholder} />
          <CommandList>
            <CommandEmpty>No options.</CommandEmpty>
            <GroupedOptions options={options} selectedSet={selectedSet} onToggle={toggle} />
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

function GroupedOptions({
  options,
  selectedSet,
  onToggle,
}: {
  options: MultiSelectOption[];
  selectedSet: Set<string>;
  onToggle: (value: string) => void;
}) {
  if (!hasGroupedOptions(options)) {
    return (
      <>
        {options.map((opt) => (
          <OptionRow
            key={opt.value}
            option={opt}
            checked={selectedSet.has(opt.value)}
            onSelect={() => onToggle(opt.value)}
          />
        ))}
      </>
    );
  }

  const groups = buildOptionGroups(options);

  return (
    <>
      {groups.map((g, idx) => (
        <Fragment key={g.heading || `__ungrouped__${idx}`}>
          {idx > 0 && <CommandSeparator />}
          <CommandGroup heading={g.heading || undefined}>
            {g.items.map((opt) => (
              <OptionRow
                key={opt.value}
                option={opt}
                checked={selectedSet.has(opt.value)}
                onSelect={() => onToggle(opt.value)}
              />
            ))}
          </CommandGroup>
        </Fragment>
      ))}
    </>
  );
}

function OptionRow({
  option,
  checked,
  onSelect,
}: {
  option: MultiSelectOption;
  checked: boolean;
  onSelect: () => void;
}) {
  return (
    <CommandItem
      // cmdk identifies and filters items by `value`; include `option.value`
      // so same-titled steps under one workflow don't collide.
      value={[option.group, option.label, option.value].filter(Boolean).join(" ")}
      onSelect={onSelect}
      data-checked={checked}
      data-testid="filter-value-multi-option"
      data-value={option.value}
      data-active={checked}
    >
      {option.color && (
        <span className={cn("mr-1 block h-2 w-2 shrink-0 rounded-full", option.color)} />
      )}
      <span className="truncate">{option.label}</span>
    </CommandItem>
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
