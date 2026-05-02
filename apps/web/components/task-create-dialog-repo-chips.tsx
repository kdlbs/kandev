"use client";

import { useEffect, useMemo, useState } from "react";
import { IconPlus, IconX, IconCode, IconGitBranch, IconGitFork } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { useBranches, type BranchSource } from "@/hooks/domains/workspace/use-repository-branches";
import type { Branch, LocalRepository, Repository } from "@/lib/types/http";
import type { DialogFormState, TaskRepoRow } from "@/components/task-create-dialog-types";
import { autoSelectBranch } from "@/components/task-create-dialog-helpers";
import { BranchRefreshButton } from "@/components/branch-refresh-button";

/**
 * Chip row for the task-create dialog. Renders one chip per row in
 * `fs.repositories`, plus a trailing "+" to add another. Each chip is two
 * pills (repo, branch) and a remove (×). All chips are equivalent — there
 * is no "primary" — and any row can hold either a workspace repo or a
 * discovered on-machine path.
 *
 * In GitHub URL mode the chips are replaced by an inline URL input pill;
 * the trailing toggle flips between the two modes.
 */
type RepoChipsRowProps = {
  fs: DialogFormState;
  repositories: Repository[];
  isTaskStarted: boolean;
  /** Required for loading branches on discovered (path-keyed) rows. */
  workspaceId: string | null;
  /**
   * Per-row repo change handler. Resolves the picked value into either a
   * workspace `repositoryId` or a discovered `localPath` and writes that
   * into the row. Comes from useDialogHandlers so the resolution logic
   * stays in one place.
   */
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  /** GitHub URL flow lives alongside the chips so users can switch in place. */
  onToggleGitHubUrl?: () => void;
  onGitHubUrlChange?: (value: string) => void;
  /**
   * Fresh-branch toggle props. When `freshBranchAvailable` is true the toggle
   * renders inline at the right edge of the chip row so it sits next to the
   * branch pills it affects, instead of taking its own row under the
   * agent/executor selectors.
   */
  freshBranchAvailable?: boolean;
  freshBranchEnabled?: boolean;
  onToggleFreshBranch?: (enabled: boolean) => void;
  /**
   * Lock branch pills when the task runs on the local executor — the user's
   * actual checkout dictates the branch, and changing it would mutate their
   * working tree. Fresh-branch mode unlocks it (we're explicitly forking a
   * new branch from a chosen base).
   */
  isLocalExecutor?: boolean;
};

export function RepoChipsRow({
  fs,
  repositories,
  isTaskStarted,
  workspaceId,
  onRowRepositoryChange,
  onRowBranchChange,
  onToggleGitHubUrl,
  onGitHubUrlChange,
  freshBranchAvailable,
  freshBranchEnabled,
  onToggleFreshBranch,
  isLocalExecutor,
}: RepoChipsRowProps) {
  const branchLocked = !!isLocalExecutor && !freshBranchEnabled;
  // No early returns above hooks. URL mode and started-state checks happen below.
  const usedIds = useMemo(() => collectUsedRepoIds(fs.repositories), [fs.repositories]);
  if (isTaskStarted) return null;

  const remainingCount = repositories.filter((r) => !usedIds.has(r.id)).length;
  // Add stays enabled when no workspace repos are left, since users can also
  // pick from discovered on-machine paths.
  const hasDiscovered = fs.discoveredRepositories.length > 0;
  const canAddMore = remainingCount > 0 || hasDiscovered;
  const addHint = canAddMore ? undefined : "All workspace repositories are already added";

  return (
    <div className="flex flex-wrap items-center gap-2" data-testid="repo-chips-row">
      {fs.useGitHubUrl ? (
        <GitHubUrlSection
          githubUrl={fs.githubUrl}
          githubUrlError={fs.githubUrlError}
          githubBranch={fs.githubBranch}
          githubBranches={fs.githubBranches}
          githubBranchesLoading={fs.githubBranchesLoading}
          onGitHubUrlChange={onGitHubUrlChange}
          onGitHubBranchChange={fs.setGitHubBranch}
        />
      ) : (
        <ChipsList
          fs={fs}
          repositories={repositories}
          workspaceId={workspaceId}
          branchLocked={branchLocked}
          canAddMore={canAddMore}
          addHint={addHint}
          onRowRepositoryChange={onRowRepositoryChange}
          onRowBranchChange={onRowBranchChange}
        />
      )}
      {freshBranchAvailable && onToggleFreshBranch && (
        <button
          type="button"
          onClick={() => onToggleFreshBranch(!freshBranchEnabled)}
          data-testid="fresh-branch-toggle"
          aria-pressed={!!freshBranchEnabled}
          aria-label={freshBranchEnabled ? "Fork a new branch" : "Use current branch"}
          className={cn(
            "inline-flex h-7 w-7 items-center justify-center rounded-md border border-input cursor-pointer transition-colors",
            freshBranchEnabled
              ? "bg-muted text-foreground"
              : "bg-transparent text-muted-foreground hover:text-foreground hover:bg-muted/60",
          )}
          title={
            freshBranchEnabled
              ? "Will fork a new branch from the selected base"
              : "Use the current branch (no fork)"
          }
        >
          <IconGitFork className="h-3.5 w-3.5" />
        </button>
      )}
      {onToggleGitHubUrl && (
        <button
          type="button"
          onClick={onToggleGitHubUrl}
          className="ml-auto text-[11px] text-muted-foreground hover:text-foreground cursor-pointer"
          data-testid="toggle-github-url"
        >
          {fs.useGitHubUrl ? "use a workspace repository" : "or paste a GitHub URL"}
        </button>
      )}
    </div>
  );
}

