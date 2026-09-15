"use client";

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
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import type { PillAction, PillOption, PillProps } from "@/components/task-create-dialog-pill";

function PillCommandList({
  options,
  value,
  onSelect,
  onPointerSelect,
  setOpen,
  emptyMessage,
  touchTarget,
}: {
  options: PillOption[];
  value: string;
  onSelect: (value: string) => void;
  onPointerSelect: (pointerType: string) => void;
  setOpen: (open: boolean) => void;
  emptyMessage: string;
  touchTarget?: boolean;
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
                    touchTarget && "min-h-11",
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

export function PillPickerContent({
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
  action,
  popoverHeader,
  touchTarget,
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
  action?: PillAction;
  popoverHeader?: React.ReactNode;
  touchTarget: boolean;
}) {
  return (
    <>
      {popoverHeader}
      <Command filter={filter}>
        <div className="flex min-h-11 items-center gap-1 px-2 pt-1">
          <div className="min-w-0 flex-1">
            <CommandInput
              placeholder={searchPlaceholder}
              className={touchTarget ? "h-11 w-full" : "h-9 w-full"}
            />
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
                  className={`${controlSizingClassName("icon")} max-md:min-h-12 max-md:min-w-12 [@media(pointer:coarse)]:min-h-12 [@media(pointer:coarse)]:min-w-12 inline-flex shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer`}
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
          touchTarget={touchTarget}
        />
      </Command>
    </>
  );
}

type PillSurfaceProps = {
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
  action?: PillAction;
  popoverHeader?: React.ReactNode;
  dropdownTestId?: string;
};

export function PillPopover({
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
}: PillSurfaceProps & { portalContainer: HTMLElement | null }) {
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
      <PopoverContent
        className="w-[min(480px,calc(100vw-2rem))] p-0"
        align="start"
        portalContainer={portalContainer}
        data-testid={dropdownTestId}
      >
        <PillPickerContent
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
          action={action}
          popoverHeader={popoverHeader}
          touchTarget={false}
        />
      </PopoverContent>
    </Popover>
  );
}

export function PillDrawer({
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
  action,
  popoverHeader,
  dropdownTestId,
  mobileTitle,
}: PillSurfaceProps & { mobileTitle: string }) {
  return (
    <>
      {triggerButton}
      <MobilePickerSheet
        open={open}
        onOpenChange={setOpen}
        title={mobileTitle}
        contentTestId={dropdownTestId}
      >
        <PillPickerContent
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
          action={action}
          popoverHeader={popoverHeader}
          touchTarget
        />
      </MobilePickerSheet>
    </>
  );
}
