"use client";

import { useState } from "react";
import {
  IconChevronDown,
  IconChevronRight,
  IconCloudUpload,
  IconGitPullRequest,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { CommitRow, type CommitItem } from "./commit-row";
import { groupByRepositoryName } from "@/lib/group-by-repo";

type ChangedFile = {
  path: string;
  status: import("@/lib/state/store").FileInfo["status"];
  staged: boolean;
  plus: number | undefined;
  minus: number | undefined;
  oldPath: string | undefined;
  repositoryName?: string;
};

export type RepoGroup = ReturnType<typeof groupByRepositoryName<ChangedFile>>[number];

/**
 * Per-repo group inside a file-list section (Unstaged / Staged). Renders the
 * collapsible repo header with optional inline action buttons (Stage all /
 * Commit / Unstage all). Action labels and handlers are owned by the parent
 * section so the same shape works for both variants.
 */
export function RepoGroupItem({
  group,
  collapsed,
  onToggle,
  renderRow,
  primaryLabel,
  secondaryLabel,
  onRepoAction,
  onRepoSecondaryAction,
  displayName,
}: {
  group: RepoGroup;
  collapsed: boolean;
  onToggle: () => void;
  renderRow: (file: ChangedFile) => React.ReactNode;
  primaryLabel: string;
  secondaryLabel?: string;
  onRepoAction?: (repo: string) => void;
  onRepoSecondaryAction?: (repo: string) => void;
  /** Optional display label override; defaults to group.repositoryName. */
  displayName?: string;
}) {
  const stop = (e: React.MouseEvent) => e.stopPropagation();
  const label = displayName || group.repositoryName || "Repository";
  return (
    <li data-testid="changes-repo-group" data-repository-name={group.repositoryName || ""}>
      <div className="flex items-center justify-between gap-2 px-1 py-0.5">
        <button
          type="button"
          className="flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground/80 uppercase tracking-wide cursor-pointer hover:text-foreground/80 min-w-0"
          data-testid="changes-repo-header"
          aria-expanded={!collapsed}
          onClick={onToggle}
        >
          {collapsed ? (
            <IconChevronRight className="h-3 w-3 text-muted-foreground/50 shrink-0" />
          ) : (
            <IconChevronDown className="h-3 w-3 text-muted-foreground/50 shrink-0" />
          )}
          <span className="truncate">{label}</span>
          <span className="text-muted-foreground/50 normal-case tracking-normal">
            {group.items.length}
          </span>
        </button>
        {(onRepoAction || onRepoSecondaryAction) && (
          <div className="flex items-center gap-1" onClick={stop}>
            {onRepoAction && (
              <Button
                size="sm"
                variant="ghost"
                className="h-5 text-[10px] px-1.5 cursor-pointer"
                data-testid="repo-group-action"
                onClick={() => onRepoAction(group.repositoryName)}
              >
                {primaryLabel}
              </Button>
            )}
            {onRepoSecondaryAction && secondaryLabel && (
              <Button
                size="sm"
                variant="ghost"
                className="h-5 text-[10px] px-1.5 cursor-pointer text-muted-foreground"
                data-testid="repo-group-secondary-action"
                onClick={() => onRepoSecondaryAction(group.repositoryName)}
              >
                {secondaryLabel}
              </Button>
            )}
          </div>
        )}
      </div>
      {!collapsed && <ul className="space-y-0.5">{group.items.map(renderRow)}</ul>}
    </li>
  );
}

function CommitsGroupActions({
  repositoryName,
  unpushedCount,
  aheadCount,
  prExists,
  canCreatePR,
  onRepoPush,
  onRepoCreatePR,
  stop,
}: {
  repositoryName: string;
  unpushedCount: number;
  aheadCount: number;
  prExists: boolean;
  canCreatePR: boolean;
  onRepoPush?: (repo: string) => void;
  onRepoCreatePR?: (repo: string) => void;
  stop: (e: React.MouseEvent) => void;
}) {
  return (
    <div className="flex items-center gap-1" onClick={stop}>
      {onRepoPush && (unpushedCount > 0 || aheadCount > 0) && (
        <Button
          size="sm"
          variant="ghost"
          className="h-5 text-[10px] px-1.5 cursor-pointer gap-1"
          data-testid="commits-repo-push"
          onClick={() => onRepoPush(repositoryName)}
        >
          <IconCloudUpload className="h-3 w-3" />
          Push
          <span className="text-muted-foreground">{unpushedCount || aheadCount}</span>
        </Button>
      )}
      {onRepoCreatePR && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="sm"
              variant="ghost"
              className="h-5 text-[10px] px-1.5 cursor-pointer gap-1"
              data-testid="commits-repo-create-pr"
              onClick={() => onRepoCreatePR(repositoryName)}
              disabled={!canCreatePR}
            >
              <IconGitPullRequest className="h-3 w-3" />
              PR
            </Button>
          </TooltipTrigger>
          {prExists && <TooltipContent>A pull request already exists for this task</TooltipContent>}
        </Tooltip>
      )}
    </div>
  );
}

