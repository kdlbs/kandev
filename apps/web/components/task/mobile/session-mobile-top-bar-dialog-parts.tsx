"use client";

import { IconGitPullRequest, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

export function MobilePRBranchSummary({
  displayBranch,
  baseBranch,
}: {
  displayBranch: string | undefined;
  baseBranch: string | undefined;
}) {
  return (
    <div className="text-sm text-muted-foreground">
      {baseBranch ? (
        <span>
          Creating PR from <span className="font-medium text-foreground">{displayBranch}</span> to{" "}
          <span className="font-medium text-foreground">{baseBranch}</span>
        </span>
      ) : (
        <span>
          Creating PR from <span className="font-medium text-foreground">{displayBranch}</span>
        </span>
      )}
    </div>
  );
}

export function CommitSummary({
  uncommittedCount,
  uncommittedAdditions,
  uncommittedDeletions,
}: {
  uncommittedCount: number;
  uncommittedAdditions: number;
  uncommittedDeletions: number;
}) {
  if (uncommittedCount <= 0) return <span>No changes to commit</span>;
  return (
    <span>
      <span className="font-medium text-foreground">{uncommittedCount}</span> file
      {uncommittedCount !== 1 ? "s" : ""} changed
      {(uncommittedAdditions > 0 || uncommittedDeletions > 0) && (
        <span className="ml-2">
          (<span className="text-green-600">+{uncommittedAdditions}</span>
          {" / "}
          <span className="text-red-600">-{uncommittedDeletions}</span>)
        </span>
      )}
    </span>
  );
}

export function PRSubmitButton({
  prTitle,
  prBody,
  prDraft,
  isGitLoading,
  onCreatePR,
}: {
  prTitle: string;
  prBody: string;
  prDraft: boolean;
  isGitLoading: boolean;
  onCreatePR: (title: string, body: string, draft: boolean) => void;
}) {
  return (
    <Button
      onClick={() => onCreatePR(prTitle.trim(), prBody.trim(), prDraft)}
      disabled={!prTitle.trim() || isGitLoading}
      className="bg-cyan-600 hover:bg-cyan-700 text-white"
    >
      {isGitLoading ? (
        <>
          <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
          Creating...
        </>
      ) : (
        <>
          <IconGitPullRequest className="h-4 w-4 mr-2" />
          Create PR
        </>
      )}
    </Button>
  );
}
