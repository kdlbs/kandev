"use client";

import { IconArrowDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

const BUTTON_CLASS =
  "p-0 cursor-pointer text-muted-foreground hover:bg-muted/70 hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring";

export function JumpToLatestButton({
  isVisible,
  onClick,
}: {
  isVisible: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  if (!isVisible) return null;
  const label = t("task:jumpToLatest");

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={onClick}
          aria-label={label}
          data-testid="jump-to-latest-button"
          className={cn(BUTTON_CLASS, isFinePointer ? "h-6 w-6" : "h-11 w-11")}
        >
          <IconArrowDown className="h-3.5 w-3.5" />
        </Button>
      </TooltipTrigger>
      <TooltipContent side="top">{label}</TooltipContent>
    </Tooltip>
  );
}
