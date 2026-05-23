"use client";

import { useCallback, useState, useSyncExternalStore } from "react";
import { type IDockviewHeaderActionsProps } from "dockview-react";
import {
  IconArrowsMaximize,
  IconArrowsMinimize,
  IconDots,
  IconLayoutColumns,
  IconLayoutRows,
  IconMinus,
  IconWindowMaximize,
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
  "h-5 w-5 inline-flex items-center justify-center rounded-[5px] text-muted-foreground/50 hover:text-foreground transition-colors cursor-pointer";

/** Width thresholds for collapsing split/close into a dropdown. Hysteresis avoids toggle flicker. */
const COLLAPSE_WIDTH = 320;
const EXPAND_WIDTH = 340;

/** Subscribe to a dockview group's width via its panel api. */
export function useDockviewGroupWidth(group: IDockviewHeaderActionsProps["group"]): number {
  // Stable subscribe/getSnapshot tied to the group identity so useSyncExternalStore
  // does not resubscribe on every render.
  const subscribe = useCallback(
    (cb: () => void) => {
      const d = group.api.onDidDimensionsChange(cb);
      return () => d.dispose();
    },
    [group],
  );
  const getSnapshot = useCallback(() => group.api.width, [group]);
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
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

type MinimizeHandlers = {
  isChatGroup: boolean;
  isMinimized: boolean;
  onMinimize: () => void;
};

function MinimizeButton({
  isMinimized,
  onMinimize,
}: {
  isMinimized: boolean;
  onMinimize: () => void;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          className={ACTION_BTN}
          onClick={onMinimize}
          data-testid="dockview-minimize-btn"
        >
          {isMinimized ? (
            <IconWindowMaximize className="h-3 w-3" />
          ) : (
            <IconMinus className="h-3 w-3" />
          )}
        </button>
      </TooltipTrigger>
      <TooltipContent>{isMinimized ? "Restore" : "Minimize"}</TooltipContent>
    </Tooltip>
  );
}

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

function InlineSplitClose({
  isChatGroup,
  onSplitRight,
  onSplitDown,
  onCloseGroup,
}: SplitCloseHandlers) {
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

function SplitCloseDropdown({
  isChatGroup,
  isMinimized,
  onMinimize,
  onSplitRight,
  onSplitDown,
  onCloseGroup,
}: SplitCloseHandlers & MinimizeHandlers) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button type="button" className={ACTION_BTN} data-testid="dockview-group-actions-menu">
          <IconDots className="h-3 w-3" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {!isChatGroup && (
          <DropdownMenuItem
            onClick={onMinimize}
            className="cursor-pointer text-xs"
            data-testid="dockview-minimize-menuitem"
          >
            {isMinimized ? (
              <IconWindowMaximize className="h-3.5 w-3.5 mr-1.5" />
            ) : (
              <IconMinus className="h-3.5 w-3.5 mr-1.5" />
            )}
            {isMinimized ? "Restore" : "Minimize"}
          </DropdownMenuItem>
        )}
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
  isMinimized: boolean;
  onMinimize: () => void;
};

/** Presentational view: maximize stays inline; split/close collapse into a dropdown when narrow.
 *  Minimize sits between Maximize and the split/close cluster in wide mode and folds into the
 *  dropdown as its first item in narrow mode. Chat group never sees Minimize (same exclusion as Close). */
export function GroupSplitCloseActionsView({
  width,
  isChatGroup,
  isMaximized,
  onMaximize,
  isMinimized,
  onMinimize,
  onSplitRight,
  onSplitDown,
  onCloseGroup,
}: GroupSplitCloseActionsViewProps) {
  const collapsed = useCollapsedWithHysteresis(width);
  const splitCloseHandlers = { isChatGroup, onSplitRight, onSplitDown, onCloseGroup };
  const minimizeHandlers = { isChatGroup, isMinimized, onMinimize };
  return (
    <>
      <MaximizeButton isMaximized={isMaximized} onMaximize={onMaximize} />
      {!isChatGroup && !collapsed && (
        <MinimizeButton isMinimized={isMinimized} onMinimize={onMinimize} />
      )}
      {collapsed ? (
        <SplitCloseDropdown {...splitCloseHandlers} {...minimizeHandlers} />
      ) : (
        <InlineSplitClose {...splitCloseHandlers} />
      )}
    </>
  );
}