/**
 * Renders the list of repo chips plus the trailing "+ add repository"
 * button. Extracted from RepoChipsRow so the parent stays under the
 * function-length cap; logic is unchanged.
 */
function ChipsList({
  fs,
  repositories,
  workspaceId,
  branchLocked,
  canAddMore,
  addHint,
  onRowRepositoryChange,
  onRowBranchChange,
}: {
  fs: DialogFormState;
  repositories: Repository[];
  workspaceId: string | null;
  branchLocked: boolean;
  canAddMore: boolean;
  addHint?: string;
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
}) {
  return (
    <>
      {fs.repositories.map((row) => (
        <RepoChip
          key={row.key}
          row={row}
          workspaceId={workspaceId}
          repositories={repositories}
          discoveredRepositories={fs.discoveredRepositories}
          excludedRepoIds={collectUsedRepoIds(fs.repositories, row.key)}
          branchLocked={branchLocked}
          branchOverride={branchLocked ? fs.currentLocalBranch : undefined}
          onRepositoryChange={(value) => onRowRepositoryChange(row.key, value)}
          onBranchChange={(value) => onRowBranchChange(row.key, value)}
          onRemove={() => fs.removeRepository(row.key)}
        />
      ))}
      <button
        type="button"
        onClick={fs.addRepository}
        disabled={!canAddMore}
        title={addHint}
        aria-label="Add repository"
        data-testid="add-repository"
        className={cn(
          "h-7 w-7 inline-flex items-center justify-center rounded-md text-muted-foreground",
          canAddMore
            ? "hover:bg-muted hover:text-foreground cursor-pointer"
            : "opacity-40 cursor-not-allowed",
        )}
      >
        <IconPlus className="h-3.5 w-3.5" />
      </button>
    </>
  );
}

/** Build the set of repo identifiers (workspace id or path) currently in use. */
function collectUsedRepoIds(rows: TaskRepoRow[], exceptKey?: string): Set<string> {
  const ids = new Set<string>();
  for (const r of rows) {
    if (r.key === exceptKey) continue;
    if (r.repositoryId) ids.add(r.repositoryId);
    if (r.localPath) ids.add(r.localPath);
  }
  return ids;
}

