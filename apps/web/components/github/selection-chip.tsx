"use client";

import { IconPlus, IconX } from "@tabler/icons-react";
import { forwardRef } from "react";
import { Badge } from "@kandev/ui/badge";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { cn } from "@/lib/utils";

/** A selected value with a touch-sized remove button. */
export function SelectionChip({
  label,
  removeLabel,
  testId,
  disabled,
  onRemove,
}: {
  label: string;
  removeLabel: string;
  testId: string;
  disabled: boolean;
  onRemove: () => void;
}) {
  return (
    <Badge
      variant="secondary"
      className={cn("gap-1 pr-0.5 text-xs", disabled && "opacity-50")}
      data-testid={testId}
    >
      <span>{label}</span>
      <button
        type="button"
        className={controlSizingClassName(
          "icon",
          "inline-flex items-center justify-center rounded-sm cursor-pointer hover:text-foreground",
        )}
        aria-label={removeLabel}
        onClick={onRemove}
        disabled={disabled}
      >
        <IconX className="h-3 w-3" />
      </button>
    </Badge>
  );
}

type SelectionAddButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  label: string;
  testId: string;
};

/** "Add …" trigger that follows a list of selection chips. */
export const SelectionAddButton = forwardRef<HTMLButtonElement, SelectionAddButtonProps>(
  function SelectionAddButton({ label, testId, disabled, className, ...props }, ref) {
    return (
      <button
        ref={ref}
        type="button"
        disabled={disabled}
        data-testid={testId}
        className={cn(
          controlSizingClassName("standard"),
          "inline-flex items-center justify-center gap-1.5 rounded-md px-2 text-xs text-muted-foreground",
          disabled
            ? "cursor-not-allowed opacity-40"
            : "cursor-pointer hover:bg-muted hover:text-foreground",
          className,
        )}
        {...props}
      >
        <IconPlus className="h-3.5 w-3.5" />
        <span>{label}</span>
      </button>
    );
  },
);
