"use client";

import { Button } from "@kandev/ui/button";
import { IconAdjustments, IconArrowRight } from "@tabler/icons-react";
import { cn } from "@kandev/ui/lib/utils";
import { useTranslation } from "react-i18next";
import { workflowMoveShortcutLabel } from "./use-workflow-move-submit";

type StepDisclosureRowActionsProps = {
  stepId: string;
  isMoving: boolean;
  movePending: boolean;
  showOptions: boolean;
  buttonSizeClass: string;
  onToggleOptions: () => void;
  onSubmit: () => Promise<void>;
};

export function StepDisclosureRowActions({
  stepId,
  isMoving,
  movePending,
  showOptions,
  buttonSizeClass,
  onToggleOptions,
  onSubmit,
}: StepDisclosureRowActionsProps) {
  const { t } = useTranslation();

  return (
    <div className="flex shrink-0 items-center gap-1">
      <Button
        type="button"
        data-testid={`workflow-step-disclosure-options-${stepId}`}
        size="sm"
        variant="ghost"
        aria-expanded={showOptions}
        aria-label={t("task:workflowMoveOptions")}
        className={cn(
          "shrink-0 cursor-pointer rounded-sm px-2 text-muted-foreground",
          buttonSizeClass,
          showOptions && "bg-muted/60 text-foreground",
        )}
        onClick={onToggleOptions}
      >
        <IconAdjustments className="h-3.5 w-3.5" />
      </Button>
      <Button
        type="button"
        data-testid={`workflow-step-disclosure-move-${stepId}`}
        size="sm"
        variant="default"
        className={cn("shrink-0 cursor-pointer rounded-sm px-2.5 text-xs", buttonSizeClass)}
        disabled={movePending}
        onClick={() => void onSubmit()}
      >
        <IconArrowRight className="h-3 w-3" />
        {isMoving ? t("task:moving") : t("task:moveHere")}
        {showOptions && (
          <kbd className="hidden self-center font-sans text-[10px] leading-none opacity-60 [@media(pointer:fine)]:inline">
            {workflowMoveShortcutLabel()}
          </kbd>
        )}
      </Button>
    </div>
  );
}