type RepoChipProps = {
  row: TaskRepoRow;
  /** Required for path-based branch loading on discovered rows. */
  workspaceId: string | null;
  repositories: Repository[];
  discoveredRepositories: LocalRepository[];
  /** Repo IDs/paths to filter out of the dropdown (already in use elsewhere). */
  excludedRepoIds: Set<string>;
  /**
   * Lock the branch pill regardless of branch availability. Used for the
   * local executor where the user's actual checkout dictates the branch
   * (and changing it would mutate their working tree). Fresh-branch mode
   * unlocks it because we're explicitly creating a new branch from a base.
   */
  branchLocked?: boolean;
  /**
   * Optional display override for the branch value. Used when the chip
   * is `branchLocked` so we surface the repo's actual current branch
   * instead of whatever the user picked under a different executor.
   */
  branchOverride?: string;
  onRepositoryChange: (value: string) => void;
  onBranchChange: (value: string) => void;
  onRemove: () => void;
};

function useRepoChipData({
  row,
  workspaceId,
  repositories,
  discoveredRepositories,
  excludedRepoIds,
  onBranchChange,
}: Pick<
  RepoChipProps,
  | "row"
  | "workspaceId"
  | "repositories"
  | "discoveredRepositories"
  | "excludedRepoIds"
  | "onBranchChange"
>) {
  const filteredRepos = useMemo(
    () => repositories.filter((r) => !excludedRepoIds.has(r.id) || r.id === row.repositoryId),
    [repositories, excludedRepoIds, row.repositoryId],
  );
  // Discovered (on-machine) repos not yet imported into the workspace. Drop
  // ones whose path is already a workspace repo so the dropdown doesn't show
  // the same folder twice.
  const filteredDiscovered = useMemo(() => {
    const workspaceRepoPaths = new Set(
      filteredRepos
        .map((r) => r.local_path)
        .filter(Boolean)
        .map((path: string) => normalizeRepoPath(path)),
    );
    return discoveredRepositories.filter(
      (r) =>
        !workspaceRepoPaths.has(normalizeRepoPath(r.path)) &&
        (!excludedRepoIds.has(r.path) || r.path === row.localPath),
    );
  }, [filteredRepos, discoveredRepositories, excludedRepoIds, row.localPath]);

  // One hook for both row sources (workspace repo by id OR on-machine path).
  // Backend resolves either to an absolute path; the Zustand cache keys
  // path-based entries with a synthetic key so workspace + discovered share
  // one slice (and the same fetch dedupe).
  const branchSource = useMemo<BranchSource | null>(() => {
    if (!workspaceId) return null;
    if (row.repositoryId) {
      return { kind: "id", workspaceId, repositoryId: row.repositoryId };
    }
    if (row.localPath) {
      return { kind: "path", workspaceId, path: row.localPath };
    }
    return null;
  }, [workspaceId, row.repositoryId, row.localPath]);
  const {
    branches,
    isLoading: branchesLoading,
    refresh: refreshBranches,
  } = useBranches(branchSource, !!branchSource);

  // Once branches load for this row, pre-fill the branch pill with the user's
  // last-used branch (when present) or main/master/origin/main/origin/master.
  // Skipped if the user already picked a branch on this row.
  useEffect(() => {
    if (!branchSource || branchesLoading || branches.length === 0 || row.branch) return;
    autoSelectBranch(branches, onBranchChange);
  }, [branchSource, branchesLoading, branches, row.branch, onBranchChange]);

  const repoOptions: PillOption[] = useMemo(
    () => [
      ...filteredRepos.map((r) => ({ value: r.id, label: r.name })),
      ...filteredDiscovered.map((r) => ({
        value: r.path,
        label: shortRepoPath(r.path),
        keywords: [r.path],
      })),
    ],
    [filteredRepos, filteredDiscovered],
  );
  const branchOptions: PillOption[] = useMemo(() => branches.map(branchToOption), [branches]);
  return { repoOptions, branchOptions, branchesLoading, refreshBranches };
}

