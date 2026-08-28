"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Link from "@/components/routing/app-link";
import { IconCheck, IconGitBranch, IconLink, IconX } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Branch } from "@/lib/types/http";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Spinner } from "@kandev/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Pill } from "@/components/task-create-dialog-pill";
import {
  branchToOption,
  computeBranchPlaceholder,
  sortBranches,
} from "@/components/task-create-dialog-branch-options";
import { scoreBranch } from "@/lib/utils/branch-filter";
import type {
  RemoteRepository,
  RemoteRepositoryProvider,
  UseRemoteRepositoriesResult,
} from "@/hooks/domains/integrations/use-remote-repositories";
import { parseGitHubAnyUrl, type PRInfo } from "@/hooks/domains/github/use-pr-info-by-url";
import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import {
  RemoteRepositoryProviderIcon,
  RemoteRepoProviderTabs,
} from "@/components/task-create-dialog-remote-repo-provider-tabs";
import { remoteRepositoryMatchesSelection } from "./task-create-dialog-remote-repo-identity";

export { selectedRemoteRepositoryIdentity } from "./task-create-dialog-remote-repo-identity";
import {
  looksLikeURL,
  looksLikeSupportedRemoteURL,
} from "@/components/workspace-source-picker/remote-url";
import { Trans, useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

const TRUNCATE_THRESHOLD = 30;

export type RemoteRepoChipProps = {
  row: TaskRemoteRepoRow;
  branches: Branch[];
  branchesLoading: boolean;
  prInfo?: PRInfo;
  resolutionError?: Error;
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
  row,
  branches,
  branchesLoading,
  prInfo,
  resolutionError,
  accessibleRepos,
  selectedRepositoryIdentities = [],
  onURLChange,
  onBranchChange,
  onRetry,
  onRemove,
}: RemoteRepoChipProps) {
  useRowBranchAutoSelect({ row, branches, prInfo, onBranchChange });
  return (
    <div
      className="flex max-w-full flex-col items-start gap-1"
      data-testid="remote-repo-chip-wrapper"
    >
      <span
        className="inline-flex max-w-full items-center rounded-md border border-input bg-input/20 dark:bg-input/30 pr-0.5"
        data-testid="remote-repo-chip"
        data-remote-url={row.url}
      >
        <RemoteRepoPill
          row={row}
          accessibleRepos={accessibleRepos}
          selectedRepositoryIdentities={selectedRepositoryIdentities}
          onURLChange={onURLChange}
        />
        <RemoteBranchPill
          url={row.url}
          branch={row.branch}
          branches={branches}
          branchesLoading={branchesLoading}
          onBranchChange={onBranchChange}
        />
        <RemoveButton onRemove={onRemove} />
      </span>
      {resolutionError && onRetry ? (
        <RemoteResolutionError error={resolutionError} onRetry={onRetry} />
      ) : null}
    </div>
  );
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
        size="sm"
        className="h-11 sm:h-9 cursor-pointer"
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

// --- Repo pill ---------------------------------------------------------------

function RemoteRepoPill({
  row,
  accessibleRepos,
  selectedRepositoryIdentities,
  onURLChange,
}: {
  row: TaskRemoteRepoRow;
  accessibleRepos: UseRemoteRepositoriesResult;
  selectedRepositoryIdentities: string[];
  onURLChange: RemoteRepoChipProps["onURLChange"];
}) {
  const [open, setOpen] = useState(false);
  const portalContainer = useTaskCreateDialogPopoverContainer();
  const triggerLabel = useMemo(() => computeTriggerLabel(row), [row]);
  const hasValue = !!row.url;
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          data-testid="remote-repo-chip-trigger"
          className={cn(
            "h-7 inline-flex items-center gap-1.5 rounded-md px-2.5 text-xs bg-transparent",
            "hover:bg-muted/60 cursor-pointer",
            !hasValue && "text-muted-foreground",
          )}
        >
          <RepoTriggerIcon row={row} />
          <span className="truncate max-w-[240px]">{triggerLabel}</span>
        </button>
      </PopoverTrigger>
      <PopoverContent
        data-testid="remote-repo-popover-content"
        className="w-[380px] max-w-[calc(100vw-2rem)] max-h-[min(420px,calc(100vh-12rem))] overflow-hidden p-0"
        align="start"
        portalContainer={portalContainer}
      >
        <RemoteRepoPopoverContent
          accessible={accessibleRepos}
          selectedRepositoryIdentities={selectedRepositoryIdentities}
          onPick={(repo) => {
            onURLChange(repo.url, "picker", {
              provider: repo.provider,
              fullName: repo.fullName,
              defaultBranch: repo.defaultBranch,
              ...(repo.provider === "github" ? {} : { remoteUrl: repo.url }),
              ...(repo.providerHost ? { providerHost: repo.providerHost } : {}),
              ...(repo.providerScope ? { providerScope: repo.providerScope } : {}),
              providerRepoId: repo.id,
              providerOwner: repo.owner,
              providerName: repo.name,
            });
            setOpen(false);
          }}
          onPaste={(value) => {
            onURLChange(value, "paste");
            setOpen(false);
          }}
        />
      </PopoverContent>
    </Popover>
  );
}

