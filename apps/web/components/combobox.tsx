"use client";

import { memo, useState } from "react";
import { IconCheck, IconChevronDown } from "@tabler/icons-react";

import { cn } from "@/lib/utils";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";

export type ComboboxOption = {
  value: string;
  label: string;
  description?: string;
  renderLabel?: () => React.ReactNode;
};

interface ComboboxProps {
  options: ComboboxOption[];
  value: string;
  onValueChange: (value: string) => void;
  dropdownLabel?: string;
  placeholder?: string;
  searchPlaceholder?: string;
  emptyMessage?: string;
  disabled?: boolean;
  className?: string;
  triggerClassName?: string;
  showSearch?: boolean;
  testId?: string;
  popoverSide?: "top" | "right" | "bottom" | "left";
  popoverAlign?: "start" | "center" | "end";
  /** When true, the trigger always renders the plain label text instead of renderLabel. */
  plainTrigger?: boolean;
}

function TriggerLabel({
  selectedOption,
  plainTrigger,
  placeholder,
}: {
  selectedOption: ComboboxOption | undefined;
  plainTrigger: boolean;
  placeholder: string;
}) {
  if (!plainTrigger && selectedOption?.renderLabel) {
    return selectedOption.renderLabel();
  }
  return <span className="truncate">{selectedOption?.label || placeholder}</span>;
}

function OptionsList({
  options,
  value,
  onSelect,
}: {
  options: ComboboxOption[];
  value: string;
  onSelect: (value: string) => void;
}) {
  return (
    <CommandGroup>
      {options.map((option) => (
        <CommandItem
          key={option.value}
          value={option.value}
          keywords={[option.label, option.description ?? ""]}
          onSelect={() => onSelect(option.value)}
          className="relative pr-7"
        >
          <div className="flex min-w-0 flex-1 items-center">
            {option.renderLabel ? option.renderLabel() : option.label}
          </div>
          <IconCheck
            className={cn(
              "absolute right-2 h-4 w-4",
              value === option.value ? "opacity-100" : "opacity-0",
            )}
          />
        </CommandItem>
      ))}
    </CommandGroup>
  );
}

export const Combobox = memo(function Combobox({
  options,
  value,
  onValueChange,
  dropdownLabel,
  placeholder = "Select option...",
  searchPlaceholder = "Search...",
  emptyMessage = "No option found.",
  disabled = false,
  className,
  triggerClassName,
  showSearch = true,
  testId,
  popoverSide,
  popoverAlign = "start",
  plainTrigger = false,
}: ComboboxProps) {
  const [open, setOpen] = useState(false);
  // Track the highlighted item. Defaults to the selected value so the current
  // selection is highlighted when the popover opens (not the first item).
  const [highlighted, setHighlighted] = useState("");

  const selectedOption = options.find((option) => option.value === value);

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) setHighlighted(value);
      }}
    >
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          role="combobox"
          aria-expanded={open}
          className={cn("w-full justify-between", !disabled && "cursor-pointer", triggerClassName)}
          disabled={disabled}
          data-testid={testId}
        >
          <div className="flex min-w-0 flex-1 items-center">
            <TriggerLabel
              selectedOption={selectedOption}
              plainTrigger={plainTrigger}
              placeholder={placeholder}
            />
          </div>
          <IconChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        portal={false}
        className={cn(
          "w-[var(--radix-popover-trigger-width)] min-w-[300px] max-w-none p-0 max-h-[var(--radix-popover-content-available-height)]",
          className,
        )}
        side={popoverSide}
        align={popoverAlign}
      >
        <Command value={highlighted} onValueChange={setHighlighted}>
          {dropdownLabel ? (
            <div className="text-muted-foreground px-2 py-1.5 text-xs border-b">
              {dropdownLabel}
            </div>
          ) : null}
          {showSearch && <CommandInput placeholder={searchPlaceholder} className="h-9" />}
          <CommandList>
            <CommandEmpty>{emptyMessage}</CommandEmpty>
            <OptionsList
              options={options}
              value={value}
              onSelect={(v) => {
                onValueChange(v === value ? "" : v);
                setOpen(false);
              }}
            />
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
});
