"use client";

import { IconStar } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type SavedQueryDefaultButtonProps = {
  label: string;
  isDefault: boolean;
  disabled?: boolean;
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
  const accessibleLabel = t(
    isDefault
      ? "integrations:clearSavedQueryAsDefaultView"
      : "integrations:setSavedQueryAsDefaultView",
    { label },
  );
  // Desktop instances sit inside a Radix menu item, so stop the complete pointer sequence;
  // the same handlers are harmless for the standalone mobile-sidebar button.
  return (
    <button
      type="button"
      aria-label={accessibleLabel}
      aria-pressed={isDefault}
      title={accessibleLabel}
      disabled={disabled}
      data-testid={testId}
      onPointerDown={(event) => event.stopPropagation()}
      onPointerUp={(event) => event.stopPropagation()}
      onClick={(event) => {
        event.stopPropagation();
        onToggle();
      }}
      className={cn(
        "flex shrink-0 cursor-pointer items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-wait disabled:opacity-50",
        size === "mobile" ? "h-11 w-11" : "h-6 w-6",
      )}
    >
      <IconStar className={cn("h-4 w-4", isDefault && "fill-amber-500 text-amber-500")} />
    </button>
  );
}
