"use client";

import { useCallback, useRef, useState } from "react";
import { IconCheck } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { prioritizeSelectedOption, selectorOptionClassName } from "@/lib/utils/selector-options";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { BranchRefreshButton } from "@/components/branch-refresh-button";
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import { usePillTooltipSuppression } from "@/hooks/use-pill-tooltip-suppression";
import { useTooltipMountGate } from "@/hooks/use-tooltip-mount-gate";

export type PillOption = {
  value: string;
  label: string;
  keywords?: string[];
  renderLabel?: () => React.ReactNode;
  renderAccessory?: () => React.ReactNode;
  group?: string;
  groupLabel?: string;
  disabled?: boolean;
  disabledReason?: string;
};

export type PillAction = {
  label: string;
  icon?: React.ReactNode;
  onSelect: () => void;
};

/**
 * `Pill` wraps cmdk's `Command` / `CommandInput` / `CommandList`. Its popover
 * body only supports cmdk children (`CommandItem`, etc.) — keyboard nav and
 * focus are routed through cmdk. If you need a popover with mixed content
 * (search list + a free-form `<input>`, banners, etc.), build a custom
 * `Popover` from `@kandev/ui/popover` instead of warping `Pill`.
 */

type PillProps = {
  icon: React.ReactNode;
  value: string;
  /** Option value used for selected-first ordering when the trigger label differs. */
  selectedValue?: string;
  placeholder: string;
  options: PillOption[];
  onSelect: (value: string) => void;
  disabled?: boolean;
  /** When provided alongside `disabled`, surfaces a tooltip explaining why. */
  disabledReason?: string;
  searchPlaceholder: string;
  emptyMessage: string;
  testId?: string;
  /** Optional refresh action rendered next to the search input. */
  onRefresh?: () => void;
  /** Show the refresh icon as spinning + disabled while a refresh is in flight. */
  refreshing?: boolean;
  /** Accessible label used for the optional refresh action. */
  refreshLabel?: string;
  /**
   * Render without its own border/bg so the pill blends into a wrapping
   * grouped container (used by RepoChip to draw one rectangle around
   * repo + branch + remove).
   */
  flat?: boolean;
  /** Optional cmdk scorer override. Branch pickers pass `scoreBranch`. */
  filter?: (value: string, search: string, keywords?: string[]) => number;
  /** Optional hover tooltip for truncated labels or extra context. */
  tooltip?: string;
  /**
   * Optional muted prefix shown before the value in the trigger button.
   * Used by the branch chip to distinguish "current: <branch>" (no-op),
   * "will switch to: <branch>" (destructive) and "from: <branch>" (worktree
   * base) without depending on the user reading a tooltip.
   */
  prefix?: string;
  /** Optional icon action rendered beside the search input. */
  action?: PillAction;
};

/** Returns the active-state hover classes for the pill trigger button. */
function pillActiveClass(flat: boolean): string {
  if (flat) return "hover:bg-muted/60 cursor-pointer";
  return "hover:bg-muted hover:border-border cursor-pointer";
}

