"use client";

import { useState, useCallback } from "react";
import type { GitOperationResult, PRCreateResult } from "@/hooks/use-git-operations";
import type { useToast } from "@/components/toast-provider";

// Accepts both useGitOperations and SessionGit
interface GitOps {
  pull: (rebase?: boolean) => Promise<GitOperationResult>;
  push: (options?: { force?: boolean; setUpstream?: boolean }) => Promise<GitOperationResult>;
  rebase: (baseBranch: string) => Promise<GitOperationResult>;
  commit: (
    message: string,
    stageAll?: boolean,
    amend?: boolean,
    repo?: string,
  ) => Promise<GitOperationResult>;
  stage: (paths?: string[], repo?: string) => Promise<GitOperationResult>;
  unstage: (paths?: string[], repo?: string) => Promise<GitOperationResult>;
  discard: (paths?: string[], repo?: string) => Promise<GitOperationResult>;
  revertCommit: (commitSHA: string, repo?: string) => Promise<GitOperationResult>;
  reset: (commitSHA: string, mode: "soft" | "hard", repo?: string) => Promise<GitOperationResult>;
  createPR: (
    title: string,
    body: string,
    baseBranch?: string,
    draft?: boolean,
  ) => Promise<PRCreateResult>;
  isLoading: boolean;
}
type Toast = ReturnType<typeof useToast>["toast"];
type GitOperationFn = (
  op: () => Promise<{ success: boolean; output: string; error?: string }>,
  name: string,
) => Promise<void>;

export function useChangesGitHandlers(
  gitOps: GitOps,
  toast: Toast,
  baseBranch: string | undefined,
) {
  const handleGitOperation = useCallback(
    async (
      operation: () => Promise<{ success: boolean; output: string; error?: string }>,
      operationName: string,
    ) => {
      try {
        const result = await operation();
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

  const handlePull = useCallback(() => {
    handleGitOperation(() => gitOps.pull(), "Pull");
  }, [handleGitOperation, gitOps]);
  const handleRebase = useCallback(() => {
    const targetBranch = baseBranch?.replace(/^origin\//, "") || "main";
    handleGitOperation(() => gitOps.rebase(targetBranch), "Rebase");
  }, [handleGitOperation, gitOps, baseBranch]);
  const handlePush = useCallback(() => {
    handleGitOperation(() => gitOps.push(), "Push");
  }, [handleGitOperation, gitOps]);
  const handleForcePush = useCallback(() => {
    handleGitOperation(() => gitOps.push({ force: true }), "Force push");
  }, [handleGitOperation, gitOps]);
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
