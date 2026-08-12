"use client";

import { useTranslation } from "react-i18next";
import type { ComponentProps } from "react";
import { IconChevronDown, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DropdownMenu, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@kandev/ui/lib/utils";
import {
  MultiRepoVcsButton,
  type PerRepoCallbacks,
  type PerRepoStatus,
} from "./vcs-multi-repo-menu";
import { VcsDropdownItems } from "./vcs-split-button-dropdown";
import type { PrimaryButtonConfig } from "./vcs-split-button";
import {
  type RemoteContributionResolutionTarget,
  type useRemoteContributionResolution,
} from "@/components/task/use-remote-contribution-resolution";
import { RemoteContributionResolutionDialog } from "@/components/task/remote-contribution-resolution-dialog";

const DEFAULT_BASE_BRANCH = "origin/main";
const AHEAD_MARK = "↑";
const BEHIND_MARK = "↓";

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
      {tone === "ahead" ? AHEAD_MARK : BEHIND_MARK}
      {value}
    </span>
  );
}

export function GitDivergencePills({ ahead, behind }: { ahead: number; behind: number }) {
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

type SingleRepoVcsButtonProps = {
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
  showContributionResolution: boolean;
  replaceDisabled: boolean;
  useDisabled: boolean;
  onPR: () => void;
  onPull: () => void;
  onPush: (force: boolean) => void;
  onReplaceContribution: () => void;
  onUseContribution: () => void;
  onViewPRVersion: () => void;
  onRebase: () => void;
  onMerge: () => void;
  className?: string;
  buttonSize?: ComponentProps<typeof Button>["size"];
  showDivergencePills?: boolean;
};

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
  showContributionResolution,
  replaceDisabled,
  useDisabled,
  onPR,
  onPull,
  onPush,
  onReplaceContribution,
  onUseContribution,
  onViewPRVersion,
  onRebase,
  onMerge,
  className,
  buttonSize = "sm",
  showDivergencePills = false,
}: SingleRepoVcsButtonProps) {
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
          showContributionResolution={showContributionResolution}
          replaceDisabled={replaceDisabled}
          useDisabled={useDisabled}
          onPR={onPR}
          onPull={onPull}
          onPush={onPush}
          onReplaceContribution={onReplaceContribution}
          onUseContribution={onUseContribution}
          onViewPRVersion={onViewPRVersion}
          onRebase={onRebase}
          onMerge={onMerge}
        />
      </DropdownMenu>
    </div>
  );
}

type VcsSplitButtonCallbacks = {
  onCommit: (repo?: string) => void;
  onPR: (repo?: string) => void;
  onPull: (repo?: string) => void;
  onPush: (force: boolean, repo?: string) => void;
  onRebase: (repo?: string) => void;
  onMerge: (repo?: string) => void;
  onReplaceContribution: (repo?: string) => void;
  onUseContribution: (repo?: string) => void;
  onViewContribution: (repo?: string) => void;
};

type VcsSplitButtonContentProps = {
  isMultiRepo: boolean;
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
  showContributionResolution: boolean;
  replaceDisabled: boolean;
  useDisabled: boolean;
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  blockedRepositoryName?: string;
  repoDisplayName: (repositoryName: string) => string | undefined;
  buttonSize?: ComponentProps<typeof Button>["size"];
  className?: string;
  showDivergencePills: boolean;
  callbacks: VcsSplitButtonCallbacks;
  resolution: ReturnType<typeof useRemoteContributionResolution>;
  resolutionTarget: RemoteContributionResolutionTarget | null;
  confirmResolution: () => Promise<void>;
};

function ContributionResolutionDialog({
  resolution,
  resolutionTarget,
  confirmResolution,
}: Pick<VcsSplitButtonContentProps, "resolution" | "resolutionTarget" | "confirmResolution">) {
  if (!resolution.pending || !resolutionTarget) return null;
  return (
    <RemoteContributionResolutionDialog
      open
      action={resolution.pending.action}
      repositoryName={resolutionTarget.repositoryName ?? ""}
      expectedRemoteHead={resolution.pending.expectedRemoteHead}
      isLoading={resolution.isLoading}
      errorKey={resolution.errorKey}
      onOpenChange={(open) => {
        if (!open) resolution.cancel();
      }}
      onConfirm={() => {
        void confirmResolution();
      }}
    />
  );
}

