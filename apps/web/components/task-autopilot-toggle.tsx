"use client";

import { useState } from "react";
import { IconInfoCircle, IconRobot } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

export function TaskAutopilotToggle({
  checked,
  onCheckedChange,
  disabled,
}: {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const [helpOpen, setHelpOpen] = useState(false);

  return (
    <div
      className={cn(
        "inline-flex h-11 w-fit max-w-full items-center gap-1.5 whitespace-nowrap rounded-md border px-2 py-0 sm:h-7",
        checked ? "border-yellow-500/40 bg-yellow-500/5" : "border-border/60",
      )}
      data-testid="autopilot-toggle-row"
    >
      <div className="flex min-w-0 items-center gap-1.5">
        <IconRobot
          className={cn(
            "h-3.5 w-3.5 shrink-0",
            checked ? "text-yellow-500" : "text-muted-foreground",
          )}
          aria-hidden="true"
        />
        <label htmlFor="task-autopilot-toggle" className="cursor-pointer text-xs font-medium">
          {t("task:autopilot")}
        </label>
        <Tooltip open={helpOpen} onOpenChange={setHelpOpen}>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              className="relative size-11 shrink-0 cursor-help text-muted-foreground after:absolute after:-inset-1 hover:text-foreground focus-visible:text-foreground sm:size-7"
              aria-label={t("task:autopilotInfoLabel")}
              onClick={() => setHelpOpen((current) => !current)}
              data-testid="autopilot-info"
            >
              <IconInfoCircle className="h-3.5 w-3.5" aria-hidden="true" />
            </Button>
          </TooltipTrigger>
          <TooltipContent className="max-w-[280px] text-xs leading-relaxed">
            {t("task:autopilotDescription")}
          </TooltipContent>
        </Tooltip>
      </div>
      <Switch
        id="task-autopilot-toggle"
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
        aria-label={t("task:autopilot")}
        className="shrink-0"
      />
    </div>
  );
}