function PillCommandList({
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

/**
 * Builds the className for the pill trigger button. Extracted so the inline
 * trigger JSX stays compact (the Pill function is right at the complexity cap).
 */
function pillTriggerClass(disabled: boolean, flat: boolean, hasValue: boolean): string {
  return cn(
    "h-7 inline-flex items-center gap-1.5 rounded-md px-2.5 text-xs",
    flat ? "bg-transparent" : "border border-border/60 bg-muted/30",
    disabled ? "opacity-50 cursor-not-allowed" : pillActiveClass(flat),
    !hasValue && "text-muted-foreground",
  );
}

function DisabledPillTooltip({
  open,
  onOpenChange,
  triggerButton,
  disabledReason,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  triggerButton: React.ReactNode;
  disabledReason: string;
}) {
  return (
    <Tooltip open={open} onOpenChange={onOpenChange}>
      <TooltipTrigger asChild>
        <span className="inline-flex" tabIndex={0} aria-label={disabledReason}>
          <span aria-hidden="true" className="inline-flex">
            {triggerButton}
          </span>
        </span>
      </TooltipTrigger>
      <TooltipContent>{disabledReason}</TooltipContent>
    </Tooltip>
  );
}

function renderDisabledPillTooltip(
  tooltipOpenState: boolean,
  onOpenChange: (open: boolean) => void,
  triggerButton: React.ReactNode,
  disabledReason: string,
): React.ReactElement {
  return (
    <DisabledPillTooltip
      open={tooltipOpenState}
      onOpenChange={onOpenChange}
      triggerButton={triggerButton}
      disabledReason={disabledReason}
    />
  );
}

function PillPopoverContent({
  filter,
  searchPlaceholder,
  onRefresh,
  refreshing,
  refreshLabel,
  options,
  value,
  onSelect,
  onPointerSelect,
  setOpen,
  emptyMessage,
  portalContainer,
  action,
}: {
  filter?: PillProps["filter"];
  searchPlaceholder: string;
  onRefresh?: () => void;
  refreshing?: boolean;
  refreshLabel?: string;
  options: PillOption[];
  value: string;
  onSelect: (value: string) => void;
  onPointerSelect: (pointerType: string) => void;
  setOpen: (open: boolean) => void;
  emptyMessage: string;
  portalContainer: HTMLElement | null;
  action?: PillAction;
}) {
  return (
    <PopoverContent
      className="w-[min(480px,calc(100vw-2rem))] p-0"
      align="start"
      portalContainer={portalContainer}
    >
      <Command filter={filter}>
        <div className="flex min-h-11 items-center gap-1 px-2 pt-1">
          <div className="min-w-0 flex-1">
            <CommandInput placeholder={searchPlaceholder} className="h-9 w-full" />
          </div>
          {onRefresh ? (
            <BranchRefreshButton
              onRefresh={onRefresh}
              refreshing={refreshing}
              label={refreshLabel}
              testId={refreshLabel === "repositories" ? "repo-refresh-button" : undefined}
              touchTarget={refreshLabel === "repositories"}
            />
          ) : null}
          {action ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  aria-label={action.label}
                  data-testid="create-local-repository-button"
                  onClick={() => {
                    action.onSelect();
                    setOpen(false);
                  }}
                  className="inline-flex h-12 w-12 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer"
                >
                  {action.icon}
                </button>
              </TooltipTrigger>
              <TooltipContent>{action.label}</TooltipContent>
            </Tooltip>
          ) : null}
        </div>
        <PillCommandList
          options={options}
          value={value}
          onSelect={onSelect}
          onPointerSelect={onPointerSelect}
          setOpen={setOpen}
          emptyMessage={emptyMessage}
        />
      </Command>
    </PopoverContent>
  );
}

function PillPopover({
  open,
  setOpen,
  triggerButton,
  filter,
  searchPlaceholder,
  onRefresh,
  refreshing,
  refreshLabel,
  options,
  value,
  onSelect,
  onPointerSelect,
  emptyMessage,
  portalContainer,
  action,
}: {
  open: boolean;
  setOpen: (open: boolean) => void;
  triggerButton: React.ReactElement;
  filter?: PillProps["filter"];
  searchPlaceholder: string;
  onRefresh?: () => void;
  refreshing?: boolean;
  refreshLabel?: string;
  options: PillOption[];
  value: string;
  onSelect: (value: string) => void;
  onPointerSelect: (pointerType: string) => void;
  emptyMessage: string;
  portalContainer: HTMLElement | null;
  action?: PillAction;
}) {
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
      <PillPopoverContent
        filter={filter}
        searchPlaceholder={searchPlaceholder}
        onRefresh={onRefresh}
        refreshing={refreshing}
        refreshLabel={refreshLabel}
        options={options}
        value={value}
        onSelect={onSelect}
        onPointerSelect={onPointerSelect}
        setOpen={setOpen}
        emptyMessage={emptyMessage}
        portalContainer={portalContainer}
        action={action}
      />
    </Popover>
  );
}

type PillPopoverShellProps = {
  open: boolean;
  setOpen: (open: boolean) => void;
  triggerButton: React.ReactElement;
  filter?: PillProps["filter"];
  searchPlaceholder: string;
  onRefresh?: () => void;
  refreshing?: boolean;
  refreshLabel?: string;
  options: PillOption[];
  value: string;
  onSelect: (value: string) => void;
  onPointerSelect: (pointerType: string) => void;
  emptyMessage: string;
  portalContainer: HTMLElement | null;
  action?: PillAction;
  tooltip?: string;
  tooltipOpenState: boolean;
  suppressTooltip: boolean;
  suppressTooltipRef: { current: boolean };
  handlePillTooltipOpenChange: (open: boolean) => void;
  suppressForSelection: () => void;
};

function renderPillPopover({
  open,
  setOpen,
  triggerButton,
  filter,
  searchPlaceholder,
  onRefresh,
  refreshing,
  refreshLabel,
  options,
  value,
  onSelect,
  onPointerSelect,
  emptyMessage,
  portalContainer,
  action,
  tooltip,
  tooltipOpenState,
  suppressTooltip,
  suppressTooltipRef,
  handlePillTooltipOpenChange,
  suppressForSelection,
}: PillPopoverShellProps): React.ReactElement {
  const popover = (
    <PillPopover
      open={open}
      setOpen={setOpen}
      triggerButton={triggerButton}
      filter={filter}
      searchPlaceholder={searchPlaceholder}
      onRefresh={onRefresh}
      refreshing={refreshing}
      refreshLabel={refreshLabel}
      options={options}
      value={value}
      onPointerSelect={onPointerSelect}
      onSelect={(selectedValue) => {
        if (tooltip) suppressForSelection();
        onSelect(selectedValue);
      }}
      emptyMessage={emptyMessage}
      portalContainer={portalContainer}
      action={action}
    />
  );

  if (!tooltip) return popover;

  const tooltipOpen =
    open || suppressTooltip || suppressTooltipRef.current ? false : tooltipOpenState;
  return (
    <Tooltip open={tooltipOpen} onOpenChange={handlePillTooltipOpenChange}>
      {popover}
      <TooltipContent className="max-w-[calc(100vw-2rem)] break-all">{tooltip}</TooltipContent>
    </Tooltip>
  );
}

