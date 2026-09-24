"use client";

import { useEffect, useMemo, useRef } from "react";
import {
  RepositoryOptions,
  RepositoryOptionsSummary,
} from "./task-create-dialog-repository-options";
import type { RepositoryCheckoutOptions } from "@/lib/types/repository-checkout-options";
import { IconGitBranch, IconX } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Branch } from "@/lib/types/http";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Pill } from "@/components/task-create-dialog-pill";
import {
  branchToOption,
  sortBranches,
  computeBranchPlaceholder,
} from "@/components/branch-picker-options";
import { scoreBranch } from "@/lib/utils/branch-filter";
import type {
  RemoteRepositoryProvider,
  UseRemoteRepositoriesResult,
} from "@/hooks/domains/integrations/use-remote-repositories";
import type { PRInfo } from "@/hooks/domains/github/use-pr-info-by-url";
import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";
import { RemoteProviderConnectionError } from "./task-create-dialog-remote-provider-error";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { RemoteRepoPill } from "./task-create-dialog-remote-repo-picker";

export { selectedRemoteRepositoryIdentity } from "./task-create-dialog-remote-repo-identity";
export { computeTriggerLabel } from "./task-create-dialog-remote-repo-picker";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

export type RemoteRepoChipProps = {
  workspaceId?: string | null;
  executorProfileId?: string;
  onOptionsChange?: (options?: RepositoryCheckoutOptions) => void;
  row: TaskRemoteRepoRow;
  branches: Branch[];
  branchesLoading: boolean;
  prInfo?: PRInfo;
  resolutionError?: Error;
  /** The provider that supplied this picker row is no longer ready. */
  connectionUnavailable?: boolean;
  accessibleRepos: UseRemoteRepositoriesResult;
  /** Identities selected by other rows. Matching entries remain selectable. */
  selectedRepositoryIdentities?: string[];
  onURLChange: (
    url: string,
    source: "picker" | "paste",
    metadata?: {
      provider: RemoteRepositoryProvider;
      fullName: string;
      defaultBranch: string;
      remoteUrl?: string;
      providerHost?: string;
      providerScope?: string;
      providerRepoId?: string;
      providerOwner?: string;
      providerName?: string;
    },
  ) => void;
  onBranchChange: (branch: string) => void;
  onRetry?: () => void;
  onRemove: () => void;
  repositoryLocked?: boolean;
  branchLocked?: boolean;
};

/**
 * Single chip in the Remote tab. Layout mirrors `RepoChip`:
 *
 *     [ repo pill ] [ branch pill ] [X]
 *
 * The repo pill opens a custom popover with one input that searches the
 * user's accessible GitHub repos or accepts a pasted GitHub URL.
 * The branch pill is the shared `Pill` primitive over the per-URL branches
 * the parent loads via `branchesByUrl`.
 */
export function RemoteRepoChip({
  workspaceId,
  executorProfileId,
  onOptionsChange,
  row,
  branches,
  branchesLoading,
  prInfo,
  resolutionError,
  connectionUnavailable = false,
  accessibleRepos,
  selectedRepositoryIdentities = [],
  onURLChange,
  onBranchChange,
  onRetry,
  onRemove,
  repositoryLocked,
  branchLocked,
}: RemoteRepoChipProps) {
  useRowBranchAutoSelect({ row, branches, prInfo, onBranchChange });
  return (
    <div
      className="flex min-w-0 max-w-full flex-col items-start gap-1"
      data-testid="remote-repo-chip-wrapper"
    >
      <span
        className="grid max-w-full grid-cols-[minmax(0,1fr)_auto_auto] items-center rounded-md border border-input bg-input/20 dark:bg-input/30 pr-0.5 md:inline-flex"
        data-testid="remote-repo-chip"
        data-remote-url={row.url}
      >
        <RemoteRepoPill
          row={row}
          accessibleRepos={accessibleRepos}
          selectedRepositoryIdentities={selectedRepositoryIdentities}
          onURLChange={onURLChange}
          disabled={repositoryLocked}
        />
        <RemoteBranchPill
          url={row.url}
          branch={row.branch}
          branches={branches}
          branchesLoading={branchesLoading}
          onBranchChange={onBranchChange}
          branchLocked={branchLocked}
        />
        {onOptionsChange && row.url.trim() && (
          <RepositoryOptions
            key={row.url}
            row={row}
            workspaceId={workspaceId}
            executorProfileId={executorProfileId}
            onChange={onOptionsChange}
          />
        )}
        {repositoryLocked ? null : <RemoveButton onRemove={onRemove} />}
      </span>
      <RepositoryOptionsSummary options={row.checkoutOptions} />
      <RemoteRepoError
        connectionUnavailable={connectionUnavailable}
        resolutionError={resolutionError}
        onRetry={onRetry}
      />
    </div>
  );
}

