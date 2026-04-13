"use client";

import { useState, useCallback } from "react";
import type { GitOperationResult, PRCreateResult } from "@/hooks/use-git-operations";
import type { useToast } from "@/components/toast-provider";

// Accepts both useGitOperations and SessionGit
interface GitOps {
  pull: (rebase?: boolean) => Promise<GitOperationResult>;
  push: (options?: { force?: boolean; setUpstream?: boolean }) => Promise<GitOperationResult>;
  rebase: (baseBranch: string) => Promise<GitOperationResult>;
  commit: (message: string, stageAll?: boolean, amend?: boolean) => Promise<GitOperationResult>;
  stage: (paths?: string[]) => Promise<GitOperationResult>;
  unstage: (paths?: string[]) => Promise<GitOperationResult>;
  discard: (paths?: string[]) => Promise<GitOperationResult>;
  revertCommit: (commitSHA: string) => Promise<GitOperationResult>;
  reset: (commitSHA: string, mode: "soft" | "hard") => Promise<GitOperationResult>;
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
    (sha: string) => {
      handleGitOperation(() => gitOps.revertCommit(sha), "Revert commit");
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

  const handleDiscardClick = useCallback((filePath: string) => {
    setFileToDiscard(filePath);
    setFilesToDiscard(null);
    setShowDiscardDialog(true);
  }, []);
  const handleBulkDiscardClick = useCallback((paths: string[]) => {
    setFilesToDiscard(paths);
    setFileToDiscard(null);
    setShowDiscardDialog(true);
  }, []);
  const handleDiscardConfirm = useCallback(async () => {
    const paths = filesToDiscard ?? (fileToDiscard ? [fileToDiscard] : null);
    if (!paths) return;
    try {
      const result = await gitOps.discard(paths);
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
    }
  }, [fileToDiscard, filesToDiscard, gitOps, toast]);

  // Amend dialog state (for editing last commit message directly)
  const [amendDialogOpen, setAmendDialogOpen] = useState(false);
  const [amendMessage, setAmendMessage] = useState("");

  const handleOpenAmendDialog = useCallback((currentMessage: string) => {
    setAmendMessage(currentMessage);
    setAmendDialogOpen(true);
  }, []);

  const handleAmend = useCallback(async () => {
    if (!amendMessage.trim()) return;
    setAmendDialogOpen(false);
    await handleGitOperation(() => gitOps.commit(amendMessage.trim(), false, true), "Amend commit");
    setAmendMessage("");
  }, [amendMessage, handleGitOperation, gitOps]);

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

  const handleOpenResetDialog = useCallback((sha: string) => {
    setResetCommitSha(sha);
    setResetDialogOpen(true);
  }, []);

  const handleReset = useCallback(
    async (mode: "soft" | "hard") => {
      if (!resetCommitSha) return;
      setResetDialogOpen(false);
      const operationName = mode === "hard" ? "Hard reset" : "Soft reset";
      await handleGitOperation(() => gitOps.reset(resetCommitSha, mode), operationName);
      setResetCommitSha(null);
    },
    [resetCommitSha, handleGitOperation, gitOps],
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
