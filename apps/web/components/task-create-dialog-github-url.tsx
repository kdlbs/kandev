"use client";

import { useMemo } from "react";
import { IconGitBranch } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Branch } from "@/lib/types/http";
import { scoreBranch } from "@/lib/utils/branch-filter";
import {
  Pill,
  sortBranches,
  branchToOption,
  computeBranchPlaceholder,
} from "@/components/task-create-dialog-pill";

function computeUrlBranchDisabledReason({
  hasUrl,
  branchesLoading,
  optionCount,
}: {
  hasUrl: boolean;
  branchesLoading: boolean;
  optionCount: number;
}): string | undefined {
  if (!hasUrl) return "Enter a GitHub URL first.";
  if (branchesLoading) return "Loading branches…";
  if (optionCount === 0) return "No branches available for this URL.";
  return undefined;
}

export function GitHubUrlSection({
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
  const branchOptions = useMemo(
    () => sortBranches(githubBranches).map(branchToOption),
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
        disabledReason={computeUrlBranchDisabledReason({
          hasUrl: !!githubUrl.trim(),
          branchesLoading: githubBranchesLoading,
          optionCount: branchOptions.length,
        })}
        searchPlaceholder="Search branches..."
        emptyMessage="No branches"
        testId="branch-chip-trigger"
        filter={scoreBranch}
        tooltip="Base branch"
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
