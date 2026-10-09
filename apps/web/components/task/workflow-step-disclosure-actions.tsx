"use client";

import { Button } from "@kandev/ui/button";
import { IconChevronDown, IconArrowRight } from "@tabler/icons-react";
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
        data-testid={`workflow-step-disclosure-move-${stepId}`}
        size="sm"
        variant="outline"
        className={cn("shrink-0 cursor-pointer rounded-sm px-2.5 text-xs", buttonSizeClass)}
        disabled={movePending}
        onClick={() => void onSubmit()}
      >
        {isMoving ? t("task:moving") : t("task:moveHere")}
        <IconArrowRight className="h-3 w-3" />
        {showOptions && (
          <kbd className="hidden self-center font-sans text-[10px] leading-none opacity-60 [@media(pointer:fine)]:inline">
            {workflowMoveShortcutLabel()}
          </kbd>
        )}
      </Button>
      <Button
        type="button"
        data-testid={`workflow-step-disclosure-options-${stepId}`}
        size="sm"
        variant="ghost"
        aria-expanded={showOptions}
        aria-label={t("task:workflowMoveOptions")}
        className={cn(
          "w-7 shrink-0 cursor-pointer rounded-sm p-0 text-muted-foreground [@media(pointer:coarse)]:min-w-11",
          buttonSizeClass,
          showOptions && "bg-muted/60 text-foreground",
        )}
        onClick={onToggleOptions}
      >
        <IconChevronDown
          className={cn("h-3.5 w-3.5 transition-transform", showOptions && "rotate-180")}
        />
      </Button>
    </div>
  );
}