function RepoTriggerIcon({ row }: { row: TaskRemoteRepoRow }) {
  if (row.source === "picker" && row.provider) {
    return <RemoteRepositoryProviderIcon provider={row.provider} />;
  }
  return <IconLink className="h-3 w-3 shrink-0 text-muted-foreground" />;
}

export function computeTriggerLabel(row: TaskRemoteRepoRow): string {
  if (!row.url) return t("task:pickOrPasteARepo");
  if (row.source === "picker" && row.fullName) return row.fullName;
  return truncateMiddle(stripScheme(row.url), TRUNCATE_THRESHOLD);
}

function stripScheme(url: string): string {
  return url.replace(/^https?:\/\//, "").replace(/^www\./, "");
}

function truncateMiddle(value: string, max: number): string {
  if (value.length <= max) return value;
  const keep = Math.max(1, Math.floor((max - 1) / 2));
  return `${value.slice(0, keep)}…${value.slice(value.length - keep)}`;
}

// --- Popover content ---------------------------------------------------------

function StagedRemoteUrlHint() {
  return (
    <div className="px-2 pt-1 text-xs text-muted-foreground">
      <Trans i18nKey="task:remoteUrlPressEnter">
        <span className="font-medium text-foreground">Remote URL</span> - press Enter to submit it.
      </Trans>
    </div>
  );
}

function RemoteRepoPopoverContent({
  accessible,
  selectedRepositoryIdentities,
  onPick,
  onPaste,
}: {
  accessible: UseRemoteRepositoriesResult;
  selectedRepositoryIdentities: string[];
  onPick: (repo: RemoteRepository) => void;
  onPaste: (value: string) => void;
}) {
  const { t } = useTranslation();
  const [value, setValue] = useState("");
  const [urlError, setUrlError] = useState<string | null>(null);
  const [activeProvider, setActiveProvider] = useState<RemoteRepositoryProvider | null>(null);
  const { search: triggerSearch } = accessible;
  const matchesURL = accessible.matchesURL ?? looksLikeSupportedRemoteURL;
  useEffect(() => {
    triggerSearch(value);
  }, [value, triggerSearch]);
  const commitURL = (candidate: string) => {
    const trimmed = candidate.trim();
    if (!isSupportedRemoteURL(trimmed, matchesURL)) {
      if (looksLikeURL(trimmed)) {
        setUrlError(t("task:enterRepositoryUrl"));
      }
      return false;
    }
    setUrlError(null);
    onPaste(trimmed);
    return true;
  };
  const visibleUrlError = accessible.unavailable ? null : urlError;
  const hasStagedURL = isSupportedRemoteURL(value.trim(), matchesURL);
  const { showProviderTabs, selectedProvider, visibleRepos } = visibleProviderRepositories(
    accessible,
    activeProvider,
  );
  return (
    <div className="flex flex-col">
      <input
        autoFocus
        value={value}
        onChange={(event) => {
          setValue(event.target.value);
          setUrlError(null);
        }}
        onPaste={(event) => {
          const pasted = event.clipboardData.getData("text");
          event.preventDefault();
          setValue(pasted);
        }}
        onKeyDown={(event) => {
          if (
            event.key !== "Enter" ||
            event.defaultPrevented ||
            event.repeat ||
            event.altKey ||
            event.ctrlKey ||
            event.metaKey ||
            event.shiftKey ||
            event.nativeEvent.isComposing ||
            event.keyCode === 229
          )
            return;
          const isURL = looksLikeURL(value.trim());
          if (commitURL(value) || isURL) event.preventDefault();
        }}
        placeholder={t("task:searchRepositoriesOrPasteARemote")}
        aria-label={t("task:searchRepositoriesOrPasteARemote")}
        aria-invalid={visibleUrlError ? true : undefined}
        data-testid="remote-repo-input"
        data-legacy-testid="remote-paste-url-input"
        className={cn(
          "h-11 sm:h-9 mx-2 mt-2 rounded-md px-2 text-xs bg-muted/30 border border-border/60",
          "outline-none focus:bg-muted focus:border-border placeholder:text-muted-foreground",
          visibleUrlError && "border-destructive focus:border-destructive",
        )}
      />
      {hasStagedURL ? <StagedRemoteUrlHint /> : null}
      <PickerList
        accessible={{ ...accessible, repos: visibleRepos }}
        selectedRepositoryIdentities={selectedRepositoryIdentities}
        onPick={onPick}
        urlError={visibleUrlError}
      />
      {showProviderTabs && selectedProvider ? (
        <RemoteRepoProviderTabs
          providers={accessible.availableProviders}
          value={selectedProvider}
          onChange={setActiveProvider}
        />
      ) : null}
    </div>
  );
}

function visibleProviderRepositories(
  accessible: UseRemoteRepositoriesResult,
  activeProvider: RemoteRepositoryProvider | null,
) {
  const showProviderTabs = accessible.availableProviders.length > 1;
  const selectedProvider =
    activeProvider && accessible.availableProviders.includes(activeProvider)
      ? activeProvider
      : accessible.availableProviders[0];
  const visibleRepos = showProviderTabs
    ? accessible.repos.filter((repo) => repo.provider === selectedProvider)
    : accessible.repos;
  return { showProviderTabs, selectedProvider, visibleRepos };
}
function isSupportedRemoteURL(value: string, matchesURL: (url: string) => boolean): boolean {
  return !!parseGitHubAnyUrl(value) || matchesURL(value);
}
function PickerList({
  accessible,
  selectedRepositoryIdentities,
  onPick,
  urlError,
}: {
  accessible: UseRemoteRepositoriesResult;
  selectedRepositoryIdentities: string[];
  onPick: (repo: RemoteRepository) => void;
  urlError: string | null;
}) {
  const { t } = useTranslation();
  const { repos, loading, error } = accessible;
  return (
    <div className="h-56 max-h-[calc(100vh-16rem)] overflow-y-auto p-1">
      {urlError ? (
        <div role="alert" className="px-2 py-3 text-xs text-destructive">
          {urlError}
        </div>
      ) : null}
      {accessible.unavailable ? <ConnectProvidersBanner /> : null}
      {!accessible.unavailable && loading && repos.length === 0 ? (
        <div
          className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground"
          data-testid="remote-repo-picker-loading"
        >
          <Spinner className="size-3" />
          <span>{t("task:loadingRepositories")}</span>
        </div>
      ) : null}
      {!accessible.unavailable && !loading && repos.length === 0 && !error ? (
        <div className="px-2 py-3 text-xs text-muted-foreground">
          {t("task:noRepositoriesFound")}
        </div>
      ) : null}
      {error ? (
        <div className="px-2 py-3 text-xs text-destructive">
          {t("task:couldNotLoadRepositories", { message: error.message })}
        </div>
      ) : null}
      {repos.map((repo) => (
        <RepoOption
          key={`${repo.provider}:${repo.id}`}
          repo={repo}
          alreadyAdded={selectedRepositoryIdentities.some((identity) =>
            remoteRepositoryMatchesSelection(repo, identity),
          )}
          onPick={onPick}
        />
      ))}
    </div>
  );
}

function RepoOption({
  repo,
  alreadyAdded,
  onPick,
}: {
  repo: RemoteRepository;
  alreadyAdded: boolean;
  onPick: (repo: RemoteRepository) => void;
}) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      onClick={() => onPick(repo)}
      data-testid="remote-repo-option"
      className={cn(
        "flex min-h-11 sm:min-h-8 w-full items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-xs",
        "hover:bg-muted cursor-pointer text-left",
      )}
    >
      <span className="flex min-w-0 items-center gap-2">
        <RemoteRepositoryProviderIcon provider={repo.provider} />
        <span className="truncate">{repo.fullName}</span>
      </span>
      <span className="flex shrink-0 items-center gap-1">
        {alreadyAdded ? <AlreadyAddedMarker /> : null}
        {repo.private ? (
          <Badge variant="outline" className="text-[10px] text-muted-foreground">
            {t("task:private")}
          </Badge>
        ) : null}
      </span>
    </button>
  );
}

