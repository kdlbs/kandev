import { IconCheck } from "@tabler/icons-react";
import type { PillOption } from "./task-create-dialog-pill";
import { cn } from "@/lib/utils";
import { prioritizeSelectedOption, selectorOptionClassName } from "@/lib/utils/selector-options";
import { CommandEmpty, CommandGroup, CommandItem, CommandList } from "@kandev/ui/command";

export function PillCommandList({
  options,
  value,
  onSelect,
  onPointerSelect,
  setOpen,
  emptyMessage,
}: {
  options: PillOption[];
  value: string;
  onSelect: (value: string) => void;
  onPointerSelect: (pointerType: string) => void;
  setOpen: (open: boolean) => void;
  emptyMessage: string;
}) {
  const groups = new Map<string, { label?: string; options: PillOption[] }>();
  for (const option of options) {
    const key = option.group ?? "";
    const group = groups.get(key) ?? { label: option.groupLabel, options: [] };
    group.options.push(option);
    groups.set(key, group);
  }
  const groupOrder = new Map([
    ["policies", 0],
    ["branches", 1],
  ]);
  const orderedGroups = Array.from(groups.entries()).sort(
    ([firstKey], [secondKey]) =>
      (groupOrder.get(firstKey) ?? Number.MAX_SAFE_INTEGER) -
      (groupOrder.get(secondKey) ?? Number.MAX_SAFE_INTEGER),
  );

  return (
    <CommandList>
      <CommandEmpty>{emptyMessage}</CommandEmpty>
      {orderedGroups.map(([key, group]) => (
        <CommandGroup key={key || "ungrouped"} heading={group.label}>
          {prioritizeSelectedOption(group.options, value, (option) => option.value).map(
            (option) => {
              const selected = option.value === value;
              const item = (
                <CommandItem
                  key={option.renderAccessory ? undefined : option.value}
                  value={option.value}
                  keywords={[option.label, ...(option.keywords ?? [])]}
                  disabled={option.disabled}
                  onPointerDown={(event) => onPointerSelect(event.pointerType)}
                  onSelect={() => {
                    onSelect(option.value);
                    setOpen(false);
                  }}
                  className={cn(
                    selectorOptionClassName(selected),
                    option.renderAccessory && "pr-14",
                  )}
                >
                  <div className="min-w-0 flex-1">
                    {option.renderLabel ? option.renderLabel() : option.label}
                  </div>
                  <IconCheck
                    className={cn(
                      "absolute right-2 h-4 w-4",
                      selected ? "opacity-100" : "opacity-0",
                    )}
                  />
                </CommandItem>
              );
              if (!option.renderAccessory) return item;
              return (
                <div key={option.value} className="relative">
                  {item}
                  <div className="absolute inset-y-0 right-7 z-10 flex items-center">
                    {option.renderAccessory()}
                  </div>
                </div>
              );
            },
          )}
        </CommandGroup>
      ))}
    </CommandList>
  );
}
