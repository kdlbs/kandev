"use client";

import { IconStar } from "@tabler/icons-react";
import { DropdownMenuCheckboxItem } from "@kandev/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type SavedQueryDefaultButtonProps = {
  label: string;
  isDefault: boolean;
  disabled?: boolean;
  /** `desktop` must render inside Radix `DropdownMenuContent`; `mobile` is a plain button. */
  size: "desktop" | "mobile";
  testId?: string;
  onToggle: () => void;
};

export function SavedQueryDefaultButton({
  label,
  isDefault,
  disabled = false,
  size,
  testId,
  onToggle,
}: SavedQueryDefaultButtonProps) {
  const { t } = useTranslation();
  const actionLabel = t(
    isDefault
      ? "integrations:clearSavedQueryAsDefaultView"
      : "integrations:setSavedQueryAsDefaultView",
    { label },
  );
  const accessibleLabel = disabled
    ? t("integrations:savedQueryDefaultUpdateInProgress", { action: actionLabel })
    : actionLabel;
  const iconClassName = cn(
    "h-4 w-4",
    isDefault && "fill-amber-500 text-amber-500",
    disabled && "animate-pulse motion-reduce:animate-none",
  );
  if (size === "desktop") {
    return (
      <DropdownMenuCheckboxItem
        checked={isDefault}
        disabled={disabled}
        aria-label={accessibleLabel}
        title={accessibleLabel}
        data-testid={testId}
        onSelect={(event) => event.preventDefault()}
        onCheckedChange={() => onToggle()}
        className="h-7 min-h-7 w-7 shrink-0 cursor-pointer justify-center p-0 text-muted-foreground hover:text-foreground [&_[data-slot=dropdown-menu-checkbox-item-indicator]]:hidden"
      >
        <IconStar className={iconClassName} />
      </DropdownMenuCheckboxItem>
    );
  }

  return (
    <button
      type="button"
      aria-label={accessibleLabel}
      aria-pressed={isDefault}
      title={accessibleLabel}
      disabled={disabled}
      data-testid={testId}
      onClick={onToggle}
      className={cn(
        "flex shrink-0 cursor-pointer items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-wait disabled:opacity-50",
        "h-11 w-11",
      )}
    >
      <IconStar className={iconClassName} />
    </button>
  );
}
