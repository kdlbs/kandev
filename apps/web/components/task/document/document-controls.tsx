"use client";

import { useMemo } from "react";
import { IconLayoutSidebarLeftCollapse, IconLayoutSidebarRightCollapse } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useLayoutStore } from "@/lib/state/layout-store";

type DocumentControlsProps = {
  activeSessionId: string | null;
};

export function DocumentControls({ activeSessionId }: DocumentControlsProps) {
  const toggleColumn = useLayoutStore((state) => state.toggleColumn);
  const showColumn = useLayoutStore((state) => state.showColumn);
  const toggleRightPanel = useLayoutStore((state) => state.toggleRightPanel);
  const layoutBySession = useLayoutStore((state) => state.columnsBySessionId);

  const layoutState = useMemo(() => {
    if (!activeSessionId) return null;
    return layoutBySession[activeSessionId] ?? null;
  }, [layoutBySession, activeSessionId]);

  if (!activeSessionId || !layoutState?.document) {
    return null;
  }

  const leftHidden = !layoutState.left;
  const rightHidden = !layoutState.right;

  return (
    <div className="inline-flex items-center rounded-md border border-border/70 bg-background">
      {leftHidden ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="icon-sm"
              variant="ghost"
              className="cursor-pointer rounded-none border-r border-border/70"
              onClick={() => showColumn(activeSessionId, "left")}
            >
              <IconLayoutSidebarLeftCollapse className="h-3 w-3" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Show sidebar</TooltipContent>
        </Tooltip>
      ) : (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="icon-sm"
              variant="ghost"
              className="cursor-pointer rounded-none border-r border-border/70"
              onClick={() => toggleColumn(activeSessionId, "left")}
            >
              <IconLayoutSidebarLeftCollapse className="h-3 w-3" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Hide sidebar</TooltipContent>
        </Tooltip>
      )}
      {rightHidden ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="icon-sm"
              variant="ghost"
              className="cursor-pointer rounded-none"
              onClick={() => toggleRightPanel(activeSessionId)}
            >
              <IconLayoutSidebarRightCollapse className="h-3 w-3" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Show right panel</TooltipContent>
        </Tooltip>
      ) : (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="icon-sm"
              variant="ghost"
              className="cursor-pointer rounded-none"
              onClick={() => toggleRightPanel(activeSessionId)}
            >
              <IconLayoutSidebarRightCollapse className="h-3 w-3" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Hide right panel</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}