function RepoChip({
  row,
  workspaceId,
  repositories,
  discoveredRepositories,
  excludedRepoIds,
  branchLocked,
  branchOverride,
  onRepositoryChange,
  onBranchChange,
  onRemove,
}: RepoChipProps) {
  const { repoOptions, branchOptions, branchesLoading, refreshBranches } = useRepoChipData({
    row,
    workspaceId,
    repositories,
    discoveredRepositories,
    excludedRepoIds,
    onBranchChange,
  });
  const repoLabel =
    repositories.find((r) => r.id === row.repositoryId)?.name ??
    discoveredRepositories
      .find((r) => r.path === row.localPath)
      ?.path?.split("/")
      .pop() ??
    "";
  const hasRepo = !!(row.repositoryId || row.localPath);
  const branchPlaceholder = computeBranchPlaceholder(
    hasRepo,
    branchesLoading,
    branchOptions.length,
  );

  return (
    <span
      className="inline-flex items-center gap-1"
      data-testid="repo-chip"
      data-repository-id={row.repositoryId || row.localPath || ""}
    >
      <Pill
        icon={<IconCode className="h-3 w-3 shrink-0 text-muted-foreground" />}
        value={repoLabel}
        placeholder="repository"
        options={repoOptions}
        onSelect={onRepositoryChange}
        searchPlaceholder="Search repositories..."
        emptyMessage="No repositories"
        testId="repo-chip-trigger"
      />
      <Pill
        icon={<IconGitBranch className="h-3 w-3 shrink-0 text-muted-foreground" />}
        value={branchOverride ?? row.branch}
        placeholder={branchPlaceholder}
        options={branchOptions}
        onSelect={onBranchChange}
        disabled={branchLocked || !hasRepo || branchesLoading || branchOptions.length === 0}
        searchPlaceholder="Search branches..."
        emptyMessage="No branches"
        testId="branch-chip-trigger"
        onRefresh={refreshBranches}
        refreshing={branchesLoading}
      />
      <button
        type="button"
        onClick={onRemove}
        aria-label="Remove repository"
        className="h-7 w-6 inline-flex items-center justify-center rounded-md text-muted-foreground hover:text-destructive hover:bg-muted cursor-pointer"
        data-testid="remove-repo-chip"
      >
        <IconX className="h-3 w-3" />
      </button>
    </span>
  );
}

type PillOption = { value: string; label: string; keywords?: string[] };

function computeBranchPlaceholder(hasRepo: boolean, loading: boolean, optionCount: number): string {
  if (!hasRepo) return "branch";
  if (loading) return "loading…";
  if (optionCount === 0) return "no branches";
  return "branch";
}

function normalizeRepoPath(path: string): string {
  return path.replace(/\\/g, "/").replace(/\/+$/g, "");
}

function shortRepoPath(path: string): string {
  // Show the trailing folder name with one parent for context (e.g. "myorg/myrepo").
  const parts = path.replace(/\\/g, "/").split("/").filter(Boolean);
  if (parts.length <= 1) return path;
  return parts.slice(-2).join("/");
}

function branchToOption(b: Branch): PillOption {
  // Remote branches keep their "origin/" prefix so they're distinguishable
  // from local branches with the same short name (e.g. "main" vs "origin/main").
  // Without the prefix, the dropdown shows two indistinguishable rows.
  const display = b.type === "remote" && b.remote ? `${b.remote}/${b.name}` : b.name;
  return { value: display, label: display };
}

/**
 * Compact pill trigger that opens a popover with a search list. Auto-widths
 * to its content (no `w-full`, no chevron) so multiple pills can sit on one
 * line without overlapping or stretching to fill the row.
 */