function RemoteRepoError({
  connectionUnavailable,
  resolutionError,
  onRetry,
}: {
  connectionUnavailable: boolean;
  resolutionError?: Error;
  onRetry?: () => void;
}) {
  if (connectionUnavailable && onRetry) {
    return <RemoteProviderConnectionError onRetry={onRetry} />;
  }
  if (resolutionError && onRetry) {
    return <RemoteResolutionError error={resolutionError} onRetry={onRetry} />;
  }
  return null;
}

function RemoteResolutionError({ error, onRetry }: { error: Error; onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <span className="flex max-w-full items-center gap-2 text-xs text-destructive" role="alert">
      <span className="min-w-0 break-words">
        {t("task:couldNotResolveRemoteRepository", { message: error.message })}
      </span>
      <Button
        type="button"
        variant="outline"
        className="cursor-pointer"
        aria-label={t("task:retryRemoteRepositoryResolution")}
        onClick={onRetry}
      >
        {t("task:retry")}
      </Button>
    </span>
  );
}

/**
 * Per-row branch autoselect for the Remote tab. Runs whenever the row's
 * URL / PR-info / branch list changes.
 *
 * Order of preference when the auto-selector is allowed to write:
 *   1. PR head branch (when the row's URL is a PR URL and PR info has
 *      loaded). Wins regardless of whether the head appears in the base
 *      repo's branch list — fork PRs keep the head name surfaced on the
 *      pill even though `origin` can't resolve it.
 *   2. `main` / `master` / first available, from the per-URL branch list.
 *
 * The PR head must outrank a list-derived default even when the branch LIST
 * resolves before the PR info: if the list resolves first and the auto-selector
 * writes `main`, the later-arriving PR head must still replace it. A naive
 * `if (row.branch) return` guard breaks this — it bails once `main` is set.
 *
 * To distinguish "the auto-selector set this" from "the user picked this", we
 * track the last value the auto-selector wrote in `autoSetRef`. The auto-
 * selector may overwrite `row.branch` only when it is empty OR equals the last
 * value we wrote; a value that differs from the ref means the user picked it,
 * and we never clobber a user pick. When the row's URL is empty the effect is a
 * no-op.
 */
function useRowBranchAutoSelect(args: {
  row: TaskRemoteRepoRow;
  branches: Branch[];
  prInfo?: PRInfo;
  onBranchChange: (branch: string) => void;
}) {
  const { row, branches, prInfo, onBranchChange } = args;
  // Last branch value this auto-selector wrote. Used to tell an auto-set value
  // (safe to overwrite) apart from a user pick (must be preserved).
  const autoSetRef = useRef<string | null>(null);
  // The URL the autoSetRef belongs to. When the row switches to a different
  // repo/URL, ownership resets — otherwise a branch prefilled for the new URL
  // (e.g. its default_branch) could be mistaken for an auto-set value and
  // clobbered, or a stale value could leak across repos.
  const lastUrlRef = useRef<string>("");
  useEffect(() => {
    if (!row.url) return;
    if (row.url !== lastUrlRef.current) {
      lastUrlRef.current = row.url;
      autoSetRef.current = null;
    }
    // A non-empty branch that we didn't write ourselves is a user pick — leave
    // it alone.
    if (row.branch && row.branch !== autoSetRef.current) return;
    const desired = computeAutoSelectedBranch(prInfo, branches);
    if (!desired) return;
    if (desired === row.branch) {
      // Already on the desired value (e.g. we wrote it on a prior run); just
      // make sure the ref reflects it so a later user pick is detectable.
      autoSetRef.current = desired;
      return;
    }
    autoSetRef.current = desired;
    onBranchChange(desired);
  }, [row.url, row.branch, prInfo, branches, onBranchChange]);
}

