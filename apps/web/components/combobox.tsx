"use client";

import { memo, useState, type Ref } from "react";
import { IconCheck, IconChevronDown, IconLoader2 } from "@tabler/icons-react";

import { cn } from "@/lib/utils";
import { prioritizeSelectedOption, selectorOptionClassName } from "@/lib/utils/selector-options";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@kandev/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import { t } from "@/lib/i18n";

export type ComboboxOption = {
  value: string;
  label: string;
  description?: string;
  keywords?: string[];
  renderLabel?: () => React.ReactNode;
  /** Optional label renderer for the selected value inside the trigger. */
  renderTriggerLabel?: () => React.ReactNode;
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
  ariaLabel?: string;
  dropdownLabel?: string;
  placeholder?: string;
  searchPlaceholder?: string;
  emptyMessage?: string;
  disabled?: boolean;
  className?: string;
  triggerClassName?: string;
  showSearch?: boolean;
  testId?: string;
  dropdownTestId?: string;
  popoverSide?: "top" | "right" | "bottom" | "left";
  popoverAlign?: "start" | "center" | "end";
  popoverPortal?: boolean;
  /** When true, the trigger always renders the plain label text instead of renderLabel. */
  plainTrigger?: boolean;
  /** Optional custom filter; defaults to cmdk's built-in command-score. */
  filter?: ComboboxFilter;
  /** Optional node rendered to the right of the dropdown label (e.g. refresh button). */
  headerAction?: React.ReactNode;
  /** When true, swap the trigger chevron for a spinner to indicate loading. */
  loading?: boolean;
  /** When true, keep option and trigger hit areas touch-sized. */
  touchTarget?: boolean;
  /** Optional id for associating a visible form label with the trigger. */
  triggerId?: string;
  /** Ref for consumers that anchor a local confirmation to this trigger. */
  triggerRef?: Ref<HTMLButtonElement>;
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
  if (!plainTrigger && selectedOption?.renderTriggerLabel) {
    return selectedOption.renderTriggerLabel();
  }
  if (!plainTrigger && selectedOption?.renderLabel) {
    return selectedOption.renderLabel();
  }
  return <span className="truncate">{selectedOption?.label || placeholder}</span>;
}

function OptionsList({
  options,
  value,
  onSelect,
  touchTarget,
}: {
  options: ComboboxOption[];
  value: string;
  onSelect: (value: string) => void;
  touchTarget: boolean;
}) {
  const orderedOptions = prioritizeSelectedOption(options, value, (option) => option.value);
  const selected = orderedOptions.find((option) => option.value === value);
  const remaining = selected
    ? orderedOptions.filter((option) => option.value !== value)
    : orderedOptions;
  const enabled = remaining.filter((o) => !o.disabled);
  const disabled = remaining.filter((o) => o.disabled);

  const renderItem = (option: ComboboxOption) => {
    const item = (
      <CommandItem
        key={option.value}
        value={option.value}
        keywords={option.keywords ?? [option.label, option.description ?? ""]}
        onSelect={() => !option.disabled && onSelect(option.value)}
        disabled={option.disabled}
        className={selectorOptionClassName(option.value === value, option.disabled, touchTarget)}
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
    );
    // cmdk's CommandItem swallows pointer events with no native tooltip slot;
    // wrap disabled items in a Tooltip trigger so the disabled reason shows.
    if (option.disabled && option.disabledReason) {
      return (
        <Tooltip key={option.value}>
          <TooltipTrigger asChild>
            <div>{item}</div>
          </TooltipTrigger>
          <TooltipContent side="right">{option.disabledReason}</TooltipContent>
        </Tooltip>
      );
    }
    return item;
  };

  return (
    <>
      {selected && <CommandGroup>{renderItem(selected)}</CommandGroup>}
      {enabled.length > 0 && <CommandGroup>{enabled.map(renderItem)}</CommandGroup>}
      {disabled.length > 0 && (
        <>
          <CommandSeparator />
          <CommandGroup>{disabled.map(renderItem)}</CommandGroup>
        </>
      )}
    </>
  );
}

function ComboboxTrigger({
  options,
  value,
  ariaLabel,
  open,
  disabled,
  touchTarget,
  triggerClassName,
  plainTrigger,
  placeholder,
  loading,
  testId,
  triggerId,
  triggerRef,
}: {
  options: ComboboxOption[];
  value: string;
  ariaLabel?: string;
  open: boolean;
  disabled: boolean;
  touchTarget: boolean;
  triggerClassName?: string;
  plainTrigger: boolean;
  placeholder: string;
  loading: boolean;
  testId?: string;
  triggerId?: string;
  triggerRef?: Ref<HTMLButtonElement>;
}) {
  return (
    <PopoverTrigger asChild>
      <Button
        ref={triggerRef}
        id={triggerId}
        variant="ghost"
        role="combobox"
        aria-label={ariaLabel}
        aria-expanded={open}
        className={cn(
          "w-full justify-between",
          !disabled && "cursor-pointer",
          touchTarget && "min-h-12",
          triggerClassName,
        )}
        disabled={disabled}
        data-testid={testId}
      >
        <div className="flex min-w-0 flex-1 items-center">
          <TriggerLabel
            selectedOption={options.find((option) => option.value === value)}
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
  );
}

export const Combobox = memo(function Combobox({
  options,
  value,
  onValueChange,
  ariaLabel,
  dropdownLabel,
  placeholder = t("common:selectOption"),
  searchPlaceholder = t("common:searchPlaceholder"),
  emptyMessage = t("common:noOptionFound"),
  disabled = false,
  className,
  triggerClassName,
  showSearch = true,
  testId,
  dropdownTestId,
  popoverSide,
  popoverAlign = "start",
  popoverPortal = false,
  plainTrigger = false,
  filter,
  headerAction,
  loading = false,
  touchTarget = false,
  triggerId,
  triggerRef,
}: ComboboxProps) {
  const [open, setOpen] = useState(false);
  const portalContainer = useTaskCreateDialogPopoverContainer();
  // Keep the selected value highlighted when the popover opens, not the first item.
  const [highlighted, setHighlighted] = useState("");

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) setHighlighted(value);
      }}
    >
      <ComboboxTrigger
        options={options}
        value={value}
        ariaLabel={ariaLabel}
        open={open}
        disabled={disabled}
        touchTarget={touchTarget}
        triggerClassName={triggerClassName}
        plainTrigger={plainTrigger}
        placeholder={placeholder}
        loading={loading}
        testId={testId}
        triggerId={triggerId}
        triggerRef={triggerRef}
      />
      <PopoverContent
        className={cn(
          "w-[var(--radix-popover-trigger-width)] min-w-[min(300px,calc(100vw-2rem))] max-w-[calc(100vw-2rem)] p-0 max-h-[var(--radix-popover-content-available-height)] pointer-events-auto",
          className,
        )}
        side={popoverSide}
        align={popoverAlign}
        portal={popoverPortal}
        portalContainer={portalContainer}
        onWheel={(event) => event.stopPropagation()}
      >
        <Command
          value={highlighted}
          onValueChange={setHighlighted}
          filter={filter}
          data-testid={dropdownTestId}
        >
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
              touchTarget={touchTarget}
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
