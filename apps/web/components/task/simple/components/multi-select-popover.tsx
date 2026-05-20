"use client";

import { useMemo, useState } from "react";
import { IconX } from "@tabler/icons-react";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { cn } from "@/lib/utils";

export type MultiSelectItem = {
  id: string;
  label: string;
  keywords?: string[];
};

type MultiSelectPopoverProps<T extends MultiSelectItem> = {
  items: T[];
  selectedIds: string[];
  onAdd: (id: string) => void | Promise<void>;
  onRemove: (id: string) => void | Promise<void>;
  renderChip: (item: T, onRemove: () => void) => React.ReactNode;
  renderItem: (item: T) => React.ReactNode;
  addLabel?: string;
  emptyMessage?: string;
  searchPlaceholder?: string;
  testId?: string;
};

function ChipsRow<T extends MultiSelectItem>({
  selected,
  renderChip,
  onRemove,
  addLabel,
}: {
  selected: T[];
  renderChip: MultiSelectPopoverProps<T>["renderChip"];
  onRemove: (id: string) => void | Promise<void>;
  addLabel: string;
}) {
  if (selected.length === 0) {
    return <span className="text-muted-foreground text-xs">{addLabel}</span>;
  }
  return (
    <>
      {selected.map((item) =>
        renderChip(item, () => {
          void onRemove(item.id);
        }),
      )}
    </>
  );
}

export function MultiSelectPopover<T extends MultiSelectItem>({
  items,
  selectedIds,
  onAdd,
  onRemove,
  renderChip,
  renderItem,
  addLabel = "+ Add",
  emptyMessage = "No items found.",
  searchPlaceholder = "Search...",
  testId,
}: MultiSelectPopoverProps<T>) {
  const [open, setOpen] = useState(false);

  const selected = useMemo(
    () => items.filter((i) => selectedIds.includes(i.id)),
    [items, selectedIds],
  );

  const addable = useMemo(
    () => items.filter((i) => !selectedIds.includes(i.id)),
    [items, selectedIds],
  );

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-haspopup="listbox"
          aria-expanded={open}
          data-testid={testId}
          className={cn(
            "flex flex-wrap items-center justify-end gap-1 ml-auto",
            "cursor-pointer rounded px-2 py-1 hover:bg-accent/50 min-h-[28px] w-full",
          )}
        >
          <ChipsRow
            selected={selected}
            renderChip={renderChip}
            onRemove={onRemove}
            addLabel={addLabel}
          />
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 p-0" portal={false}>
        <Command>
          <CommandInput placeholder={searchPlaceholder} className="h-9" />
          <CommandList>
            <CommandEmpty>{emptyMessage}</CommandEmpty>
            {selected.length > 0 && (
              <CommandGroup heading="Selected">
                {selected.map((item) => (
                  <CommandItem
                    key={item.id}
                    value={item.id}
                    keywords={item.keywords ?? [item.label]}
                    onSelect={() => {
                      void onRemove(item.id);
                    }}
                    className="justify-between"
                    data-testid={`multi-select-remove-${item.id}`}
                  >
                    {renderItem(item)}
                    <IconX className="h-3 w-3 opacity-60" />
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
            {addable.length > 0 && (
              <CommandGroup heading={selected.length > 0 ? "Add more" : undefined}>
                {addable.map((item) => (
                  <CommandItem
                    key={item.id}
                    value={item.id}
                    keywords={item.keywords ?? [item.label]}
                    onSelect={() => {
                      void onAdd(item.id);
                    }}
                    data-testid={`multi-select-add-${item.id}`}
                  >
                    {renderItem(item)}
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
