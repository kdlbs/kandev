"use client";

import { useCallback, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Command, CommandInput } from "@kandev/ui/command";
import { BranchRefreshButton } from "@/components/branch-refresh-button";
import { PillCommandList } from "@/components/task-create-dialog-pill-command-list";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
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
  testId?: string;
};

/**
 * `Pill` wraps cmdk's `Command` / `CommandInput` / `CommandList`. Searchable
 * content must remain cmdk children so keyboard navigation and focus stay
 * correct. Contextual controls can use `popoverHeader`, which renders outside
 * `Command`; mixed searchable content still needs a custom `Popover`.
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
  emptyMessage: React.ReactNode;
  testId?: string;
  triggerClassName?: string;
  ariaLabel?: string;
  ariaDescribedBy?: string;
  dropdownTestId?: string;
  onOpenChange?: (open: boolean) => void;
  /** Optional refresh action rendered next to the search input. */
  onRefresh?: () => void;
  /** Show the refresh icon as spinning + disabled while a refresh is in flight. */
  refreshing?: boolean;
  /** Accessible label used for the optional refresh action. */
  refreshLabel?: string;
  /** Render without its own border/bg for a grouped repo chip. */
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
  /** Optional multiple icon actions rendered beside the search input. */
  actions?: PillAction[];
  /** Optional contextual controls rendered above the searchable list. */
  popoverHeader?: React.ReactNode;
};

/** Returns the active-state hover classes for the pill trigger button. */
function pillActiveClass(flat: boolean): string {
  if (flat) return "hover:bg-muted/60 cursor-pointer";
  return "hover:bg-muted hover:border-border cursor-pointer";
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
  actions,
  popoverHeader,
  dropdownTestId,
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
  emptyMessage: React.ReactNode;
  portalContainer: HTMLElement | null;
  action?: PillAction;
  actions?: PillAction[];
  popoverHeader?: React.ReactNode;
  dropdownTestId?: string;
}) {
  const renderedActions = actions ?? (action ? [action] : []);
  return (
    <PopoverContent
      className="w-[min(480px,calc(100vw-2rem))] p-0"
      align="start"
      portalContainer={portalContainer}
      data-testid={dropdownTestId}
    >
      {popoverHeader}
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
              touchTarget
            />
          ) : null}
          {renderedActions.map((act, index) => (
            <Tooltip key={act.label || index}>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  aria-label={act.label}
                  data-testid={act.testId ?? "create-local-repository-button"}
                  onClick={() => {
                    act.onSelect();
                    setOpen(false);
                  }}
                  className={`${controlSizingClassName("icon")} max-md:min-h-12 max-md:min-w-12 [@media(pointer:coarse)]:min-h-12 [@media(pointer:coarse)]:min-w-12 inline-flex shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer`}
                >
                  {act.icon}
                </button>
              </TooltipTrigger>
              <TooltipContent>{act.label}</TooltipContent>
            </Tooltip>
          ))}
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
  actions,
  popoverHeader,
  dropdownTestId,
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
  emptyMessage: React.ReactNode;
  portalContainer: HTMLElement | null;
  action?: PillAction;
  actions?: PillAction[];
  popoverHeader?: React.ReactNode;
  dropdownTestId?: string;
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
        actions={actions}
        popoverHeader={popoverHeader}
        dropdownTestId={dropdownTestId}
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
  emptyMessage: React.ReactNode;
  portalContainer: HTMLElement | null;
  action?: PillAction;
  actions?: PillAction[];
  popoverHeader?: React.ReactNode;
  dropdownTestId?: string;
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
  actions,
  popoverHeader,
  dropdownTestId,
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
      actions={actions}
      popoverHeader={popoverHeader}
      dropdownTestId={dropdownTestId}
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
  triggerClassName,
  ariaLabel,
  ariaDescribedBy,
  prefix,
  onPointerEnter,
  onPointerLeave,
  onBlur,
}: Pick<
  PillProps,
  | "icon"
  | "value"
  | "placeholder"
  | "disabled"
  | "flat"
  | "testId"
  | "triggerClassName"
  | "ariaLabel"
  | "ariaDescribedBy"
  | "prefix"
> & {
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
      aria-label={ariaLabel}
      aria-describedby={ariaDescribedBy}
      data-testid={testId}
      className={cn(pillTriggerClass(Boolean(disabled), Boolean(flat), hasValue), triggerClassName)}
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
  onOpenChange?: (open: boolean) => void,
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
      } else suppressForSelection();
      setOpenState(next);
      onOpenChange?.(next);
    },
    [closeTooltip, onOpenChange, setOpenState, suppressForSelection],
  );
  const recordPointerSelection = useCallback((pointerType: string) => {
    selectionPointerTypeRef.current = pointerType;
  }, []);
  return { setOpen, suppressForSelection, recordPointerSelection };
}

function useTooltipOpenChange(
  suppressTooltipRef: { current: boolean },
  handleTooltipOpenChange: (open: boolean) => void,
) {
  return useCallback(
    (next: boolean) => {
      if (next && suppressTooltipRef.current) return;
      handleTooltipOpenChange(next);
    },
    [handleTooltipOpenChange],
  );
}

function usePillState(onOpenChange?: (open: boolean) => void) {
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
    onOpenChange,
  );
  const handleTooltipChange = useTooltipOpenChange(suppressTooltipRef, handleTooltipOpenChange);

  return {
    open,
    setOpen,
    portalContainer,
    tooltipOpenState,
    suppressTooltip,
    suppressTooltipRef,
    handlePointerEnter,
    handlePointerLeave,
    handleBlur,
    suppressForSelection,
    recordPointerSelection,
    handleTooltipChange,
  };
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
  triggerClassName,
  ariaLabel,
  ariaDescribedBy,
  dropdownTestId,
  onOpenChange,
  filter,
  tooltip,
  prefix,
  action,
  actions,
  popoverHeader,
}: PillProps) {
  const state = usePillState(onOpenChange);
  const triggerButton = renderPillTriggerButton({
    icon,
    value,
    placeholder,
    disabled,
    flat,
    hasValue: Boolean(value),
    testId,
    triggerClassName,
    ariaLabel,
    ariaDescribedBy,
    prefix,
    onPointerEnter: tooltip ? state.handlePointerEnter : undefined,
    onPointerLeave: tooltip ? state.handlePointerLeave : undefined,
    onBlur: tooltip ? state.handleBlur : undefined,
  });
  // Disabled buttons swallow events, so the wrapper owns tooltip focus.
  if (disabled && disabledReason && !state.open) {
    return (
      <DisabledPillTooltip
        open={state.tooltipOpenState}
        onOpenChange={state.handleTooltipChange}
        triggerButton={triggerButton}
        disabledReason={disabledReason}
      />
    );
  }

  return renderPillPopover({
    open: state.open,
    setOpen: state.setOpen,
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
    onPointerSelect: state.recordPointerSelection,
    onSelect,
    emptyMessage,
    portalContainer: state.portalContainer,
    action,
    actions,
    popoverHeader,
    dropdownTestId,
    tooltip,
    tooltipOpenState: state.tooltipOpenState,
    suppressTooltip: state.suppressTooltip,
    suppressTooltipRef: state.suppressTooltipRef,
    handlePillTooltipOpenChange: state.handleTooltipChange,
    suppressForSelection: state.suppressForSelection,
  });
}
