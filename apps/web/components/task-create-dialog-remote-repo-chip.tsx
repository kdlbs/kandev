"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { IconBrandGithub, IconGitBranch, IconLink, IconX } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Branch } from "@/lib/types/http";
import { Badge } from "@kandev/ui/badge";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Spinner } from "@kandev/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  Pill,
  branchToOption,
  computeBranchPlaceholder,
  sortBranches,
} from "@/components/task-create-dialog-pill";
import { scoreBranch } from "@/lib/utils/branch-filter";
import type { UseAccessibleReposResult } from "@/hooks/domains/github/use-accessible-repos";
import type { AccessibleRepo } from "@/lib/api/domains/github-api";
import type { PRInfo } from "@/hooks/domains/github/use-pr-info-by-url";
import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";

const TRUNCATE_THRESHOLD = 30;

/**
 * Props for the per-row remote-repo chip used in the Remote tab of the
 * task-create dialog. The chip itself is presentational — branches and the
 * loading flag are passed in by the parent row (which keys them off the
 * row's URL via `branchesByUrl`), and writes happen through the supplied
 * callbacks.
 *
 * `onURLChange` receives the new URL plus how it was produced. The "picker"
 * arm also carries the canonical `owner/name`, provider, and the repo's
 * `default_branch` so the parent can pre-fill the row's branch without
 * waiting for the branch list to load; the "paste" arm leaves metadata
 * undefined so the row drops any stale picker data and the user picks
 * their own branch.
 */
export type RemoteRepoChipProps = {
  row: TaskRemoteRepoRow;
  branches: Branch[];
  branchesLoading: boolean;
  /**
   * PR info for the row's URL (when the URL is a PR URL and the info has
   * loaded). Drives the per-row PR-head auto-select effect: if the row's
   * branch is empty, the chip writes the PR head branch into it. The
   * dialog separately reads the first row's `suggestedTitle` to autofill
   * the task title.
   */
  prInfo?: PRInfo;
  /**
   * Shared `useAccessibleRepos` result hoisted up to the chips-row level so
   * one backend request serves every chip in the row (previously each open
   * popover fired its own request). Each chip still keeps its own local
   * search-text state — the hoisted hook only owns the in-flight fetch and
   * the cache. When two popovers happen to be open at once with different
   * search texts, both see the same `repos` (the last `search(q)` wins);
   * that's acceptable because in practice only one popover is open at a
   * time.
   */
  accessibleRepos: UseAccessibleReposResult;
  onURLChange: (
    url: string,
    source: "picker" | "paste",
    metadata?: {
      provider: "github" | "gitlab";
      fullName: string;
      defaultBranch: string;
    },
  ) => void;
  onBranchChange: (branch: string) => void;
  onRemove: () => void;
};

/**
 * Single chip in the Remote tab. Layout mirrors `RepoChip`:
 *
 *     [ repo pill ] [ branch pill ] [X]
 *
 * The repo pill opens a custom popover with two sections (an autocomplete
 * search over the user's accessible GitHub repos, and a paste-a-URL input).
 * The branch pill is the shared `Pill` primitive over the per-URL branches
 * the parent loads via `branchesByUrl`.
 */