function Pill({
  icon,
  value,
  placeholder,
  options,
  onSelect,
  disabled = false,
  searchPlaceholder,
  emptyMessage,
  testId,
  onRefresh,
  refreshing,
}: {
  icon: React.ReactNode;
  value: string;
  placeholder: string;
  options: PillOption[];
  onSelect: (value: string) => void;
  disabled?: boolean;
  searchPlaceholder: string;
  emptyMessage: string;
  testId?: string;
  /** Optional refresh action rendered next to the search input. */
  onRefresh?: () => void;
  /** Show the refresh icon as spinning + disabled while a refresh is in flight. */
  refreshing?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const hasValue = !!value;
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          disabled={disabled}
          data-testid={testId}
          className={cn(
            "h-7 inline-flex items-center gap-1.5 rounded-md px-2.5 text-xs",
            "border border-border/60 bg-muted/30",
            disabled
              ? "opacity-50 cursor-not-allowed"
              : "hover:bg-muted hover:border-border cursor-pointer",
            !hasValue && "text-muted-foreground",
          )}
        >
          {icon}
          <span className="truncate max-w-[160px]">{value || placeholder}</span>
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-0" align="start" portal={false}>
        <Command>
          <div className="flex items-center gap-1 px-2 pt-1">
            <CommandInput placeholder={searchPlaceholder} className="h-9 flex-1" />
            {onRefresh && <BranchRefreshButton onRefresh={onRefresh} refreshing={refreshing} />}
          </div>
          <CommandList>
            <CommandEmpty>{emptyMessage}</CommandEmpty>
            <CommandGroup>
              {options.map((option) => (
                <CommandItem
                  key={option.value}
                  value={option.value}
                  keywords={[option.label, ...(option.keywords ?? [])]}
                  onSelect={() => {
                    onSelect(option.value);
                    setOpen(false);
                  }}
                >
                  {option.label}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

function GitHubUrlSection({
  githubUrl,
  githubUrlError,
  githubBranch,
  githubBranches,
  githubBranchesLoading,
  onGitHubUrlChange,
  onGitHubBranchChange,
}: {
  githubUrl: string;
  githubUrlError: string | null;
  githubBranch: string;
  githubBranches: Branch[];
  githubBranchesLoading: boolean;
  onGitHubUrlChange?: (value: string) => void;
  onGitHubBranchChange: (value: string) => void;
}) {
  // URL mode is always remote-clone-based, so the branch is freely
  // selectable regardless of executor type. Local executor's "branch is
  // dictated by the user's checkout" rule does not apply here — there is
  // no pre-existing local checkout for an arbitrary GitHub URL.
  const branchOptions: PillOption[] = useMemo(
    () => githubBranches.map(branchToOption),
    [githubBranches],
  );
  const branchPlaceholder = computeBranchPlaceholder(
    !!githubUrl.trim(),
    githubBranchesLoading,
    branchOptions.length,
  );
  return (
    <>
      <GitHubUrlPill
        githubUrl={githubUrl}
        githubUrlError={githubUrlError}
        onGitHubUrlChange={onGitHubUrlChange}
      />
      <Pill
        icon={<IconGitBranch className="h-3 w-3 shrink-0 text-muted-foreground" />}
        value={githubBranch}
        placeholder={branchPlaceholder}
        options={branchOptions}
        onSelect={onGitHubBranchChange}
        disabled={!githubUrl.trim() || githubBranchesLoading || branchOptions.length === 0}
        searchPlaceholder="Search branches..."
        emptyMessage="No branches"
        testId="branch-chip-trigger"
      />
    </>
  );
}

function GitHubUrlPill({
  githubUrl,
  githubUrlError,
  onGitHubUrlChange,
}: {
  githubUrl: string;
  githubUrlError: string | null;
  onGitHubUrlChange?: (value: string) => void;
}) {
  return (
    <div className="relative inline-flex items-center">
      <input
        type="text"
        value={githubUrl}
        onChange={(e) => onGitHubUrlChange?.(e.target.value)}
        placeholder="github.com/owner/repo"
        data-testid="github-url-input"
        aria-label="GitHub repository URL"
        aria-invalid={!!githubUrlError}
        aria-describedby={githubUrlError ? "github-url-error" : undefined}
        className={cn(
          "h-7 rounded-md px-2.5 text-xs bg-muted/30 border border-border/60",
          "outline-none focus:bg-muted focus:border-border placeholder:text-muted-foreground",
          githubUrlError && "border-destructive text-destructive",
        )}
        autoFocus
      />
      {githubUrlError && (
        <div
          id="github-url-error"
          role="alert"
          className="absolute left-0 top-full mt-1 z-50 rounded-md border bg-popover px-2 py-1 text-[11px] text-destructive shadow-md whitespace-nowrap"
          data-testid="github-url-error"
        >
          {githubUrlError}
        </div>
      )}
    </div>
  );
}
