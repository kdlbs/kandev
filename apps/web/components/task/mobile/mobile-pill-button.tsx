"use client";

import { forwardRef, type ReactNode } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

type MobilePillButtonProps = {
  /** Optional icon to lead with (e.g. folder, terminal). */
  icon?: ReactNode;
  /** Primary visible label. Hidden when `compact` is true. */
  label: string;
  /** Optional count badge ("3", "2/4", etc.) shown after the label. */
  count?: string | number;
  /** Hide the label, keeping just icon + count + chevron. For tight viewports. */
  compact?: boolean;
  /** Stretch the pill to fill its parent's width and left-align the label.
   * Use when the pill owns the entire header bar with no other content. */
  fullWidth?: boolean;
  /** Whether the picker this opens is currently open. Drives aria-expanded. */
  isOpen?: boolean;
  /** Tap handler — opens the associated picker sheet. */
  onClick: () => void;
  /** Stable test id. */
  "data-testid"?: string;
  /** Accessible label override. Defaults to `label`. */
  ariaLabel?: string;
};

/**
 * Header trigger for a bottom-sheet picker. Pills look like buttons (background
 * + chevron) so users see them as interactive, not a passive status badge.
 */
export const MobilePillButton = forwardRef<HTMLButtonElement, MobilePillButtonProps>(
  function MobilePillButton(
    { icon, label, count, compact, fullWidth, isOpen, onClick, ariaLabel, ...rest },
    ref,
  ) {
    return (
      <Button
        ref={ref}
        type="button"
        variant="outline"
        size="sm"
        className={`h-8 px-3 gap-2 cursor-pointer ${fullWidth ? "w-full justify-between" : ""}`}
        aria-label={ariaLabel ?? label}
        aria-haspopup="dialog"
        aria-expanded={isOpen ?? false}
        title={label}
        onClick={onClick}
        data-testid={rest["data-testid"]}
      >
        <span className="flex items-center gap-2 min-w-0">
          {icon}
          {!compact && (
            <span className={`text-xs truncate ${fullWidth ? "" : "max-w-[120px]"}`}>{label}</span>
          )}
          {count !== undefined && count !== "" && (
            <span className="text-[10px] font-medium px-1 py-0.5 rounded bg-foreground/10 text-muted-foreground leading-none shrink-0">
              {count}
            </span>
          )}
        </span>
        <IconChevronDown className="h-3.5 w-3.5 shrink-0 opacity-70" />
      </Button>
    );
  },
);
