"use client";

import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useTranslation } from "react-i18next";

/**
 * Current failed-turn alert dismissal. The owning action group supplies the
 * callback and shared busy state so continuation and dismissal cannot overlap.
 */
export function TaskTurnErrorDismissButton({
  disabled = false,
  onClick,
}: {
  disabled?: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Button
      variant="outline"
      size="default"
      className={`w-full gap-1.5 whitespace-normal break-words text-xs md:w-auto ${controlSizingClassName("standard", "max-md:min-h-12 [@media(pointer:coarse)]:min-h-12")}`}
      disabled={disabled}
      onClick={onClick}
      data-testid="turn-error-dismiss-button"
    >
      {t("task:dismissTurnError")}
    </Button>
  );
}
