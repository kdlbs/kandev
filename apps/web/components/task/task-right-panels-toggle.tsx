"use client";

import { IconLayoutSidebarRightCollapse, IconLayoutSidebarRightExpand } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskRightPanelsToggle } from "@/hooks/use-task-right-panels-toggle";
import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";

type TaskRightPanelsToggleProps = {
  sessionId?: string | null;
};

export function TaskRightPanelsToggle({ sessionId = null }: TaskRightPanelsToggleProps) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const { isSupported, isReady, isMaximized, rightPanelsVisible, toggleRightPanels } =
    useTaskRightPanelsToggle(sessionId);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const restoreFocusRef = useRef(false);
  const previousReadyRef = useRef(isReady);

  useEffect(() => {
    if (isReady && !previousReadyRef.current && restoreFocusRef.current) {
      buttonRef.current?.focus();
      restoreFocusRef.current = false;
    }
    previousReadyRef.current = isReady;
  }, [isReady]);

  if (!isSupported) return null;

  let label: string;
  if (isMaximized) {
    label = t("task:rightPanelsUnavailableWhileMaximized");
  } else if (rightPanelsVisible) {
    label = t("task:hideRightPanels");
  } else {
    label = t("task:showRightPanels");
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          className={`cursor-pointer text-muted-foreground hover:bg-muted/70 hover:text-foreground ${
            isFinePointer ? "" : "min-h-11 min-w-11"
          }`}
          data-testid="task-right-panels-toggle"
          ref={buttonRef}
          aria-label={label}
          aria-expanded={rightPanelsVisible}
          title={label}
          disabled={!isReady}
          onClick={() => {
            restoreFocusRef.current = document.activeElement === buttonRef.current;
            toggleRightPanels();
          }}
        >
          {rightPanelsVisible ? (
            <IconLayoutSidebarRightCollapse className="h-3.5 w-3.5" />
          ) : (
            <IconLayoutSidebarRightExpand className="h-3.5 w-3.5" />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
