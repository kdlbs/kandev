"use client";

import { memo, useCallback, type ComponentProps, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
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
import { useRemoteContributionRelation } from "@/hooks/domains/session/use-remote-contribution-relation";
import { remoteContributionActionPolicy } from "@/hooks/domains/session/remote-contribution-relation";
import { gitOperationLabel, useGitWithFeedback } from "@/hooks/use-git-with-feedback";
import { useVcsDialogs } from "@/components/vcs/vcs-dialogs";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useRepoDisplayName } from "@/hooks/domains/session/use-repo-display-name";
import { MultiRepoVcsButton } from "@/components/vcs-multi-repo-menu";

const DEFAULT_BASE_BRANCH = "origin/main";

export function determinePrimaryAction(
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

// The count-bearing tooltips go through `count` + `_one`/`_other` rather than a
// concatenated `commit${n !== 1 ? "s" : ""}`: an English morpheme appended here
// puts the plural rule at the call site, where no other locale can reach it.
function buildCommitConfig(
  t: TFunction,
  uncommittedFileCount: number,
  openCommitDialog: () => void,
): PrimaryButtonConfig {
  return {
    icon: <IconGitCommit className="h-4 w-4" />,
    label: t("integrations:commit"),
    badge: uncommittedFileCount > 0 ? uncommittedFileCount : null,
    tooltip:
      uncommittedFileCount > 0
        ? t("integrations:commitChangedFiles", { count: uncommittedFileCount })
        : t("integrations:noChangesToCommit"),
    onClick: openCommitDialog,
  };
}

function buildPrConfig(
  t: TFunction,
  aheadCount: number,
  openPRDialog: () => void,
): PrimaryButtonConfig {
  return {
    icon: <IconGitPullRequest className="h-4 w-4" />,
    label: t("integrations:createPr"),
    badge: null,
    tooltip: t("integrations:createPrCommitsAhead", { count: aheadCount }),
    onClick: openPRDialog,
  };
}

function buildPushConfig(
  t: TFunction,
  aheadCount: number,
  handlePush: () => void,
): PrimaryButtonConfig {
  return {
    icon: <IconCloudUpload className="h-4 w-4" />,
    label: t("integrations:push"),
    badge: null,
    tooltip: t("integrations:pushCommitsToRemote", { count: aheadCount }),
    onClick: handlePush,
  };
}

function buildRebaseConfig(
  t: TFunction,
  behindCount: number,
  baseBranch: string | undefined,
  handleRebase: () => void,
): PrimaryButtonConfig {
  return {
    icon: <IconGitCherryPick className="h-4 w-4" />,
    label: t("integrations:rebase"),
    badge: behindCount > 0 ? behindCount : null,
    // `branch` is a git ref — data, interpolated rather than translated.
    tooltip: t("integrations:rebaseOntoBranchBehind", {
      branch: baseBranch || DEFAULT_BASE_BRANCH,
      behind: behindCount,
    }),
    onClick: handleRebase,
  };
}

type PrimaryConfigArgs = {
  t: TFunction;
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
  t,
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
  if (primaryAction === "push") return buildPushConfig(t, aheadCount, handlePush);
  if (primaryAction === "pr") return buildPrConfig(t, aheadCount, openPRDialog);
  if (primaryAction === "rebase")
    return buildRebaseConfig(t, behindCount, baseBranch, handleRebase);
  return buildCommitConfig(t, uncommittedFileCount, openCommitDialog);
}

type DivergenceTone = "ahead" | "behind";

const divergenceToneClass: Record<DivergenceTone, string> = {
  ahead: "border-emerald-500/40 bg-emerald-500/10 text-emerald-500",
  behind: "border-yellow-500/40 bg-yellow-500/10 text-yellow-500",
};

function DivergencePill({
  tone,
  value,
  ariaLabel,
}: {
  tone: DivergenceTone;
  value: number;
  ariaLabel: string;
}) {
  if (value <= 0) return null;

  return (
    <span
      aria-label={ariaLabel}
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
  const { t } = useTranslation();
  if (ahead <= 0 && behind <= 0) return null;

  // `{{value}}`, not `{{count}}`: the shipped English is count-invariant
  // ("1 commits ahead"), and switching to a plural here would change the
  // accessible name that E2E specs query by.
  return (
    <span className="ml-1 inline-flex items-center gap-1">
      <DivergencePill
        tone="ahead"
        value={ahead}
        ariaLabel={t("integrations:commitsAheadAriaLabel", { value: ahead })}
      />
      <DivergencePill
        tone="behind"
        value={behind}
        ariaLabel={t("integrations:commitsBehindAriaLabel", { value: behind })}
      />
    </span>
  );
}

type VcsDropdownItemsProps = {
  disabled: boolean;
  baseBranch?: string;
  hasUpstream: boolean;
  behindCount: number;
  aheadCount: number;
  pushDisabled: boolean;
  pullDisabled: boolean;
  onPR: () => void;
  onPull: () => void;
  onPush: (force: boolean) => void;
  onRebase: () => void;
  onMerge: () => void;
};

function VcsDropdownItems({
  disabled,
  baseBranch,
  hasUpstream,
  behindCount,
  aheadCount,
  pushDisabled,
  pullDisabled,
  onPR,
  onPull,
  onPush,
  onRebase,
  onMerge,
}: VcsDropdownItemsProps) {
  const { t } = useTranslation();
  const remoteActionsDisabledTitle = t("task:remoteActionsDisabledHistoryChanged");
  return (
    <DropdownMenuContent align="end" className="w-56">
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onPR} disabled={disabled}>
        <IconGitPullRequest className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("integrations:createPr")}</span>
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem
        className="cursor-pointer gap-3"
        onClick={onPull}
        disabled={disabled || pullDisabled}
        title={pullDisabled ? remoteActionsDisabledTitle : undefined}
      >
        <IconCloudDownload className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("integrations:pull")}</span>
        {hasUpstream && behindCount > 0 && (
          <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
            ↓{behindCount}
          </span>
        )}
      </DropdownMenuItem>
      <DropdownMenuSub>
        <DropdownMenuSubTrigger
          className="cursor-pointer gap-3"
          disabled={disabled || pushDisabled}
          title={pushDisabled ? remoteActionsDisabledTitle : undefined}
        >
          <IconCloudUpload className="h-4 w-4 text-muted-foreground" />
          <span className="flex-1">{t("integrations:push")}</span>
          {hasUpstream && aheadCount > 0 && (
            <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
              ↑{aheadCount}
            </span>
          )}
        </DropdownMenuSubTrigger>
        <DropdownMenuSubContent>
          <DropdownMenuItem
            className="cursor-pointer gap-3"
            onClick={() => onPush(false)}
            disabled={disabled || pushDisabled}
            title={pushDisabled ? remoteActionsDisabledTitle : undefined}
          >
            <IconCloudUpload className="h-4 w-4 text-muted-foreground" />
            <span>{t("integrations:push")}</span>
          </DropdownMenuItem>
          <DropdownMenuItem
            className="cursor-pointer gap-3"
            onClick={() => onPush(true)}
            disabled={disabled || pushDisabled}
            title={pushDisabled ? remoteActionsDisabledTitle : undefined}
          >
            <IconAlertTriangle className="h-4 w-4 text-muted-foreground" />
            <span>{t("integrations:forcePush")}</span>
          </DropdownMenuItem>
        </DropdownMenuSubContent>
      </DropdownMenuSub>
      <DropdownMenuSeparator />
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onRebase} disabled={disabled}>
        <IconGitCherryPick className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("integrations:rebase")}</span>
        {/* The branch is a git ref — data. One message with the ref
            interpolated, not an "onto" stem concatenated onto it. */}
        <span className="text-xs text-muted-foreground">
          {t("integrations:ontoBranch", { branch: baseBranch || DEFAULT_BASE_BRANCH })}
        </span>
      </DropdownMenuItem>
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onMerge} disabled={disabled}>
        <IconGitMerge className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("integrations:merge")}</span>
        <span className="text-xs text-muted-foreground">
          {t("integrations:fromBranch", { branch: baseBranch || DEFAULT_BASE_BRANCH })}
        </span>
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
  const { t } = useTranslation();
  const gitWithFeedback = useGitWithFeedback();

  const handlePull = useCallback(
    (repo?: string) => {
      gitWithFeedback(() => git.pull(false, repo), gitOperationLabel(t, "common:gitOpPull", repo));
    },
    [gitWithFeedback, git, t],
  );

  const handlePush = useCallback(
    (force = false, repo?: string) => {
      const operationKey = force ? "common:gitOpForcePush" : "common:gitOpPush";
      gitWithFeedback(() => git.push({ force }, repo), gitOperationLabel(t, operationKey, repo));
    },
    [gitWithFeedback, git, t],
  );

  const handleRebase = useCallback(
    (repo?: string) => {
      const targetBranch = baseBranch?.replace(/^origin\//, "") || "main";
      gitWithFeedback(
        () => git.rebase(targetBranch, repo),
        gitOperationLabel(t, "common:gitOpRebase", repo),
      );
    },
    [gitWithFeedback, git, baseBranch, t],
  );

  const handleMerge = useCallback(
    (repo?: string) => {
      const targetBranch = baseBranch?.replace(/^origin\//, "") || "main";
      gitWithFeedback(
        () => git.merge(targetBranch, repo),
        gitOperationLabel(t, "common:gitOpMerge", repo),
      );
    },
    [gitWithFeedback, git, baseBranch, t],
  );

  return { handlePull, handlePush, handleRebase, handleMerge };
}

const VcsSplitButton = memo(function VcsSplitButton({
  sessionId,
  baseBranch,
  buttonSize = "sm",
  className,
}: VcsSplitButtonProps) {
  const { t } = useTranslation();
  const git = useSessionGit(sessionId);
  const { openCommitDialog, openPRDialog } = useVcsDialogs();
  const activePR = useActiveTaskPR();
  const { relation } = useRemoteContributionRelation(sessionId);
  const remoteActionPolicy = remoteContributionActionPolicy(relation);
  const hasOpenPR = activePR?.state === "open";
  const { handlePull, handlePush, handleRebase, handleMerge } = useGitActions(git, baseBranch);
  const repoDisplayName = useRepoDisplayName(sessionId);

  const remoteBranch = git.remoteBranch;
  const hasUpstream = Boolean(remoteBranch);
  const uncommittedFileCount = git.allFiles.length;
  const aheadCount = remoteActionPolicy.pushDisabled ? 0 : git.pushAhead;
  const behindCount = git.behind;
  const pullCount = git.pullBehind;
  const isDisabled = git.isLoading || !sessionId;
  const isGitLoading = git.isLoading;
  // Multi-repo when there's more than one named repo. Single-repo workspaces
  // get either a single empty-name entry or no entries at all in repoNames.
  const isMultiRepo = git.repoNames.length > 1;

  const primaryAction = determinePrimaryAction(
    uncommittedFileCount,
    aheadCount,
    behindCount,
    hasOpenPR,
  );
  const primaryButtonConfig = buildPrimaryButtonConfig({
    t,
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

  if (isMultiRepo) {
    return (
      <MultiRepoVcsButton
        primaryButtonConfig={primaryButtonConfig}
        primaryAction={primaryAction}
        isDisabled={isDisabled}
        isGitLoading={isGitLoading}
        baseBranch={baseBranch || DEFAULT_BASE_BRANCH}
        repoNames={git.repoNames}
        perRepoStatus={git.perRepoStatus}
        pushDisabled={remoteActionPolicy.pushDisabled}
        pullDisabled={remoteActionPolicy.pullDisabled}
        repoDisplayName={repoDisplayName}
        callbacks={{
          onCommit: (repo) => openCommitDialog(repo),
          onPR: (repo) => openPRDialog(repo),
          onPull: handlePull,
          onPush: handlePush,
          onRebase: handleRebase,
          onMerge: handleMerge,
        }}
      />
    );
  }

  return (
    <SingleRepoVcsButton
      primaryButtonConfig={primaryButtonConfig}
      primaryAction={primaryAction}
      isDisabled={isDisabled}
      isGitLoading={isGitLoading}
      baseBranch={baseBranch}
      hasUpstream={hasUpstream}
      behindCount={pullCount}
      aheadCount={aheadCount}
      pushDisabled={remoteActionPolicy.pushDisabled}
      pullDisabled={remoteActionPolicy.pullDisabled}
      buttonSize={buttonSize}
      className={className}
      showDivergencePills={showDivergencePills}
      onPR={() => openPRDialog()}
      onPull={() => handlePull()}
      onPush={(force) => handlePush(force)}
      onRebase={() => handleRebase()}
      onMerge={() => handleMerge()}
    />
  );
});

function SingleRepoVcsButton({
  primaryButtonConfig,
  primaryAction,
  isDisabled,
  isGitLoading,
  baseBranch,
  hasUpstream,
  behindCount,
  aheadCount,
  pushDisabled,
  pullDisabled,
  onPR,
  onPull,
  onPush,
  onRebase,
  onMerge,
  className,
  buttonSize = "sm",
  showDivergencePills = false,
}: {
  primaryButtonConfig: PrimaryButtonConfig;
  primaryAction: "commit" | "push" | "pr" | "rebase";
  isDisabled: boolean;
  isGitLoading: boolean;
  baseBranch?: string;
  hasUpstream: boolean;
  behindCount: number;
  aheadCount: number;
  pushDisabled: boolean;
  pullDisabled: boolean;
  onPR: () => void;
  onPull: () => void;
  onPush: (force: boolean) => void;
  onRebase: () => void;
  onMerge: () => void;
  className?: string;
  buttonSize?: ComponentProps<typeof Button>["size"];
  showDivergencePills?: boolean;
}) {
  const { t } = useTranslation();
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
            aria-label={t("integrations:openVcsOptions")}
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
          hasUpstream={hasUpstream}
          behindCount={behindCount}
          aheadCount={aheadCount}
          pushDisabled={pushDisabled}
          pullDisabled={pullDisabled}
          onPR={onPR}
          onPull={onPull}
          onPush={onPush}
          onRebase={onRebase}
          onMerge={onMerge}
        />
      </DropdownMenu>
    </div>
  );
}

export { VcsSplitButton };
