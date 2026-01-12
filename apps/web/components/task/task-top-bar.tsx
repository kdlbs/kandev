'use client';

import { memo, useState } from 'react';
import Link from 'next/link';
import {
  IconArrowDown,
  IconArrowLeft,
  IconBrandVscode,
  IconChevronDown,
  IconChevronRight,
  IconCopy,
  IconEye,
  IconFolder,
  IconGitBranch,
  IconGitFork,
  IconGitMerge,
  IconGitPullRequest,
  IconLoader2,
  IconPencil,
  IconPlayerPlay,
  IconPlayerStop,
} from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { CommitStatBadge, LineStat } from '@/components/diff-stat';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';

type TaskTopBarProps = {
  taskTitle?: string;
  baseBranch?: string;
  branches?: string[];
  branchesLoading?: boolean;
  onStartAgent?: () => void;
  onStopAgent?: () => void;
  isAgentRunning?: boolean;
  isAgentLoading?: boolean;
  worktreePath?: string | null;
  worktreeBranch?: string | null;
};

const TaskTopBar = memo(function TaskTopBar({
  taskTitle,
  baseBranch,
  branches = [],
  branchesLoading = false,
  onStartAgent,
  onStopAgent,
  isAgentRunning = false,
  isAgentLoading = false,
  worktreePath,
  worktreeBranch,
}: TaskTopBarProps) {
  const [branchName, setBranchName] = useState('feature/agent-ui');
  const [isEditingBranch, setIsEditingBranch] = useState(false);
  const [selectedBaseBranch, setSelectedBaseBranch] = useState(baseBranch ?? 'origin/main');

  return (
    <header className="flex items-center justify-between p-3">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="sm" asChild>
          <Link href="/">
            <IconArrowLeft className="h-4 w-4" />
            Back
          </Link>
        </Button>
        <span className="text-xs text-muted-foreground">{taskTitle ?? 'Task details'}</span>
        <div className="flex items-center gap-2">
          <div className="group flex items-center gap-2 rounded-md px-2 h-8 hover:bg-muted/40 cursor-default">
            <IconGitFork className="h-4 w-4 text-muted-foreground" />
            {isEditingBranch ? (
              <input
                className="bg-background text-sm outline-none w-[160px] rounded-md border border-border/70 px-1"
                value={branchName}
                onChange={(event) => setBranchName(event.target.value)}
                onBlur={() => setIsEditingBranch(false)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' || event.key === 'Escape') {
                    setIsEditingBranch(false);
                  }
                }}
                autoFocus
              />
            ) : (
              <>
                <span className="text-sm font-medium">{branchName}</span>
                <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100">
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type="button"
                        className="text-muted-foreground hover:text-foreground cursor-pointer"
                        onClick={() => setIsEditingBranch(true)}
                      >
                        <IconPencil className="h-3.5 w-3.5" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent>Edit branch name</TooltipContent>
                  </Tooltip>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type="button"
                        className="text-muted-foreground hover:text-foreground cursor-pointer"
                        onClick={() => navigator.clipboard?.writeText(branchName)}
                      >
                        <IconCopy className="h-3.5 w-3.5" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent>Copy branch name</TooltipContent>
                  </Tooltip>
                </div>
              </>
            )}
          </div>
          <IconChevronRight className="h-4 w-4 text-muted-foreground" />
          <Select
            value={selectedBaseBranch}
            onValueChange={setSelectedBaseBranch}
            disabled={branches.length === 0 && !branchesLoading}
          >
            <Tooltip>
              <TooltipTrigger asChild>
                <SelectTrigger className="w-[190px] h-8 cursor-pointer border border-transparent bg-transparent hover:bg-muted/40 data-[state=open]:bg-background data-[state=open]:border-border/70">
                  <SelectValue
                    placeholder={branchesLoading ? 'Loading branches...' : 'Base branch'}
                  />
                </SelectTrigger>
              </TooltipTrigger>
              <TooltipContent>Change base branch</TooltipContent>
            </Tooltip>
            <SelectContent>
              {branches.length === 0 ? (
                <SelectItem value={selectedBaseBranch} disabled>
                  {selectedBaseBranch}
                </SelectItem>
              ) : (
                branches.map((branch) => (
                  <SelectItem key={branch} value={branch}>
                    {branch}
                  </SelectItem>
                ))
              )}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="cursor-default">
                <CommitStatBadge label="2 ahead" tone="ahead" />
              </span>
            </TooltipTrigger>
            <TooltipContent>Commits ahead of base</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="cursor-default">
                <CommitStatBadge label="4 behind" tone="behind" />
              </span>
            </TooltipTrigger>
            <TooltipContent>Commits behind base</TooltipContent>
          </Tooltip>
        </div>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="cursor-default">
              <LineStat added={855} removed={8} />
            </span>
          </TooltipTrigger>
          <TooltipContent>Lines changed</TooltipContent>
        </Tooltip>
        {/* Worktree Info */}
        {worktreePath && (
          <div className="flex items-center gap-2 ml-2 pl-2 border-l border-border/50">
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="group flex items-center gap-1.5 rounded-md px-2 h-7 bg-muted/40 hover:bg-muted/60 cursor-pointer">
                  <IconFolder className="h-3.5 w-3.5 text-muted-foreground" />
                  <span className="text-xs text-muted-foreground max-w-[200px] truncate">
                    {worktreePath}
                  </span>
                  <button
                    type="button"
                    className="text-muted-foreground hover:text-foreground opacity-0 group-hover:opacity-100"
                    onClick={(e) => {
                      e.stopPropagation();
                      navigator.clipboard?.writeText(worktreePath);
                    }}
                  >
                    <IconCopy className="h-3 w-3" />
                  </button>
                </div>
              </TooltipTrigger>
              <TooltipContent className="max-w-[400px]">
                <div className="space-y-1">
                  <div className="font-medium">Worktree Path</div>
                  <div className="text-xs text-muted-foreground break-all">{worktreePath}</div>
                  {worktreeBranch && (
                    <div className="text-xs">
                      Branch: <span className="font-mono">{worktreeBranch}</span>
                    </div>
                  )}
                </div>
              </TooltipContent>
            </Tooltip>
          </div>
        )}
      </div>
      <div className="flex items-center gap-2">
        {/* Start/Stop Agent Button */}
        {isAgentRunning ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant="destructive"
                className="cursor-pointer"
                onClick={onStopAgent}
                disabled={isAgentLoading}
              >
                {isAgentLoading ? (
                  <IconLoader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <IconPlayerStop className="h-4 w-4" />
                )}
                Stop Agent
              </Button>
            </TooltipTrigger>
            <TooltipContent>Stop the agent</TooltipContent>
          </Tooltip>
        ) : (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant="default"
                className="cursor-pointer"
                onClick={onStartAgent}
                disabled={isAgentLoading}
              >
                {isAgentLoading ? (
                  <IconLoader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <IconPlayerPlay className="h-4 w-4" />
                )}
                Start Agent
              </Button>
            </TooltipTrigger>
            <TooltipContent>Start the agent on this task</TooltipContent>
          </Tooltip>
        )}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="sm" variant="outline" className="cursor-pointer">
              <IconBrandVscode className="h-4 w-4" />
              Editor
            </Button>
          </TooltipTrigger>
          <TooltipContent>Open in editor</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="sm" variant="outline" className="cursor-pointer">
              <IconArrowDown className="h-4 w-4" />
              Pull
            </Button>
          </TooltipTrigger>
          <TooltipContent>Pull from remote</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="sm" variant="outline" className="cursor-pointer">
              <IconEye className="h-4 w-4" />
              Review
            </Button>
          </TooltipTrigger>
          <TooltipContent>Open review</TooltipContent>
        </Tooltip>
        <div className="inline-flex rounded-md border border-border overflow-hidden">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button size="sm" variant="outline" className="rounded-none border-0 cursor-pointer">
                <IconGitPullRequest className="h-4 w-4" />
                Create PR
              </Button>
            </TooltipTrigger>
            <TooltipContent>Create PR</TooltipContent>
          </Tooltip>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm" variant="outline" className="rounded-none border-0 px-2 cursor-pointer">
                <IconChevronDown className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem>
                <IconPencil className="h-4 w-4" />
                Create PR manually
              </DropdownMenuItem>
              <DropdownMenuItem>
                <IconGitMerge className="h-4 w-4" />
                Merge
              </DropdownMenuItem>
              <DropdownMenuItem>
                <IconGitBranch className="h-4 w-4" />
                Rebase
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </header>
  );
});

export { TaskTopBar };
