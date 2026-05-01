"use client";

import { useState, useCallback } from "react";
import type { useToast } from "@/components/toast-provider";
import type { SessionGit, PerRepoOperationResult } from "@/hooks/domains/session/use-session-git";

// Bug 7: drop the local GitOps shape — `SessionGit` is the single source of
// truth for all the methods this module needs (pull, push, rebase, merge,
// commit, stage, unstage, discard, revertCommit, reset, createPR, isLoading).
// Callers pass the SessionGit returned by `useSessionGit` directly.
type GitOps = Pick<
  SessionGit,
  | "pull"
  | "push"
  | "rebase"
  | "merge"
  | "commit"
  | "stage"
  | "unstage"
  | "discard"
  | "revertCommit"
  | "reset"
  | "createPR"
  | "isLoading"
>;
type Toast = ReturnType<typeof useToast>["toast"];
type GitOperationResultLike = {
  success: boolean;
  output: string;
  error?: string;
  per_repo?: PerRepoOperationResult[];
};
type GitOperationFn = (op: () => Promise<GitOperationResultLike>, name: string) => Promise<void>;

/**
 * Builds the toast description for a fan-out result. When `per_repo` is
 * present, summarise per-repo successes/failures instead of returning the
 * raw output (which was just the last repo's text and hid partial-success).
 */
function describePerRepo(
  perRepo: PerRepoOperationResult[],
  operationName: string,
): { title: string; description: string; variant: "success" | "error" } {
  const succeeded = perRepo.filter((r) => r.success);
  const failed = perRepo.filter((r) => !r.success);
  const succeededNames = succeeded.map((r) => r.repository_name).join(", ");
  const failedSummary = failed
    .map((r) => `${r.repository_name}: ${r.error || "unknown error"}`)
    .join("; ");
  if (failed.length === 0) {
    return {
      title: `${operationName} successful`,
      description: `${operationName} succeeded in ${succeeded.length} repos: ${succeededNames}`,
      variant: "success",
    };
  }
  if (succeeded.length === 0) {
    return {
      title: `${operationName} failed`,
      description: `Failed in ${failed.length} repos — ${failedSummary}`,
      variant: "error",
    };
  }
  // Partial success: surface as error so the user notices, but include the
  // succeeded list in the description so they don't retry the whole op.
  return {
    title: `${operationName} partially succeeded`,
    description: `${operationName} succeeded in ${succeeded.length} of ${perRepo.length} repos (${succeededNames}); failed in ${failedSummary}`,
    variant: "error",
  };
}

function labelWithRepo(label: string, repo: string | undefined): string {
  return repo ? `${label} (${repo})` : label;
}

