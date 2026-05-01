"use client";

import { memo, useCallback, type ComponentProps, type ReactNode } from "react";
import {
  IconGitCommit,
  IconGitPullRequest,
  IconGitMerge,
  IconGitCherryPick,
  IconLoader2,
  IconChevronDown,
  IconCloudDownload,
  IconCloudUpload,
  IconAlertTriangle,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@kandev/ui/lib/utils";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import { useGitWithFeedback } from "@/hooks/use-git-with-feedback";
import { useVcsDialogs } from "@/components/vcs/vcs-dialogs";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";

function determinePrimaryAction(
  uncommittedFileCount: number,
  aheadCount: number,
  behindCount: number,
  hasOpenPR: boolean,
): "commit" | "push" | "pr" | "rebase" {
  if (uncommittedFileCount > 0) return "commit";
  if (aheadCount > 0 && hasOpenPR) return "push";
  if (aheadCount > 0) return "pr";
  if (behindCount > 0) return "rebase";
  return "commit";
}

type PrimaryButtonConfig = {
  icon: ReactNode;
  label: string;
  badge: number | null;
  tooltip: string;
  onClick: () => void;
};

function buildCommitConfig(
  uncommittedFileCount: number,
  openCommitDialog: () => void,
): PrimaryButtonConfig {
  return {
    icon: <IconGitCommit className="h-4 w-4" />,
    label: "Commit",
    badge: uncommittedFileCount > 0 ? uncommittedFileCount : null,
    tooltip:
      uncommittedFileCount > 0
        ? `Commit ${uncommittedFileCount} changed file${uncommittedFileCount !== 1 ? "s" : ""}`
        : "No changes to commit",
    onClick: openCommitDialog,
  };
}

function buildPrConfig(aheadCount: number, openPRDialog: () => void): PrimaryButtonConfig {
  return {
    icon: <IconGitPullRequest className="h-4 w-4" />,
    label: "Create PR",
    badge: null,
    tooltip: `Create PR (${aheadCount} commit${aheadCount !== 1 ? "s" : ""} ahead)`,
    onClick: openPRDialog,
  };
}

function buildPushConfig(aheadCount: number, handlePush: () => void): PrimaryButtonConfig {
  return {
    icon: <IconCloudUpload className="h-4 w-4" />,
    label: "Push",
    badge: null,
    tooltip: `Push ${aheadCount} commit${aheadCount !== 1 ? "s" : ""} to remote`,
    onClick: handlePush,
  };
}

function buildRebaseConfig(
  behindCount: number,
  baseBranch: string | undefined,
  handleRebase: () => void,
): PrimaryButtonConfig {
  return {
    icon: <IconGitCherryPick className="h-4 w-4" />,
    label: "Rebase",
    badge: null,
    tooltip: `Rebase onto ${baseBranch || "origin/main"} (${behindCount} behind)`,
    onClick: handleRebase,
  };
}

type PrimaryConfigArgs = {
  primaryAction: "commit" | "push" | "pr" | "rebase";
  uncommittedFileCount: number;
  aheadCount: number;
  behindCount: number;
  baseBranch: string | undefined;
  openCommitDialog: () => void;
  openPRDialog: () => void;
  handlePush: () => void;
  handleRebase: () => void;
};

function buildPrimaryButtonConfig({
  primaryAction,
  uncommittedFileCount,
  aheadCount,
  behindCount,
  baseBranch,
  openCommitDialog,
  openPRDialog,
  handlePush,
  handleRebase,
}: PrimaryConfigArgs): PrimaryButtonConfig {
  if (primaryAction === "push") return buildPushConfig(aheadCount, handlePush);
  if (primaryAction === "pr") return buildPrConfig(aheadCount, openPRDialog);
  if (primaryAction === "rebase") return buildRebaseConfig(behindCount, baseBranch, handleRebase);
  return buildCommitConfig(uncommittedFileCount, openCommitDialog);
}

type DivergenceTone = "ahead" | "behind";

const divergenceToneClass: Record<DivergenceTone, string> = {
  ahead: "border-emerald-500/40 bg-emerald-500/10 text-emerald-500",
  behind: "border-yellow-500/40 bg-yellow-500/10 text-yellow-500",
};

function DivergencePill({
  tone,
  value,
  label,
}: {
  tone: DivergenceTone;
  value: number;
  label: string;
}) {
  if (value <= 0) return null;

  return (
    <span
      aria-label={`${value} ${label}`}
      className={cn(
        "inline-flex h-5 items-center rounded-md border px-1.5 text-[11px] font-semibold leading-none tabular-nums",
        divergenceToneClass[tone],
      )}
    >
      {tone === "ahead" ? "↑" : "↓"}
      {value}
    </span>
  );
}

function GitDivergencePills({ ahead, behind }: { ahead: number; behind: number }) {
  if (ahead <= 0 && behind <= 0) return null;

  return (
    <span className="ml-1 inline-flex items-center gap-1">
      <DivergencePill tone="ahead" value={ahead} label="commits ahead" />
      <DivergencePill tone="behind" value={behind} label="commits behind" />
    </span>
  );
}

type VcsDropdownItemsProps = {
  disabled: boolean;
  baseBranch?: string;
  hasMatchingUpstream: boolean | "" | null | undefined;
  behindCount: number;
  aheadCount: number;
  onPR: () => void;
  onPull: () => void;
  onPush: (force: boolean) => void;
  onRebase: () => void;
  onMerge: () => void;
};

function VcsDropdownItems({
  disabled,
  baseBranch,
  hasMatchingUpstream,
  behindCount,
  aheadCount,
  onPR,
  onPull,
  onPush,
  onRebase,
  onMerge,
}: VcsDropdownItemsProps) {
  return (
    <DropdownMenuContent align="end" className="w-56">
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onPR} disabled={disabled}>
        <IconGitPullRequest className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">Create PR</span>
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onPull} disabled={disabled}>
        <IconCloudDownload className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">Pull</span>
        {hasMatchingUpstream && behindCount > 0 && (
          <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
            ↓{behindCount}
          </span>
        )}
      </DropdownMenuItem>
      <DropdownMenuSub>
        <DropdownMenuSubTrigger className="cursor-pointer gap-3" disabled={disabled}>
          <IconCloudUpload className="h-4 w-4 text-muted-foreground" />
          <span className="flex-1">Push</span>
          {hasMatchingUpstream && aheadCount > 0 && (
            <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
              ↑{aheadCount}
            </span>
          )}
        </DropdownMenuSubTrigger>
        <DropdownMenuSubContent>
          <DropdownMenuItem
            className="cursor-pointer gap-3"
            onClick={() => onPush(false)}
            disabled={disabled}
          >
            <IconCloudUpload className="h-4 w-4 text-muted-foreground" />
            <span>Push</span>
          </DropdownMenuItem>
          <DropdownMenuItem
            className="cursor-pointer gap-3"
            onClick={() => onPush(true)}
            disabled={disabled}
          >
            <IconAlertTriangle className="h-4 w-4 text-muted-foreground" />
            <span>Force Push</span>
          </DropdownMenuItem>
        </DropdownMenuSubContent>
      </DropdownMenuSub>
      <DropdownMenuSeparator />
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onRebase} disabled={disabled}>
        <IconGitCherryPick className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">Rebase</span>
        <span className="text-xs text-muted-foreground">onto {baseBranch || "origin/main"}</span>
      </DropdownMenuItem>
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onMerge} disabled={disabled}>
        <IconGitMerge className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">Merge</span>
        <span className="text-xs text-muted-foreground">from {baseBranch || "origin/main"}</span>
      </DropdownMenuItem>
    </DropdownMenuContent>
  );
}