function MultiRepoVcsContent({
  primaryButtonConfig,
  primaryAction,
  isDisabled,
  isGitLoading,
  baseBranch,
  repoNames,
  perRepoStatus,
  pushDisabled,
  pullDisabled,
  showContributionResolution,
  replaceDisabled,
  useDisabled,
  blockedRepositoryName,
  repoDisplayName,
  callbacks,
  resolution,
  resolutionTarget,
  confirmResolution,
}: Pick<
  VcsSplitButtonContentProps,
  | "primaryButtonConfig"
  | "primaryAction"
  | "isDisabled"
  | "isGitLoading"
  | "baseBranch"
  | "repoNames"
  | "perRepoStatus"
  | "pushDisabled"
  | "pullDisabled"
  | "showContributionResolution"
  | "replaceDisabled"
  | "useDisabled"
  | "blockedRepositoryName"
  | "repoDisplayName"
  | "callbacks"
  | "resolution"
  | "resolutionTarget"
  | "confirmResolution"
>) {
  const multiRepoCallbacks: PerRepoCallbacks = {
    onCommit: (repo) => callbacks.onCommit(repo),
    onPR: (repo) => callbacks.onPR(repo),
    onPull: (repo) => callbacks.onPull(repo),
    onPush: (force, repo) => callbacks.onPush(force, repo),
    onRebase: (repo) => callbacks.onRebase(repo),
    onMerge: (repo) => callbacks.onMerge(repo),
    onReplaceContribution: (repo) => callbacks.onReplaceContribution(repo),
    onUseContribution: (repo) => callbacks.onUseContribution(repo),
    onViewContribution: (repo) => callbacks.onViewContribution(repo),
  };
  return (
    <>
      <MultiRepoVcsButton
        primaryButtonConfig={primaryButtonConfig}
        primaryAction={primaryAction}
        isDisabled={isDisabled}
        isGitLoading={isGitLoading}
        baseBranch={baseBranch || DEFAULT_BASE_BRANCH}
        repoNames={repoNames}
        perRepoStatus={perRepoStatus}
        pushDisabled={pushDisabled}
        pullDisabled={pullDisabled}
        showContributionResolution={showContributionResolution}
        replaceDisabled={replaceDisabled}
        useDisabled={useDisabled}
        blockedRepositoryName={blockedRepositoryName}
        repoDisplayName={repoDisplayName}
        callbacks={multiRepoCallbacks}
      />
      <ContributionResolutionDialog
        resolution={resolution}
        resolutionTarget={resolutionTarget}
        confirmResolution={confirmResolution}
      />
    </>
  );
}

export function VcsSplitButtonContent({
  isMultiRepo,
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
  showContributionResolution,
  replaceDisabled,
  useDisabled,
  repoNames,
  perRepoStatus,
  blockedRepositoryName,
  repoDisplayName,
  buttonSize,
  className,
  showDivergencePills,
  callbacks,
  resolution,
  resolutionTarget,
  confirmResolution,
}: VcsSplitButtonContentProps) {
  if (isMultiRepo) {
    return (
      <MultiRepoVcsContent
        primaryButtonConfig={primaryButtonConfig}
        primaryAction={primaryAction}
        isDisabled={isDisabled}
        isGitLoading={isGitLoading}
        baseBranch={baseBranch}
        repoNames={repoNames}
        perRepoStatus={perRepoStatus}
        pushDisabled={pushDisabled}
        pullDisabled={pullDisabled}
        showContributionResolution={showContributionResolution}
        replaceDisabled={replaceDisabled}
        useDisabled={useDisabled}
        blockedRepositoryName={blockedRepositoryName}
        repoDisplayName={repoDisplayName}
        callbacks={callbacks}
        resolution={resolution}
        resolutionTarget={resolutionTarget}
        confirmResolution={confirmResolution}
      />
    );
  }

  return (
    <>
      <SingleRepoVcsButton
        primaryButtonConfig={primaryButtonConfig}
        primaryAction={primaryAction}
        isDisabled={isDisabled}
        isGitLoading={isGitLoading}
        baseBranch={baseBranch}
        hasUpstream={hasUpstream}
        behindCount={behindCount}
        aheadCount={aheadCount}
        pushDisabled={pushDisabled}
        pullDisabled={pullDisabled}
        showContributionResolution={showContributionResolution}
        replaceDisabled={replaceDisabled}
        useDisabled={useDisabled}
        buttonSize={buttonSize}
        className={className}
        showDivergencePills={showDivergencePills}
        onPR={() => callbacks.onPR()}
        onPull={() => callbacks.onPull()}
        onPush={(force) => callbacks.onPush(force)}
        onReplaceContribution={() => callbacks.onReplaceContribution()}
        onUseContribution={() => callbacks.onUseContribution()}
        onViewPRVersion={() => callbacks.onViewContribution()}
        onRebase={() => callbacks.onRebase()}
        onMerge={() => callbacks.onMerge()}
      />
      <ContributionResolutionDialog
        resolution={resolution}
        resolutionTarget={resolutionTarget}
        confirmResolution={confirmResolution}
      />
    </>
  );
}
