"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { IconBrandGithub, IconGitBranch, IconLink, IconX } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Branch } from "@/lib/types/http";
import { Badge } from "@kandev/ui/badge";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  Pill,
  branchToOption,
  computeBranchPlaceholder,
  sortBranches,
} from "@/components/task-create-dialog-pill";
import { computeUrlBranchDisabledReason } from "@/components/task-create-dialog-github-url";
import { scoreBranch } from "@/lib/utils/branch-filter";
import {
  useAccessibleRepos,
  type UseAccessibleReposResult,
} from "@/hooks/domains/github/use-accessible-repos";
import type { AccessibleRepo } from "@/lib/api/domains/github-api";
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
 * arm also carries the canonical `owner/name` and provider so the chip can
 * display a friendly label without re-parsing the URL; the "paste" arm
 * leaves metadata undefined so the row drops any stale picker data.
 */
export type RemoteRepoChipProps = {
  row: TaskRemoteRepoRow;
  branches: Branch[];
  branchesLoading: boolean;
  onURLChange: (
    url: string,
    source: "picker" | "paste",
    metadata?: { provider: "github" | "gitlab"; fullName: string },
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
  onURLChange,
  onBranchChange,
  onRemove,
}: RemoteRepoChipProps) {
  return (
    <span
      className="inline-flex items-center rounded-md border border-input bg-input/20 dark:bg-input/30 pr-0.5"
      data-testid="remote-repo-chip"
      data-remote-url={row.url}
    >
      <RemoteRepoPill row={row} onURLChange={onURLChange} />
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

// --- Repo pill ---------------------------------------------------------------

function RemoteRepoPill({
  row,
  onURLChange,
}: {
  row: TaskRemoteRepoRow;
  onURLChange: RemoteRepoChipProps["onURLChange"];
}) {
  const [open, setOpen] = useState(false);
  const accessible = useAccessibleRepos();
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
          accessible={accessible}
          onPick={(repo) => {
            onURLChange(`https://github.com/${repo.owner}/${repo.name}`, "picker", {
              provider: "github",
              fullName: repo.full_name,
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
  // Drive the hook's debounced search whenever the user types. The hook
  // itself owns the 250ms debounce so we just forward the latest value.
  useEffect(() => {
    accessible.search(search);
  }, [search, accessible]);
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
        <div className="px-2 py-3 text-xs text-muted-foreground">Loading…</div>
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
      <span className="truncate">{repo.full_name}</span>
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
  const branchOptions = useMemo(() => sortBranches(branches).map(branchToOption), [branches]);
  const placeholder = computeBranchPlaceholder(hasUrl, branchesLoading, branchOptions.length);
  return (
    <Pill
      icon={<IconGitBranch className="h-3 w-3 shrink-0 text-muted-foreground" />}
      value={branch}
      placeholder={placeholder}
      options={branchOptions}
      onSelect={onBranchChange}
      disabled={!hasUrl || branchesLoading || branchOptions.length === 0}
      disabledReason={computeUrlBranchDisabledReason({
        hasUrl,
        branchesLoading,
        optionCount: branchOptions.length,
      })}
      searchPlaceholder="Search branches..."
      emptyMessage="No branches"
      testId="remote-branch-chip-trigger"
      filter={scoreBranch}
      tooltip="Base branch"
      flat
    />
  );
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