// computeAutoSelectedBranch returns the branch the auto-selector wants for a
// row: the PR head branch (when known) outranks a list-derived default
// (main → master → first available). Returns "" when nothing can be chosen yet.
function computeAutoSelectedBranch(prInfo: PRInfo | undefined, branches: Branch[]): string {
  if (prInfo?.prHeadBranch) return prInfo.prHeadBranch;
  if (branches.length === 0) return "";
  const preferred =
    branches.find((b) => b.name === "main") ??
    branches.find((b) => b.name === "master") ??
    branches[0];
  return preferred?.name ?? "";
}

// --- Branch pill -------------------------------------------------------------

function RemoteBranchPill({
  url,
  branch,
  branches,
  branchesLoading,
  onBranchChange,
  branchLocked,
}: {
  url: string;
  branch: string;
  branches: Branch[];
  branchesLoading: boolean;
  onBranchChange: (branch: string) => void;
  branchLocked?: boolean;
}) {
  const { t } = useTranslation();
  const hasUrl = !!url.trim();
  const hasBranch = !!branch.trim();
  const branchOptions = useMemo(() => sortBranches(branches).map(branchToOption), [branches]);
  const placeholder = computeBranchPlaceholder(hasUrl, branchesLoading, branchOptions.length);
  // If the row already has a branch (e.g. pre-filled with the repo's
  // default_branch from a picker selection), keep the pill enabled so the
  // user sees the value as the active selection and can still re-open the
  // dropdown to swap branches once the list loads. The pill's own popover
  // will show "loading" / "no branches" if the list isn't ready yet.
  const disabled =
    branchLocked || !hasUrl || (!hasBranch && (branchesLoading || branchOptions.length === 0));
  return (
    <Pill
      icon={<IconGitBranch className="h-3 w-3 shrink-0 text-muted-foreground" />}
      value={branch}
      placeholder={placeholder}
      options={branchOptions}
      onSelect={onBranchChange}
      disabled={disabled}
      disabledReason={
        branchLocked
          ? t("task:branchLocked")
          : computeRemoteBranchDisabledReason(
              hasUrl,
              hasBranch,
              branchesLoading,
              branchOptions.length,
            )
      }
      searchPlaceholder={t("task:searchBranches")}
      emptyMessage={branchesLoading ? t("task:loadingBranches") : t("task:noBranches")}
      testId="remote-branch-chip-trigger"
      filter={scoreBranch}
      tooltip={t("task:baseBranch")}
      mobileTitle={t("task:branch")}
      flat
    />
  );
}

function computeRemoteBranchDisabledReason(
  hasUrl: boolean,
  hasBranch: boolean,
  branchesLoading: boolean,
  optionCount: number,
): string | undefined {
  if (!hasUrl) return t("task:selectOrEnterRemoteRepoFirst");
  // If a branch is already set the pill is enabled; no disabled reason needed.
  if (hasBranch) return undefined;
  if (branchesLoading) return t("task:loadingBranches3");
  if (optionCount === 0) return t("task:noBranchesForUrl");
  return undefined;
}

// --- Remove button -----------------------------------------------------------

function RemoveButton({ onRemove }: { onRemove: () => void }) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onRemove}
          aria-label={t("task:removeRepository")}
          data-testid="remote-chip-remove"
          className={cn(
            "h-6 w-6 inline-flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-muted/60 cursor-pointer",
            usesTouchDrawer && "min-h-11 min-w-11",
          )}
        >
          <IconX className="h-3 w-3" />
        </button>
      </TooltipTrigger>
      <TooltipContent>{t("task:removeRepository")}</TooltipContent>
    </Tooltip>
  );
}