type VcsSplitButtonProps = {
  sessionId: string | null;
  baseBranch?: string;
  buttonSize?: ComponentProps<typeof Button>["size"];
  className?: string;
};

function useGitActions(git: ReturnType<typeof useSessionGit>, baseBranch?: string) {
  const gitWithFeedback = useGitWithFeedback();

  const handlePull = useCallback(() => {
    gitWithFeedback(() => git.pull(), "Pull");
  }, [gitWithFeedback, git]);

  const handlePush = useCallback(
    (force = false) => {
      gitWithFeedback(() => git.push({ force }), force ? "Force Push" : "Push");
    },
    [gitWithFeedback, git],
  );

  const handleRebase = useCallback(() => {
    const targetBranch = baseBranch?.replace(/^origin\//, "") || "main";
    gitWithFeedback(() => git.rebase(targetBranch), "Rebase");
  }, [gitWithFeedback, git, baseBranch]);

  const handleMerge = useCallback(() => {
    const targetBranch = baseBranch?.replace(/^origin\//, "") || "main";
    gitWithFeedback(() => git.merge(targetBranch), "Merge");
  }, [gitWithFeedback, git, baseBranch]);

  return { handlePull, handlePush, handleRebase, handleMerge };
}

const VcsSplitButton = memo(function VcsSplitButton({
  sessionId,
  baseBranch,
  buttonSize = "sm",
  className,
}: VcsSplitButtonProps) {
  const git = useSessionGit(sessionId);
  const { openCommitDialog, openPRDialog } = useVcsDialogs();
  const activePR = useActiveTaskPR();
  const hasOpenPR = activePR?.state === "open";
  const { handlePull, handlePush, handleRebase, handleMerge } = useGitActions(git, baseBranch);

  const currentBranch = git.branch;
  const remoteBranch = git.remoteBranch;
  const hasMatchingUpstream =
    remoteBranch && currentBranch && remoteBranch === `origin/${currentBranch}`;
  const uncommittedFileCount = git.allFiles.length;
  const aheadCount = git.ahead;
  const behindCount = git.behind;
  const isDisabled = git.isLoading || !sessionId;
  const isGitLoading = git.isLoading;

  const primaryAction = determinePrimaryAction(
    uncommittedFileCount,
    aheadCount,
    behindCount,
    hasOpenPR,
  );
  const primaryButtonConfig = buildPrimaryButtonConfig({
    primaryAction,
    uncommittedFileCount,
    aheadCount,
    behindCount,
    baseBranch,
    openCommitDialog,
    openPRDialog,
    handlePush: () => handlePush(false),
    handleRebase,
  });
  const showDivergencePills = primaryAction !== "commit";

  return (
    <div className={cn("inline-flex", className)}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size={buttonSize}
            variant="outline"
            className="rounded-r-none border-r-0 cursor-pointer"
            onClick={primaryButtonConfig.onClick}
            disabled={isDisabled}
            data-testid={`vcs-primary-${primaryAction}`}
          >
            {primaryButtonConfig.icon}
            {primaryButtonConfig.label}
            {primaryButtonConfig.badge != null && (
              <span className="ml-1 rounded-full bg-muted px-1.5 py-0.5 text-xs font-medium text-muted-foreground">
                {primaryButtonConfig.badge}
              </span>
            )}
            {showDivergencePills && <GitDivergencePills ahead={aheadCount} behind={behindCount} />}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{primaryButtonConfig.tooltip}</TooltipContent>
      </Tooltip>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size={buttonSize}
            variant="outline"
            className="-ml-px rounded-l-none px-2 cursor-pointer"
            aria-label="Open VCS options"
            disabled={isDisabled}
          >
            {isGitLoading ? (
              <IconLoader2 className="h-4 w-4 animate-spin" />
            ) : (
              <IconChevronDown className="h-4 w-4" />
            )}
          </Button>
        </DropdownMenuTrigger>
        <VcsDropdownItems
          disabled={isDisabled}
          baseBranch={baseBranch}
          hasMatchingUpstream={hasMatchingUpstream}
          behindCount={behindCount}
          aheadCount={aheadCount}
          onPR={openPRDialog}
          onPull={handlePull}
          onPush={handlePush}
          onRebase={handleRebase}
          onMerge={handleMerge}
        />
      </DropdownMenu>
    </div>
  );
});

export { VcsSplitButton };
