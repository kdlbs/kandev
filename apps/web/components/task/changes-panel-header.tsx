"use client";

import {
  IconCloudDownload,
  IconEye,
  IconChevronDown,
  IconGitBranch,
  IconGitCherryPick,
  IconGitMerge,
  IconArrowRight,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { PanelHeaderBarSplit } from "./panel-primitives";

function BranchHoverCard({
  displayBranch,
  baseBranchDisplay,
}: {
  displayBranch: string;
  baseBranchDisplay: string;
}) {
  return (
    <HoverCard openDelay={200} closeDelay={100}>
      <HoverCardTrigger asChild>
        <button
          type="button"
          className="flex items-center justify-center size-5 rounded hover:bg-muted/60 text-muted-foreground hover:text-foreground transition-colors cursor-default"
        >
          <IconGitBranch className="h-3.5 w-3.5" />
        </button>
      </HoverCardTrigger>
      <HoverCardContent side="bottom" align="end" className="w-auto p-3">
        <div className="flex flex-col gap-2.5 text-xs">
          <div className="flex items-center justify-between gap-6">
            <span className="text-muted-foreground/60">Your code lives in:</span>
            <span className="text-muted-foreground/60">and will be merged into:</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="flex items-center gap-1.5 text-foreground font-medium">
              <IconGitBranch className="h-3.5 w-3.5 text-muted-foreground" />
              {displayBranch}
            </span>
            <div className="flex-1 border-t border-muted-foreground/20 min-w-8" />
            <IconArrowRight className="h-3 w-3 text-muted-foreground/40" />
            <span className="text-foreground font-medium">{baseBranchDisplay}</span>
          </div>
        </div>
      </HoverCardContent>
    </HoverCard>
  );
}

function PullDropdown({
  behindCount,
  isLoading,
  onPull,
  onRebase,
}: {
  behindCount: number;
  isLoading: boolean;
  onPull: () => void;
  onRebase: () => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
          disabled={isLoading}
        >
          <IconCloudDownload className="h-3 w-3" />
          Pull
          {behindCount > 0 && <span className="text-yellow-500 text-[10px]">{behindCount}</span>}
          <IconChevronDown className="h-2.5 w-2.5 text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-44">
        <DropdownMenuItem onClick={onPull} className="cursor-pointer text-xs gap-2">
          <IconCloudDownload className="h-3.5 w-3.5 text-muted-foreground" />
          Pull
          {behindCount > 0 && (
            <span className="ml-auto text-muted-foreground text-[10px]">{behindCount} behind</span>
          )}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onRebase} className="cursor-pointer text-xs gap-2">
          <IconGitCherryPick className="h-3.5 w-3.5 text-muted-foreground" />
          Rebase
          {behindCount > 0 && (
            <span className="ml-auto text-muted-foreground text-[10px]">{behindCount} behind</span>
          )}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function ChangesPanelHeader({
  hasChanges,
  hasCommits,
  displayBranch,
  baseBranchDisplay,
  behindCount,
  isLoading,
  onOpenDiffAll,
  onOpenReview,
  onPull,
  onRebase,
}: {
  hasChanges: boolean;
  hasCommits: boolean;
  displayBranch: string | null;
  baseBranchDisplay: string;
  behindCount: number;
  isLoading: boolean;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
  onPull: () => void;
  onRebase: () => void;
}) {
  return (
    <PanelHeaderBarSplit
      left={
        hasChanges || hasCommits ? (
          <>
            <Button
              size="sm"
              variant="ghost"
              className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
              onClick={onOpenDiffAll}
            >
              <IconGitMerge className="h-3 w-3" />
              Diff
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
              onClick={onOpenReview}
            >
              <IconEye className="h-3 w-3" />
              Review
            </Button>
          </>
        ) : undefined
      }
      right={
        <>
          {displayBranch && (
            <BranchHoverCard displayBranch={displayBranch} baseBranchDisplay={baseBranchDisplay} />
          )}
          <PullDropdown
            behindCount={behindCount}
            isLoading={isLoading}
            onPull={onPull}
            onRebase={onRebase}
          />
        </>
      }
    />
  );
}
