"use client";

import {
  IconSettings,
  IconTextWrap,
  IconLayoutColumns,
  IconLayoutRows,
  IconMessageForward,
  IconArrowsMaximize,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { PanelHeaderBarSplit } from "./panel-primitives";

export type ChangesTopBarProps = {
  autoMarkOnScroll: boolean;
  splitView: boolean;
  wordWrap: boolean;
  totalCommentCount: number;
  reviewedCount: number;
  totalCount: number;
  progressPercent: number;
  setWordWrap: (v: boolean) => void;
  handleToggleSplitView: (v: boolean) => void;
  handleToggleAutoMark: (v: boolean) => void;
  handleFixComments: () => void;
};

function ChangesTopBarLeft({
  autoMarkOnScroll,
  totalCount,
  reviewedCount,
  progressPercent,
  handleToggleAutoMark,
}: Pick<
  ChangesTopBarProps,
  "autoMarkOnScroll" | "totalCount" | "reviewedCount" | "progressPercent" | "handleToggleAutoMark"
>) {
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" variant="ghost" className="px-1.5 h-5 cursor-pointer">
            <IconSettings className="h-3.5 w-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-64">
          <DropdownMenuItem
            className="cursor-pointer gap-2"
            onSelect={(e) => {
              e.preventDefault();
              handleToggleAutoMark(!autoMarkOnScroll);
            }}
          >
            <Checkbox checked={autoMarkOnScroll} className="pointer-events-none" />
            <span className="text-sm flex-1">Auto-mark reviewed on scroll</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {totalCount > 0 && (
        <div className="flex items-center gap-2 min-w-0">
          <div className="w-20 h-1 rounded-full bg-muted overflow-hidden">
            <div
              className="h-full bg-indigo-500 rounded-full transition-all duration-300"
              style={{ width: `${progressPercent}%` }}
            />
          </div>
          <span className="text-[11px] text-muted-foreground whitespace-nowrap">
            {reviewedCount}/{totalCount} Reviewed
          </span>
        </div>
      )}
    </>
  );
}

function ChangesTopBarRight({
  splitView,
  wordWrap,
  totalCommentCount,
  setWordWrap,
  handleToggleSplitView,
  handleFixComments,
}: Pick<
  ChangesTopBarProps,
  | "splitView"
  | "wordWrap"
  | "totalCommentCount"
  | "setWordWrap"
  | "handleToggleSplitView"
  | "handleFixComments"
>) {
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className="px-1.5 h-5 cursor-pointer"
            onClick={() => window.dispatchEvent(new CustomEvent("open-review-dialog"))}
          >
            <IconArrowsMaximize className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Expand review</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className={`px-1.5 h-5 cursor-pointer ${wordWrap ? "bg-muted" : ""}`}
            onClick={() => setWordWrap(!wordWrap)}
          >
            <IconTextWrap className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Toggle word wrap</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className="px-1.5 h-5 cursor-pointer"
            onClick={() => handleToggleSplitView(!splitView)}
          >
            {splitView ? (
              <IconLayoutRows className="h-3.5 w-3.5" />
            ) : (
              <IconLayoutColumns className="h-3.5 w-3.5" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{splitView ? "Unified view" : "Split view"}</TooltipContent>
      </Tooltip>
      {totalCommentCount > 0 && (
        <Button
          size="sm"
          variant="outline"
          className="h-5 text-xs cursor-pointer"
          onClick={handleFixComments}
        >
          <IconMessageForward className="h-3.5 w-3.5" />
          Fix
          <span className="ml-0.5 rounded-full bg-blue-500/30 px-1 py-0 text-[10px] font-medium text-blue-600 dark:text-blue-400">
            {totalCommentCount}
          </span>
        </Button>
      )}
    </>
  );
}

export function ChangesTopBar({
  autoMarkOnScroll,
  splitView,
  wordWrap,
  totalCommentCount,
  reviewedCount,
  totalCount,
  progressPercent,
  setWordWrap,
  handleToggleSplitView,
  handleToggleAutoMark,
  handleFixComments,
}: ChangesTopBarProps) {
  return (
    <PanelHeaderBarSplit
      left={
        <ChangesTopBarLeft
          autoMarkOnScroll={autoMarkOnScroll}
          totalCount={totalCount}
          reviewedCount={reviewedCount}
          progressPercent={progressPercent}
          handleToggleAutoMark={handleToggleAutoMark}
        />
      }
      right={
        <ChangesTopBarRight
          splitView={splitView}
          wordWrap={wordWrap}
          totalCommentCount={totalCommentCount}
          setWordWrap={setWordWrap}
          handleToggleSplitView={handleToggleSplitView}
          handleFixComments={handleFixComments}
        />
      }
    />
  );
}
