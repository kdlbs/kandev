"use client";

import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { CommitStatBadge } from "@/components/diff-stat";

/** Ahead/Behind commit status badges for the task topbar. */
export function GitAheadBehindBadges({
  gitStatus,
  baseBranch,
  isMultiRepo,
}: {
  gitStatus: { ahead: number; behind: number };
  baseBranch?: string;
  isMultiRepo: boolean;
}) {
  // Multi-repo: a single ahead/behind badge is meaningless — each repo has
  // its own counts (and one repo could be ahead while another is behind).
  // The Pull dropdown surfaces per-repo behind counts; the per-repo Push
  // buttons surface ahead counts. So we suppress the global badge here to
  // avoid showing whichever value the legacy single-status slot last
  // received (which conflated repos).
  if (isMultiRepo) return null;
  const ahead = gitStatus?.ahead ?? 0;
  const behind = gitStatus?.behind ?? 0;
  if (ahead === 0 && behind === 0) return null;
  const compareRef = baseBranch || "main";
  return (
    <div className="flex items-center gap-1">
      {ahead > 0 && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="cursor-default">
              <CommitStatBadge label={`${ahead} ahead`} tone="ahead" />
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {ahead} commit{ahead !== 1 ? "s" : ""} ahead of {compareRef}
          </TooltipContent>
        </Tooltip>
      )}
      {behind > 0 && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="cursor-default">
              <CommitStatBadge label={`${behind} behind`} tone="behind" />
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {behind} commit{behind !== 1 ? "s" : ""} behind {compareRef}
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}
