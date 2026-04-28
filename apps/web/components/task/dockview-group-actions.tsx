"use client";

import { useState, useSyncExternalStore } from "react";
import { type IDockviewHeaderActionsProps } from "dockview-react";
import {
  IconArrowsMaximize,
  IconArrowsMinimize,
  IconDots,
  IconLayoutColumns,
  IconLayoutRows,
  IconX,
} from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";

const ACTION_BTN =
  "h-5 w-5 inline-flex items-center justify-center text-muted-foreground/50 hover:text-foreground transition-colors cursor-pointer";

/** Width thresholds for collapsing split/close into a dropdown. Hysteresis avoids toggle flicker. */
const COLLAPSE_WIDTH = 320;
const EXPAND_WIDTH = 340;

/** Subscribe to a dockview group's width via its panel api. */
export function useDockviewGroupWidth(group: IDockviewHeaderActionsProps["group"]): number {
  return useSyncExternalStore(
    (cb) => {
      const d = group.api.onDidDimensionsChange(cb);
      return () => d.dispose();
    },
    () => group.api.width,
    () => group.api.width,
  );
}

/**
 * Sticky collapsed state with hysteresis. Uses the documented React pattern of
 * storing previous-derived state and conditionally updating during render —
 * https://react.dev/reference/react/useState#storing-information-from-previous-renders
 */
function useCollapsedWithHysteresis(width: number): boolean {
  const [collapsed, setCollapsed] = useState<boolean>(width < COLLAPSE_WIDTH);
  const next = collapsed ? width < EXPAND_WIDTH : width < COLLAPSE_WIDTH;
  if (next !== collapsed) {
    setCollapsed(next);
    return next;
  }
  return collapsed;
}

type SplitCloseHandlers = {
  isChatGroup: boolean;
  onSplitRight: () => void;
  onSplitDown: () => void;
  onCloseGroup: () => void;
};

function MaximizeButton({
  isMaximized,
  onMaximize,
}: {
  isMaximized: boolean;
  onMaximize: () => void;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          className={ACTION_BTN}
          onClick={onMaximize}
          data-testid="dockview-maximize-btn"
        >
          {isMaximized ? (
            <IconArrowsMinimize className="h-3 w-3" />
          ) : (
            <IconArrowsMaximize className="h-3 w-3" />
          )}
        </button>
      </TooltipTrigger>
      <TooltipContent>{isMaximized ? "Restore" : "Maximize"}</TooltipContent>
    </Tooltip>
  );
}

function InlineSplitClose({ isChatGroup, onSplitRight, onSplitDown, onCloseGroup }: SplitCloseHandlers) {
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className={ACTION_BTN}
            onClick={onSplitRight}
            data-testid="dockview-split-right-btn"
          >
            <IconLayoutColumns className="h-3 w-3" />
          </button>
        </TooltipTrigger>
        <TooltipContent>Split right</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className={ACTION_BTN}
            onClick={onSplitDown}
            data-testid="dockview-split-down-btn"
          >
            <IconLayoutRows className="h-3 w-3" />
          </button>
        </TooltipTrigger>
        <TooltipContent>Split down</TooltipContent>
      </Tooltip>
      {!isChatGroup && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              className={ACTION_BTN}
              onClick={onCloseGroup}
              data-testid="dockview-close-group-btn"
            >
              <IconX className="h-3 w-3" />
            </button>
          </TooltipTrigger>
          <TooltipContent>Close group</TooltipContent>
        </Tooltip>
      )}
    </>
  );
}


function SplitCloseDropdown({ isChatGroup, onSplitRight, onSplitDown, onCloseGroup }: SplitCloseHandlers) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className={ACTION_BTN}
          data-testid="dockview-group-actions-menu"
        >
          <IconDots className="h-3 w-3" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={onSplitRight} className="cursor-pointer text-xs">
          <IconLayoutColumns className="h-3.5 w-3.5 mr-1.5" />
          Split right
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onSplitDown} className="cursor-pointer text-xs">
          <IconLayoutRows className="h-3.5 w-3.5 mr-1.5" />
          Split down
        </DropdownMenuItem>
        {!isChatGroup && (
          <DropdownMenuItem onClick={onCloseGroup} className="cursor-pointer text-xs">
            <IconX className="h-3.5 w-3.5 mr-1.5" />
            Close group
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

type GroupSplitCloseActionsViewProps = SplitCloseHandlers & {
  width: number;
  isMaximized: boolean;
  onMaximize: () => void;
};

/** Presentational view: maximize stays inline; split/close collapse into a dropdown when narrow. */
export function GroupSplitCloseActionsView({
  width,
  isChatGroup,
  isMaximized,
  onMaximize,
  onSplitRight,
  onSplitDown,
  onCloseGroup,
}: GroupSplitCloseActionsViewProps) {
  const collapsed = useCollapsedWithHysteresis(width);
  const handlers = { isChatGroup, onSplitRight, onSplitDown, onCloseGroup };
  return (
    <>
      <MaximizeButton isMaximized={isMaximized} onMaximize={onMaximize} />
      {collapsed ? <SplitCloseDropdown {...handlers} /> : <InlineSplitClose {...handlers} />}
    </>
  );
}