function renderPillTriggerButton({
  icon,
  value,
  placeholder,
  disabled,
  flat,
  hasValue,
  testId,
  prefix,
  onPointerEnter,
  onPointerLeave,
  onBlur,
}: Pick<PillProps, "icon" | "value" | "placeholder" | "disabled" | "flat" | "testId" | "prefix"> & {
  hasValue: boolean;
  onPointerEnter?: React.PointerEventHandler<HTMLButtonElement>;
  onPointerLeave?: React.PointerEventHandler<HTMLButtonElement>;
  onBlur?: React.FocusEventHandler<HTMLButtonElement>;
}): React.ReactElement {
  const showPrefix = !!prefix && hasValue;
  return (
    <button
      type="button"
      disabled={disabled}
      data-testid={testId}
      className={pillTriggerClass(Boolean(disabled), Boolean(flat), hasValue)}
      onPointerEnter={onPointerEnter}
      onPointerLeave={onPointerLeave}
      onBlur={onBlur}
    >
      {icon}
      <span className="truncate max-w-[240px]">
        {showPrefix && <span className="text-muted-foreground">{prefix}</span>}
        {value || placeholder}
      </span>
    </button>
  );
}

function usePillOpenHandlers(
  setOpenState: React.Dispatch<React.SetStateAction<boolean>>,
  closeTooltip: () => void,
  suppressTooltipUntilLeave: (releaseOnExit?: boolean) => void,
) {
  const selectionPointerTypeRef = useRef("");
  const suppressForSelection = useCallback(() => {
    suppressTooltipUntilLeave(selectionPointerTypeRef.current !== "touch");
  }, [suppressTooltipUntilLeave]);
  const setOpen = useCallback(
    (next: boolean) => {
      if (next) {
        closeTooltip();
        selectionPointerTypeRef.current = "";
      } else {
        suppressForSelection();
      }
      setOpenState(next);
    },
    [closeTooltip, setOpenState, suppressForSelection],
  );
  const recordPointerSelection = useCallback((pointerType: string) => {
    selectionPointerTypeRef.current = pointerType;
  }, []);
  return { setOpen, suppressForSelection, recordPointerSelection };
}

/**
 * Compact pill trigger that opens a popover with a search list. Auto-widths
 * to its content (no `w-full`, no chevron) so multiple pills can sit on one
 * line without overlapping or stretching to fill the row.
 */
export function Pill({
  icon,
  value,
  selectedValue,
  placeholder,
  options,
  onSelect,
  disabled = false,
  disabledReason,
  searchPlaceholder,
  emptyMessage,
  testId,
  onRefresh,
  refreshing,
  refreshLabel,
  flat = false,
  filter,
  tooltip,
  prefix,
  action,
}: PillProps) {
  const [open, setOpenState] = useState(false);
  const { tooltipOpenState, handleTooltipOpenChange, closeTooltip } = useTooltipMountGate();
  const portalContainer = useTaskCreateDialogPopoverContainer();
  const {
    suppressTooltip,
    suppressTooltipRef,
    suppressTooltipUntilLeave,
    handlePointerEnter,
    handlePointerLeave,
    handleBlur,
  } = usePillTooltipSuppression(open);
  const { setOpen, suppressForSelection, recordPointerSelection } = usePillOpenHandlers(
    setOpenState,
    closeTooltip,
    suppressTooltipUntilLeave,
  );
  const handlePillTooltipOpenChange = useCallback(
    (next: boolean) => {
      if (next && suppressTooltipRef.current) return;
      handleTooltipOpenChange(next);
    },
    [handleTooltipOpenChange],
  );
  const triggerButton = renderPillTriggerButton({
    icon,
    value,
    placeholder,
    disabled,
    flat,
    hasValue: Boolean(value),
    testId,
    prefix,
    onPointerEnter: tooltip ? handlePointerEnter : undefined,
    onPointerLeave: tooltip ? handlePointerLeave : undefined,
    onBlur: tooltip ? handleBlur : undefined,
  });
  // Disabled buttons swallow events, so the wrapper owns tooltip focus.
  if (disabled && disabledReason && !open) {
    return renderDisabledPillTooltip(
      tooltipOpenState,
      handleTooltipOpenChange,
      triggerButton,
      disabledReason,
    );
  }

  return renderPillPopover({
    open,
    setOpen,
    triggerButton: tooltip ? (
      <TooltipTrigger asChild>{triggerButton}</TooltipTrigger>
    ) : (
      triggerButton
    ),
    filter,
    searchPlaceholder,
    onRefresh,
    refreshing,
    refreshLabel,
    options,
    value: selectedValue ?? value,
    onPointerSelect: recordPointerSelection,
    onSelect,
    emptyMessage,
    portalContainer,
    action,
    tooltip,
    tooltipOpenState,
    suppressTooltip,
    suppressTooltipRef,
    handlePillTooltipOpenChange,
    suppressForSelection,
  });
}
