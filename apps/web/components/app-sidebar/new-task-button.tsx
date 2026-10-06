"use client";

import { IconPlus } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";

export function NewTaskButton({
  collapsed = false,
  disabled,
  onClick,
  className,
  testId = "create-task-button",
}: {
  collapsed?: boolean;
  disabled?: boolean;
  onClick: () => void;
  className?: string;
  testId?: string;
}) {
  const { t } = useTranslation();
  const label = t("sidebar:newTask");
  const button = (
    <Button
      variant="outline"
      onClick={onClick}
      disabled={disabled}
      aria-label={label}
      data-testid={testId}
      className={cn(
        "border-border/70 bg-muted/50 text-foreground shadow-none hover:bg-muted dark:bg-muted/50 dark:hover:bg-muted",
        collapsed
          ? "mx-auto size-9 [@media(pointer:coarse)]:size-11"
          : "h-11 w-full gap-2.5 px-3 text-sm [&_svg]:size-4.5",
        className,
      )}
    >
      <IconPlus aria-hidden="true" />
      {!collapsed && <span className="truncate">{label}</span>}
    </Button>
  );
  if (!collapsed) return button;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  );
}