export function RemoteRepoChip({
  row,
  branches,
  branchesLoading,
  prInfo,
  accessibleRepos,
  onURLChange,
  onBranchChange,
  onRemove,
}: RemoteRepoChipProps) {
  useRowBranchAutoSelect({ row, branches, prInfo, onBranchChange });
  return (
    <span
      className="inline-flex items-center rounded-md border border-input bg-input/20 dark:bg-input/30 pr-0.5"
      data-testid="remote-repo-chip"
      data-remote-url={row.url}
    >
      <RemoteRepoPill row={row} accessibleRepos={accessibleRepos} onURLChange={onURLChange} />
      <RemoteBranchPill
        url={row.url}
        branch={row.branch}
        branches={branches}
        branchesLoading={branchesLoading}
        onBranchChange={onBranchChange}
      />
      <RemoveButton onRemove={onRemove} />
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
  useEffect(() => {
    if (!row.url) return;
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
  onURLChange,
}: {
  row: TaskRemoteRepoRow;
  accessibleRepos: UseAccessibleReposResult;
  onURLChange: RemoteRepoChipProps["onURLChange"];
}) {
  const [open, setOpen] = useState(false);
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
      <PopoverContent className="w-[380px] p-0" align="start" portal={false}>
        <RemoteRepoPopoverContent
          accessible={accessibleRepos}
          onPick={(repo) => {
            onURLChange(`https://github.com/${repo.owner}/${repo.name}`, "picker", {
              provider: "github",
              fullName: repo.full_name,
              defaultBranch: repo.default_branch,
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
  if (row.source === "picker" && row.provider === "github") {
    return <IconBrandGithub className="h-3 w-3 shrink-0 text-muted-foreground" />;
  }
  return <IconLink className="h-3 w-3 shrink-0 text-muted-foreground" />;
}

export function computeTriggerLabel(row: TaskRemoteRepoRow): string {
  if (!row.url) return "Pick or paste a repo";
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

function RemoteRepoPopoverContent({
  accessible,
  onPick,
  onPaste,
}: {
  accessible: UseAccessibleReposResult;
  onPick: (repo: AccessibleRepo) => void;
  onPaste: (value: string) => void;
}) {
  const [search, setSearch] = useState("");
  // Destructure the stable `search` callback so the effect's dep array is
  // not invalidated on every render of the parent (the hook's result object
  // identity changes each render, but `search` is a useCallback). The hook
  // itself owns the 250ms debounce so we just forward the latest value.
  const { search: triggerSearch } = accessible;
  useEffect(() => {
    triggerSearch(search);
  }, [search, triggerSearch]);
  return (
    <div className="flex flex-col">
      <PickerSection
        accessible={accessible}
        search={search}
        onSearchChange={setSearch}
        onPick={onPick}
      />
      <div className="border-t" />
      <PasteSection onPaste={onPaste} />
    </div>
  );
}

function PickerSection({
  accessible,
  search,
  onSearchChange,
  onPick,
}: {
  accessible: UseAccessibleReposResult;
  search: string;
  onSearchChange: (v: string) => void;
  onPick: (repo: AccessibleRepo) => void;
}) {
  if (accessible.unavailable) return <ConnectGitHubBanner />;
  return (
    <div className="flex flex-col">
      <input
        type="text"
        value={search}
        onChange={(e) => onSearchChange(e.target.value)}
        placeholder="Search your GitHub repos…"
        data-testid="remote-repo-search"
        className={cn(
          "h-9 mx-2 mt-2 rounded-md px-2 text-xs bg-muted/30 border border-border/60",
          "outline-none focus:bg-muted focus:border-border placeholder:text-muted-foreground",
        )}
      />
      <PickerList accessible={accessible} onPick={onPick} />
    </div>
  );
}

function PickerList({
  accessible,
  onPick,
}: {
  accessible: UseAccessibleReposResult;
  onPick: (repo: AccessibleRepo) => void;
}) {
  const { repos, loading, error } = accessible;
  return (
    <div className="max-h-56 overflow-y-auto p-1">
      {loading && repos.length === 0 ? (
        <div
          className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground"
          data-testid="remote-repo-picker-loading"
        >
          <Spinner className="size-3" />
          <span>Loading repositories…</span>
        </div>
      ) : null}
      {!loading && repos.length === 0 && !error ? (
        <div className="px-2 py-3 text-xs text-muted-foreground">No repositories found.</div>
      ) : null}
      {error ? (
        <div className="px-2 py-3 text-xs text-destructive">
          Could not load repositories: {error.message}
        </div>
      ) : null}
      {repos.map((repo) => (
        <RepoOption key={repo.full_name} repo={repo} onPick={onPick} />
      ))}
    </div>
  );
}

function RepoOption({
  repo,
  onPick,
}: {
  repo: AccessibleRepo;
  onPick: (repo: AccessibleRepo) => void;
}) {
  return (
    <button
      type="button"
      onClick={() => onPick(repo)}
      data-testid="remote-repo-option"
      className={cn(
        "flex w-full items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-xs",
        "hover:bg-muted cursor-pointer text-left",
      )}
    >
      <span className="truncate min-w-0">{repo.full_name}</span>
      {repo.private ? (
        <Badge variant="outline" className="text-[10px] text-muted-foreground shrink-0">
          private
        </Badge>
      ) : null}
    </button>
  );
}

function ConnectGitHubBanner() {
  return (
    <div className="px-3 py-3 text-xs text-muted-foreground">
      Connect a GitHub account in{" "}
      <Link
        href="/settings/integrations/github"
        className="text-foreground underline underline-offset-2 cursor-pointer"
      >
        Settings
      </Link>{" "}
      to pick from your repositories.
    </div>
  );
}

function PasteSection({ onPaste }: { onPaste: (value: string) => void }) {
  const [value, setValue] = useState("");
  const commit = () => {
    const trimmed = value.trim();
    if (!trimmed) return;
    onPaste(trimmed);
  };
  return (
    <div className="flex flex-col gap-1 p-2">
      <label className="text-[11px] text-muted-foreground" htmlFor="remote-paste-url-input">
        …or paste a URL/PR
      </label>
      <input
        id="remote-paste-url-input"
        type="text"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onBlur={(e) => {
          // If focus moved to another element inside this popover (e.g., a
          // picker option click), let that handler run instead of committing
          // the paste — otherwise the popover would close before the click
          // lands and the user's pick would be lost.
          const popoverContent = e.currentTarget.closest('[data-slot="popover-content"]');
          if (
            popoverContent &&
            e.relatedTarget instanceof Node &&
            popoverContent.contains(e.relatedTarget)
          ) {
            return;
          }
          commit();
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            commit();
          }
        }}
        placeholder="github.com/owner/repo or .../pull/123"
        data-testid="remote-paste-url-input"
        className={cn(
          "h-8 rounded-md px-2 text-xs bg-muted/30 border border-border/60",
          "outline-none focus:bg-muted focus:border-border placeholder:text-muted-foreground",
        )}
      />
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
      searchPlaceholder="Search branches..."
      emptyMessage={branchesLoading ? "Loading branches…" : "No branches"}
      testId="remote-branch-chip-trigger"
      filter={scoreBranch}
      tooltip="Base branch"
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
  if (!hasUrl) return "Enter a GitHub URL first.";
  // If a branch is already set the pill is enabled; no disabled reason needed.
  if (hasBranch) return undefined;
  if (branchesLoading) return "Loading branches…";
  if (optionCount === 0) return "No branches available for this URL.";
  return undefined;
}

// --- Remove button -----------------------------------------------------------

function RemoveButton({ onRemove }: { onRemove: () => void }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onRemove}
          aria-label="Remove repository"
          data-testid="remote-chip-remove"
          className="h-6 w-6 inline-flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-muted/60 cursor-pointer"
        >
          <IconX className="h-3 w-3" />
        </button>
      </TooltipTrigger>
      <TooltipContent>Remove repository</TooltipContent>
    </Tooltip>
  );
}
