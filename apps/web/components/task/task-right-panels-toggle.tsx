"use client";

import { IconLayoutSidebarRightCollapse, IconLayoutSidebarRightExpand } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { SurfaceAction } from "@/components/actions/surface-action";
import { useTaskRightPanelsToggle } from "@/hooks/use-task-right-panels-toggle";
import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";

type TaskRightPanelsToggleProps = {
  sessionId?: string | null;
};

export function TaskRightPanelsToggle({ sessionId = null }: TaskRightPanelsToggleProps) {
  const { t } = useTranslation();
  const { isSupported, isReady, isMaximized, isAvailable, rightPanelsVisible, toggleRightPanels } =
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
    label = t("task:rightPaneUnavailableWhileMaximized");
  } else if (!isAvailable) {
    label = t("task:rightPaneUnavailable");
  } else if (rightPanelsVisible) {
    label = t("task:hideRightPane");
  } else {
    label = t("task:showRightPane");
  }

  const canActivate = isReady && isAvailable;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          tabIndex={canActivate ? -1 : 0}
          aria-label={canActivate ? undefined : label}
          className="inline-flex"
        >
          <SurfaceAction
            surface="topbar"
            presentation="desktop"
            label={canActivate ? label : undefined}
            title={label}
            icon={
              rightPanelsVisible ? (
                <IconLayoutSidebarRightCollapse className="h-3.5 w-3.5" />
              ) : (
                <IconLayoutSidebarRightExpand className="h-3.5 w-3.5" />
              )
            }
            data-testid="task-right-panels-toggle"
            ref={buttonRef}
            aria-expanded={rightPanelsVisible}
            disabled={!canActivate}
            onClick={() => {
              restoreFocusRef.current = document.activeElement === buttonRef.current;
              toggleRightPanels();
            }}
          />
        </span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