function AlreadyAddedMarker() {
  const { t } = useTranslation();
  return (
    <span
      role="img"
      aria-label={t("task:alreadyAdded")}
      data-testid="already-added-repository-marker"
      className="text-primary"
    >
      <IconCheck aria-hidden="true" className="h-4 w-4" />
    </span>
  );
}

function ConnectProvidersBanner() {
  return (
    <div className="px-3 py-3 text-xs text-muted-foreground">
      <Trans i18nKey="task:connectProviderBanner">
        Connect a source control provider in{" "}
        <Link
          href="/settings/integrations"
          className="text-foreground underline underline-offset-2 cursor-pointer"
        >
          Settings
        </Link>{" "}
        to pick from your repositories.
      </Trans>
    </div>
  );
}

// --- Branch pill -------------------------------------------------------------

function RemoteBranchPill({
  url,
  branch,
  branches,
  branchesLoading,
  onBranchChange,
}: {
  url: string;
  branch: string;
  branches: Branch[];
  branchesLoading: boolean;
  onBranchChange: (branch: string) => void;
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
  const disabled = !hasUrl || (!hasBranch && (branchesLoading || branchOptions.length === 0));
  return (
    <Pill
      icon={<IconGitBranch className="h-3 w-3 shrink-0 text-muted-foreground" />}
      value={branch}
      placeholder={placeholder}
      options={branchOptions}
      onSelect={onBranchChange}
      disabled={disabled}
      disabledReason={computeRemoteBranchDisabledReason(
        hasUrl,
        hasBranch,
        branchesLoading,
        branchOptions.length,
      )}
      searchPlaceholder={t("task:searchBranches")}
      emptyMessage={branchesLoading ? t("task:loadingBranches") : t("task:noBranches")}
      testId="remote-branch-chip-trigger"
      filter={scoreBranch}
      tooltip={t("task:baseBranch")}
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
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onRemove}
          aria-label={t("task:removeRepository")}
          data-testid="remote-chip-remove"
          className="h-6 w-6 inline-flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-muted/60 cursor-pointer"
        >
          <IconX className="h-3 w-3" />
        </button>
      </TooltipTrigger>
      <TooltipContent>{t("task:removeRepository")}</TooltipContent>
    </Tooltip>
  );
}
