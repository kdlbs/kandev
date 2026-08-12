"use client";

import { memo, useCallback, useMemo, type ComponentProps, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  IconGitCommit,
  IconGitPullRequest,
  IconGitCherryPick,
  IconCloudUpload,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import { useRemoteContributionRelation } from "@/hooks/domains/session/use-remote-contribution-relation";
import { remoteContributionActionPolicy } from "@/hooks/domains/session/remote-contribution-relation";
import { gitOperationLabel, useGitWithFeedback } from "@/hooks/use-git-with-feedback";
import { useVcsDialogs } from "@/components/vcs/vcs-dialogs";
import { useToast } from "@/components/toast-provider";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useRepoDisplayName } from "@/hooks/domains/session/use-repo-display-name";
import {
  useRemoteContributionResolution,
  type RemoteContributionResolutionTarget,
} from "@/components/task/use-remote-contribution-resolution";
import { openExternalLink } from "@/lib/desktop/external-links";
import { VcsSplitButtonContent } from "./vcs-split-button-parts";

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

export type PrimaryButtonConfig = {
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

function useVcsContributionResolution(
  sessionId: string | null,
  relation: ReturnType<typeof useRemoteContributionRelation>["relation"],
  repositoryName: string | undefined,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const resolution = useRemoteContributionResolution(sessionId);
  const confirmResolution = useCallback(async () => {
    const action = resolution.pending?.action;
    const result = await resolution.confirm();
    if (!result?.success) return;
    toast({
      title:
        action === "replace"
          ? t("task:remoteContributionReplaced")
          : t("task:remoteContributionUsed", {
              branch: result.recovery_branch || t("task:remoteRepository"),
            }),
      variant: "success",
    });
  }, [resolution.confirm, resolution.pending?.action, t, toast]);
  const resolutionTarget = useMemo<RemoteContributionResolutionTarget | null>(() => {
    if (!relation.providerHead || (!relation.canReplaceRemote && !relation.canUseRemote)) {
      return null;
    }
    const repo = repositoryName ?? "";
    return {
      expectedRemoteHead: relation.providerHead,
      repo,
      repositoryName: repo || t("task:remoteRepository"),
    };
  }, [relation, repositoryName, t]);
  return { resolution, confirmResolution, resolutionTarget };
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
  const { relation, repositoryName } = useRemoteContributionRelation(sessionId);
  const remoteActionPolicy = remoteContributionActionPolicy(relation);
  const { resolution, confirmResolution, resolutionTarget } = useVcsContributionResolution(
    sessionId,
    relation,
    repositoryName,
  );
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
  const showContributionResolution = relation.action === "diverged_replace";
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

  return (
    <VcsSplitButtonContent
      isMultiRepo={isMultiRepo}
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
      showContributionResolution={showContributionResolution}
      replaceDisabled={remoteActionPolicy.replaceDisabled}
      useDisabled={remoteActionPolicy.useDisabled}
      repoNames={git.repoNames}
      perRepoStatus={git.perRepoStatus}
      blockedRepositoryName={repositoryName}
      repoDisplayName={repoDisplayName}
      buttonSize={buttonSize}
      className={className}
      showDivergencePills={showDivergencePills}
      callbacks={{
        onCommit: (repo) => openCommitDialog(repo),
        onPR: (repo) => openPRDialog(repo),
        onPull: handlePull,
        onPush: handlePush,
        onRebase: handleRebase,
        onMerge: handleMerge,
        onReplaceContribution: (repo) => {
          if (resolutionTarget && resolutionTarget.repo === repo) {
            resolution.requestReplace(resolutionTarget);
          }
        },
        onUseContribution: (repo) => {
          if (resolutionTarget && resolutionTarget.repo === repo) {
            resolution.requestUse(resolutionTarget);
          }
        },
        onViewContribution: (repo) => {
          if (resolutionTarget?.repo !== repo || !activePR?.pr_url) return;
          void openExternalLink(activePR.pr_url).catch(() => undefined);
        },
      }}
      resolution={resolution}
      resolutionTarget={resolutionTarget}
      confirmResolution={confirmResolution}
    />
  );
});

export { VcsSplitButton };
