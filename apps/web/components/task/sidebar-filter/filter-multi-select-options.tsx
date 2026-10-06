import { Fragment } from "react";
import { CommandGroup, CommandItem, CommandSeparator } from "@kandev/ui/command";
import { cn } from "@/lib/utils";
import {
  buildOptionGroups,
  hasGroupedOptions,
  partitionSelectedOptions,
} from "./filter-option-groups";
import type { MultiSelectOption } from "./filter-multi-select";

type Props = {
  options: MultiSelectOption[];
  selectedSet: Set<string>;
  search: string;
  onToggle: (value: string) => void;
};

export function FilterMultiSelectOptions({ options, selectedSet, search, onToggle }: Props) {
  if (search)
    return <GroupedOptions options={options} selectedSet={selectedSet} onToggle={onToggle} />;
  const { selected, unselected } = partitionSelectedOptions(options, selectedSet);
  return (
    <>
      <GroupedOptions options={selected} selectedSet={selectedSet} onToggle={onToggle} />
      {selected.length > 0 && unselected.length > 0 && <CommandSeparator />}
      <GroupedOptions options={unselected} selectedSet={selectedSet} onToggle={onToggle} />
    </>
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
      className="max-md:min-h-12 [@media(pointer:coarse)]:min-h-12"
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