export function useChangesGitHandlers(
  gitOps: GitOps,
  toast: Toast,
  baseBranch: string | undefined,
) {
  const handleGitOperation = useCallback(
    async (operation: () => Promise<GitOperationResultLike>, operationName: string) => {
      try {
        const result = await operation();
        // Bug 2: when the underlying op fanned out across multiple repos,
        // describe the per-repo breakdown instead of the legacy flat
        // success/error so partial successes are visible.
        if (result.per_repo && result.per_repo.length > 1) {
          const { title, description, variant } = describePerRepo(result.per_repo, operationName);
          toast({ title, description, variant });
          return;
        }
        const variant = result.success ? "success" : "error";
        const title = result.success ? `${operationName} successful` : `${operationName} failed`;
        const description = result.success
          ? result.output.slice(0, 200) || `${operationName} completed`
          : result.error || "An error occurred";
        toast({ title, description, variant });
      } catch (e) {
        toast({
          title: `${operationName} failed`,
          description: e instanceof Error ? e.message : "An unexpected error occurred",
          variant: "error",
        });
      }
    },
    [toast],
  );

  const handlePull = useCallback(
    (repo?: string) => {
      handleGitOperation(() => gitOps.pull(false, repo), labelWithRepo("Pull", repo));
    },
    [handleGitOperation, gitOps],
  );
  const handleRebase = useCallback(
    (repo?: string) => {
      const targetBranch = baseBranch?.replace(/^origin\//, "") || "main";
      handleGitOperation(() => gitOps.rebase(targetBranch, repo), labelWithRepo("Rebase", repo));
    },
    [handleGitOperation, gitOps, baseBranch],
  );
  const handleMerge = useCallback(
    (repo?: string) => {
      const targetBranch = baseBranch?.replace(/^origin\//, "") || "main";
      handleGitOperation(() => gitOps.merge(targetBranch, repo), labelWithRepo("Merge", repo));
    },
    [handleGitOperation, gitOps, baseBranch],
  );
  const handlePush = useCallback(
    (repo?: string) => {
      handleGitOperation(() => gitOps.push(undefined, repo), labelWithRepo("Push", repo));
    },
    [handleGitOperation, gitOps],
  );
  const handleForcePush = useCallback(
    (repo?: string) => {
      handleGitOperation(
        () => gitOps.push({ force: true }, repo),
        labelWithRepo("Force push", repo),
      );
    },
    [handleGitOperation, gitOps],
  );
  const handleRevertCommit = useCallback(
    (sha: string, repo?: string) => {
      handleGitOperation(() => gitOps.revertCommit(sha, repo), "Revert commit");
    },
    [handleGitOperation, gitOps],
  );

  return {
    handleGitOperation,
    handlePull,
    handleRebase,
    handleMerge,
    handlePush,
    handleForcePush,
    handleRevertCommit,
  };
}

function useChangesDiscardAmendHandlers(
  gitOps: GitOps,
  toast: Toast,
  handleGitOperation: GitOperationFn,
) {
  const [showDiscardDialog, setShowDiscardDialog] = useState(false);
  const [fileToDiscard, setFileToDiscard] = useState<string | null>(null);
  const [filesToDiscard, setFilesToDiscard] = useState<string[] | null>(null);
  // Multi-repo: remember the clicked file's repo so the discard op routes to
  // the right git repo. Path alone is ambiguous when two repos share a name.
  const [repoToDiscard, setRepoToDiscard] = useState<string | undefined>(undefined);

  const handleDiscardClick = useCallback((filePath: string, repo?: string) => {
    setFileToDiscard(filePath);
    setRepoToDiscard(repo);
    setFilesToDiscard(null);
    setShowDiscardDialog(true);
  }, []);
  const handleBulkDiscardClick = useCallback((paths: string[]) => {
    setFilesToDiscard(paths);
    setFileToDiscard(null);
    setRepoToDiscard(undefined);
    setShowDiscardDialog(true);
  }, []);
  const handleDiscardConfirm = useCallback(async () => {
    const paths = filesToDiscard ?? (fileToDiscard ? [fileToDiscard] : null);
    if (!paths) return;
    try {
      const result = await gitOps.discard(paths, repoToDiscard);
      if (!result.success)
        toast({
          title: "Failed to discard changes",
          description: result.error || "An unknown error occurred",
          variant: "error",
        });
    } catch (error) {
      toast({
        title: "Failed to discard changes",
        description: error instanceof Error ? error.message : "An unknown error occurred",
        variant: "error",
      });
    } finally {
      setShowDiscardDialog(false);
      setFileToDiscard(null);
      setFilesToDiscard(null);
      setRepoToDiscard(undefined);
    }
  }, [fileToDiscard, filesToDiscard, repoToDiscard, gitOps, toast]);

  // Amend dialog state (for editing last commit message directly)
  const [amendDialogOpen, setAmendDialogOpen] = useState(false);
  const [amendMessage, setAmendMessage] = useState("");
  // Multi-repo: capture the commit's repo at click time so the amend lands in
  // the right git repo. Path/SHA alone can't be disambiguated when each repo
  // has its own HEAD.
  const [amendRepo, setAmendRepo] = useState<string | undefined>(undefined);

  const handleOpenAmendDialog = useCallback((currentMessage: string, repo?: string) => {
    setAmendMessage(currentMessage);
    setAmendRepo(repo);
    setAmendDialogOpen(true);
  }, []);

  const handleAmend = useCallback(async () => {
    if (!amendMessage.trim()) return;
    setAmendDialogOpen(false);
    await handleGitOperation(
      () => gitOps.commit(amendMessage.trim(), false, true, amendRepo),
      "Amend commit",
    );
    setAmendMessage("");
    setAmendRepo(undefined);
  }, [amendMessage, amendRepo, handleGitOperation, gitOps]);

  return {
    showDiscardDialog,
    setShowDiscardDialog,
    fileToDiscard,
    filesToDiscard,
    handleDiscardClick,
    handleBulkDiscardClick,
    handleDiscardConfirm,
    // Amend dialog
    amendDialogOpen,
    setAmendDialogOpen,
    amendMessage,
    setAmendMessage,
    handleOpenAmendDialog,
    handleAmend,
  };
}

function useChangesResetHandlers(gitOps: GitOps, handleGitOperation: GitOperationFn) {
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const [resetCommitSha, setResetCommitSha] = useState<string | null>(null);
  // Multi-repo: capture the commit's repo so reset runs against the right
  // git repo. Without it, reset hits the workspace root and fails.
  const [resetRepo, setResetRepo] = useState<string | undefined>(undefined);

  const handleOpenResetDialog = useCallback((sha: string, repo?: string) => {
    setResetCommitSha(sha);
    setResetRepo(repo);
    setResetDialogOpen(true);
  }, []);

  const handleReset = useCallback(
    async (mode: "soft" | "hard") => {
      if (!resetCommitSha) return;
      setResetDialogOpen(false);
      const operationName = mode === "hard" ? "Hard reset" : "Soft reset";
      await handleGitOperation(() => gitOps.reset(resetCommitSha, mode, resetRepo), operationName);
      setResetCommitSha(null);
      setResetRepo(undefined);
    },
    [resetCommitSha, resetRepo, handleGitOperation, gitOps],
  );

  return {
    resetDialogOpen,
    setResetDialogOpen,
    resetCommitSha,
    handleOpenResetDialog,
    handleReset,
  };
}

export function useChangesDialogHandlers(
  gitOps: GitOps,
  toast: Toast,
  handleGitOperation: GitOperationFn,
) {
  const discardAmend = useChangesDiscardAmendHandlers(gitOps, toast, handleGitOperation);
  const reset = useChangesResetHandlers(gitOps, handleGitOperation);
  return { ...discardAmend, ...reset };
}
