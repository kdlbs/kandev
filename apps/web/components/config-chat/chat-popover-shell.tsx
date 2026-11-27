"use client";

import type { ReactNode } from "react";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent } from "@kandev/ui/popover";

type ChatPopoverShellProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  trigger: ReactNode;
  icon: ReactNode;
  title: string;
  closeLabel: string;
  headerActions?: ReactNode;
  header?: ReactNode;
  beforeBody?: ReactNode;
  testId: string;
  children: ReactNode;
  /** Forwarded to Radix `PopoverContent`: lets a caller that closes the popover to move
   *  focus somewhere specific (e.g. a deep-linked form field) override Radix's default
   *  return-focus-to-trigger behavior for that one close, without losing it for Escape/the
   *  header Close button. */
  onCloseAutoFocus?: (event: Event) => void;
};

/**
 * Shared popover chrome for Configuration chat and the coordinator popover:
 * position, size, the header row (icon, title, an actions slot, Close), a
 * floating-actions host slot, and Escape/outside-click handling. Callers
 * supply the trigger already wrapped in `PopoverTrigger`.
 */
export function ChatPopoverShell({
  open,
  onOpenChange,
  trigger,
  icon,
  title,
  closeLabel,
  headerActions,
  header,
  beforeBody,
  testId,
  children,
  onCloseAutoFocus,
}: ChatPopoverShellProps) {
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      {trigger}
      <PopoverContent
        side="top"
        align="end"
        sideOffset={8}
        onInteractOutside={(event) => event.preventDefault()}
        onCloseAutoFocus={onCloseAutoFocus}
        data-testid={testId}
        className="relative flex h-[min(550px,calc(100dvh_-_11rem_-_env(safe-area-inset-top)_-_env(safe-area-inset-bottom)))] max-h-[550px] w-[min(420px,calc(100vw_-_2rem))] flex-col gap-0 overflow-visible p-0 shadow-2xl"
      >
        {beforeBody}
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-[inherit]">
          {header ?? (
            <header className="flex h-12 shrink-0 items-center justify-between border-b bg-muted/30 pl-3">
              <div className="flex min-w-0 items-center gap-2">
                {icon}
                <span className="truncate text-sm font-medium">{title}</span>
              </div>
              <div className="flex items-center">
                {headerActions}
                <Button
                  size="icon"
                  variant="ghost"
                  className="h-11 w-11 cursor-pointer rounded-none"
                  onClick={() => onOpenChange(false)}
                  aria-label={closeLabel}
                >
                  <IconX className="h-4 w-4" />
                </Button>
              </div>
            </header>
          )}
          {children}
        </div>
      </PopoverContent>
    </Popover>
  );
}
