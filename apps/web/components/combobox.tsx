"use client";

import { memo, useState } from "react";
import { IconCheck, IconChevronDown, IconLoader2 } from "@tabler/icons-react";

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
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

export type ComboboxOption = {
  value: string;
  label: string;
  description?: string;
  keywords?: string[];
  renderLabel?: () => React.ReactNode;
  /** When true the option renders dimmed and isn't selectable. */
  disabled?: boolean;
  /** Tooltip shown on hover when disabled is true. */
  disabledReason?: string;
};

// Custom filter compatible with cmdk's `<Command filter>` prop.
// Returns a number in [0, 1]; >0 means the option is included, sorted desc.
export type ComboboxFilter = (value: string, search: string, keywords?: string[]) => number;

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
  /** Optional custom filter; defaults to cmdk's built-in command-score. */
  filter?: ComboboxFilter;
  /** Optional node rendered to the right of the dropdown label (e.g. refresh button). */
  headerAction?: React.ReactNode;
  /** When true, swap the trigger chevron for a spinner to indicate loading. */
  loading?: boolean;
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

function DisabledOptionItem({ option }: { option: ComboboxOption }) {
  // cmdk's CommandItem swallows pointer events with no native tooltip slot, so
  // we wrap the disabled item in a Tooltip trigger. The trigger is a plain
  // div; CommandItem still owns keyboard nav and rendering.
  const item = (
    <CommandItem
      key={option.value}
      value={option.value}
      keywords={[option.label, option.description ?? ""]}
      disabled
      className="relative pr-7 opacity-50 cursor-not-allowed"
    >
      <div className="flex min-w-0 flex-1 items-center">
        {option.renderLabel ? option.renderLabel() : option.label}
      </div>
    </CommandItem>
  );
  if (!option.disabledReason) return item;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div>{item}</div>
      </TooltipTrigger>
      <TooltipContent side="right">{option.disabledReason}</TooltipContent>
    </Tooltip>
  );
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
      {options.map((option) =>
        option.disabled ? (
          <DisabledOptionItem key={option.value} option={option} />
        ) : (
          <CommandItem
            key={option.value}
            value={option.value}
            keywords={option.keywords ?? [option.label, option.description ?? ""]}
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
        ),
      )}
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
  filter,
  headerAction,
  loading = false,
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
          {loading ? (
            <IconLoader2 className="ml-2 h-4 w-4 shrink-0 animate-spin opacity-50" />
          ) : (
            <IconChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className={cn(
          "w-[var(--radix-popover-trigger-width)] min-w-[300px] max-w-none p-0 max-h-[var(--radix-popover-content-available-height)]",
          className,
        )}
        side={popoverSide}
        align={popoverAlign}
        portal={false}
      >
        <Command value={highlighted} onValueChange={setHighlighted} filter={filter}>
          {dropdownLabel || headerAction ? (
            <div className="text-muted-foreground flex items-center justify-between gap-2 px-2 py-1 text-xs border-b">
              <span>{dropdownLabel}</span>
              {headerAction}
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