/**
 * Per-repo group inside the Commits section. Like {@link RepoGroupItem} but
 * for commit rows; surfaces a Push button when the repo has unpushed commits.
 */
export function CommitsRepoGroup({
  repositoryName,
  displayName,
  groupCommits,
  aheadCount = 0,
  existingPrUrl,
  onOpenCommitDetail,
  onAmendCommit,
  onRevertCommit,
  onResetToCommit,
  onRepoPush,
  onRepoCreatePR,
}: {
  repositoryName: string;
  displayName?: string;
  groupCommits: CommitItem[];
  aheadCount?: number;
  existingPrUrl?: string;
  onOpenCommitDetail?: (sha: string, repo?: string) => void;
  onAmendCommit?: (currentMessage: string, repo?: string) => void;
  onRevertCommit?: (sha: string, repo?: string) => void;
  onResetToCommit?: (sha: string, repo?: string) => void;
  onRepoPush?: (repo: string) => void;
  onRepoCreatePR?: (repo: string) => void;
  /** Base branch passed through; reserved for richer per-repo PR UX. */
  baseBranch?: string;
}) {
  const [collapsed, setCollapsed] = useState(false);
  // Each repo has its own "latest unpushed commit" — revert/amend in this
  // group must target THIS repo's newest, not the merged-list newest.
  const firstUnpushedInGroup = groupCommits.findIndex((c) => c.pushed !== true);
  const unpushedCount = groupCommits.filter((c) => !c.pushed).length;
  const stop = (e: React.MouseEvent) => e.stopPropagation();
  const label = displayName || repositoryName || "Repository";
  // Bug 10 acknowledged trade-off: `existingPrUrl` is sourced from a
  // workspace-scoped `prByRepo` map keyed only by "" today — the kandev task
  // model has one PR per task, not one PR per repo. As a result every per-
  // repo group inherits the same PR URL. When the backend grows per-repo PR
  // tracking, callers should key `prByRepo` by `repositoryName` and the
  // `?? prByRepo[""]` fallback in CommitsSection should be removed so each
  // group surfaces only its own PR. Until then, the visual is "PR exists for
  // this task" rather than "PR exists for this repo" — accept the imprecision.
  const prExists = !!existingPrUrl;
  // The PR scope today is workspace-wide (one PR per task). Disable per-repo
  // Create PR once a PR exists; the user can update it via push instead.
  const canCreatePR = !!onRepoCreatePR && !prExists;
  return (
    <li data-testid="commits-repo-group" data-repository-name={repositoryName || ""}>
      <div className="flex items-center justify-between gap-2 px-1 py-0.5">
        <button
          type="button"
          className="flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground/80 uppercase tracking-wide cursor-pointer hover:text-foreground/80 min-w-0"
          data-testid="commits-repo-header"
          aria-expanded={!collapsed}
          onClick={() => setCollapsed((c) => !c)}
        >
          {collapsed ? (
            <IconChevronRight className="h-3 w-3 text-muted-foreground/50 shrink-0" />
          ) : (
            <IconChevronDown className="h-3 w-3 text-muted-foreground/50 shrink-0" />
          )}
          <span className="truncate">{label}</span>
          <span className="text-muted-foreground/50 normal-case tracking-normal">
            {groupCommits.length}
          </span>
        </button>
        <CommitsGroupActions
          repositoryName={repositoryName}
          unpushedCount={unpushedCount}
          aheadCount={aheadCount}
          prExists={prExists}
          canCreatePR={canCreatePR}
          onRepoPush={onRepoPush}
          onRepoCreatePR={onRepoCreatePR}
          stop={stop}
        />
      </div>
      {!collapsed && (
        <ul className="space-y-0.5">
          {groupCommits.map((commit, index) => (
            <CommitRow
              key={commit.commit_sha}
              commit={commit}
              isLatest={index === firstUnpushedInGroup}
              onOpenCommitDetail={onOpenCommitDetail}
              onAmendCommit={commit.pushed ? undefined : onAmendCommit}
              onRevertCommit={commit.pushed ? undefined : onRevertCommit}
              onResetToCommit={commit.pushed ? undefined : onResetToCommit}
            />
          ))}
        </ul>
      )}
    </li>
  );
}
