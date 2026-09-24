"use client";

import { useCallback, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTaskCreateDialogPortalContainer } from "@/hooks/use-task-create-dialog-popover-container";
import { usePillTooltipSuppression } from "@/hooks/use-pill-tooltip-suppression";
import { useTooltipMountGate } from "@/hooks/use-tooltip-mount-gate";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { PillDrawer, PillPopover } from "@/components/task-create-dialog-pill-surfaces";

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
 * `Pill` wraps cmdk's `Command` / `CommandInput` / `CommandList`. Searchable
 * content must remain cmdk children so keyboard navigation and focus stay
 * correct. Contextual controls can use `popoverHeader`, which renders outside
 * `Command`; mixed searchable content still needs a custom `Popover`.
 */
export type PillProps = {
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
  triggerClassName?: string;
  ariaLabel?: string;
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
  /** Optional contextual controls rendered above the searchable list. */
  popoverHeader?: React.ReactNode;
  /** Title used by the mobile picker sheet. */
  mobileTitle?: string;
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
function pillTriggerClass(
  disabled: boolean,
  flat: boolean,
  hasValue: boolean,
  touchTarget: boolean,
): string {
  return cn(
    "h-7 inline-flex items-center gap-1.5 rounded-md px-2.5 text-xs",
    touchTarget && "min-h-11",
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
  popoverHeader?: React.ReactNode;
  dropdownTestId?: string;
  tooltip?: string;
  tooltipOpenState: boolean;
  suppressTooltip: boolean;
  suppressTooltipRef: { current: boolean };
  handlePillTooltipOpenChange: (open: boolean) => void;
  suppressForSelection: () => void;
  usesTouchDrawer: boolean;
  mobileTitle: string;
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
  popoverHeader,
  dropdownTestId,
  tooltip,
  tooltipOpenState,
  suppressTooltip,
  suppressTooltipRef,
  handlePillTooltipOpenChange,
  suppressForSelection,
  usesTouchDrawer,
  mobileTitle,
}: PillPopoverShellProps): React.ReactElement {
  const handleSelect = (selectedValue: string) => {
    if (tooltip) suppressForSelection();
    onSelect(selectedValue);
  };
  const surface = usesTouchDrawer ? (
    <PillDrawer
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
      onSelect={handleSelect}
      emptyMessage={emptyMessage}
      action={action}
      popoverHeader={popoverHeader}
      dropdownTestId={dropdownTestId}
      mobileTitle={mobileTitle}
    />
  ) : (
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
      onSelect={handleSelect}
      emptyMessage={emptyMessage}
      portalContainer={portalContainer}
      action={action}
      popoverHeader={popoverHeader}
      dropdownTestId={dropdownTestId}
    />
  );

  if (!tooltip || usesTouchDrawer) return surface;

  const tooltipOpen =
    open || suppressTooltip || suppressTooltipRef.current ? false : tooltipOpenState;
  return (
    <Tooltip open={tooltipOpen} onOpenChange={handlePillTooltipOpenChange}>
      {surface}
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
  prefix,
  touchTarget,
  onClick,
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
  | "prefix"
> & {
  hasValue: boolean;
  touchTarget: boolean;
  onClick?: React.MouseEventHandler<HTMLButtonElement>;
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
      data-testid={testId}
      className={cn(
        pillTriggerClass(Boolean(disabled), Boolean(flat), hasValue, touchTarget),
        triggerClassName,
      )}
      onClick={onClick}
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

function pillTriggerSurface(
  triggerButton: React.ReactElement,
  usesTouchDrawer: boolean,
  tooltip?: string,
): React.ReactElement {
  if (!usesTouchDrawer && tooltip) return <TooltipTrigger asChild>{triggerButton}</TooltipTrigger>;
  return triggerButton;
}

function renderDisabledPill(
  open: boolean,
  onOpenChange: (open: boolean) => void,
  triggerButton: React.ReactNode,
  disabledReason: string,
): React.ReactElement {
  return (
    <DisabledPillTooltip
      open={open}
      onOpenChange={onOpenChange}
      triggerButton={triggerButton}
      disabledReason={disabledReason}
    />
  );
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
  dropdownTestId,
  onOpenChange,
  filter,
  tooltip,
  prefix,
  action,
  popoverHeader,
  mobileTitle,
}: PillProps) {
  const [open, setOpenState] = useState(false);
  const { tooltipOpenState, handleTooltipOpenChange, closeTooltip } = useTooltipMountGate();
  const portalContainer = useTaskCreateDialogPortalContainer();
  const {
    suppressTooltip,
    suppressTooltipRef,
    suppressTooltipUntilLeave,
    handlePointerEnter,
    handlePointerLeave,
    handleBlur,
  } = usePillTooltipSuppression(open);
  const usesTouchDrawer = useTouchDrawer();
  const { setOpen, suppressForSelection, recordPointerSelection } = usePillOpenHandlers(
    setOpenState,
    closeTooltip,
    suppressTooltipUntilLeave,
    onOpenChange,
  );
  const handleTooltipChange = useTooltipOpenChange(suppressTooltipRef, handleTooltipOpenChange);
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
    prefix,
    touchTarget: usesTouchDrawer,
    onClick: usesTouchDrawer ? () => setOpen(true) : undefined,
    onPointerEnter: tooltip ? handlePointerEnter : undefined,
    onPointerLeave: tooltip ? handlePointerLeave : undefined,
    onBlur: tooltip ? handleBlur : undefined,
  });
  if (disabled && disabledReason && !open)
    return renderDisabledPill(
      tooltipOpenState,
      handleTooltipOpenChange,
      triggerButton,
      disabledReason,
    );

  return renderPillPopover({
    open,
    setOpen,
    triggerButton: pillTriggerSurface(triggerButton, usesTouchDrawer, tooltip),
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
    popoverHeader,
    dropdownTestId,
    tooltip,
    tooltipOpenState,
    suppressTooltip,
    suppressTooltipRef,
    handlePillTooltipOpenChange: handleTooltipChange,
    suppressForSelection,
    usesTouchDrawer,
    mobileTitle: mobileTitle ?? placeholder,
  });
}
