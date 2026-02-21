"use client";

import { useState, useCallback, useEffect } from "react";
import type { useGitOperations } from "@/hooks/use-git-operations";
import type { useToast } from "@/components/toast-provider";

type GitOps = ReturnType<typeof useGitOperations>;
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

  return { handleGitOperation, handlePull, handleRebase, handlePush };
}

export function useChangesStageHandlers(gitOps: GitOps, changedFiles: unknown[]) {
  const [pendingStageFiles, setPendingStageFiles] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (pendingStageFiles.size > 0) setPendingStageFiles(new Set());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [changedFiles]);

  const handleStageAll = useCallback(async () => {
    try {
      await gitOps.stage();
    } catch {
      /* handled by gitOps */
    }
  }, [gitOps]);
  const handleStage = useCallback(
    async (path: string) => {
      setPendingStageFiles((prev) => new Set(prev).add(path));
      try {
        await gitOps.stage([path]);
      } catch {
        setPendingStageFiles((prev) => {
          const next = new Set(prev);
          next.delete(path);
          return next;
        });
      }
    },
    [gitOps],
  );
  const handleUnstage = useCallback(
    async (path: string) => {
      setPendingStageFiles((prev) => new Set(prev).add(path));
      try {
        await gitOps.unstage([path]);
      } catch {
        setPendingStageFiles((prev) => {
          const next = new Set(prev);
          next.delete(path);
          return next;
        });
      }
    },
    [gitOps],
  );

  return { pendingStageFiles, handleStageAll, handleStage, handleUnstage };
}

function useChangesDiscardCommitHandlers(
  gitOps: GitOps,
  toast: Toast,
  handleGitOperation: GitOperationFn,
) {
  const [showDiscardDialog, setShowDiscardDialog] = useState(false);
  const [fileToDiscard, setFileToDiscard] = useState<string | null>(null);
  const [commitDialogOpen, setCommitDialogOpen] = useState(false);
  const [commitMessage, setCommitMessage] = useState("");

  const handleDiscardClick = useCallback((filePath: string) => {
    setFileToDiscard(filePath);
    setShowDiscardDialog(true);
  }, []);
  const handleDiscardConfirm = useCallback(async () => {
    if (!fileToDiscard) return;
    try {
      const result = await gitOps.discard([fileToDiscard]);
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
    }
  }, [fileToDiscard, gitOps, toast]);

  const handleOpenCommitDialog = useCallback(() => {
    setCommitMessage("");
    setCommitDialogOpen(true);
  }, []);
  const handleCommit = useCallback(async () => {
    if (!commitMessage.trim()) return;
    setCommitDialogOpen(false);
    await handleGitOperation(() => gitOps.commit(commitMessage.trim(), false), "Commit");
    setCommitMessage("");
  }, [commitMessage, handleGitOperation, gitOps]);

  return {
    showDiscardDialog,
    setShowDiscardDialog,
    fileToDiscard,
    commitDialogOpen,
    setCommitDialogOpen,
    commitMessage,
    setCommitMessage,
    handleDiscardClick,
    handleDiscardConfirm,
    handleOpenCommitDialog,
    handleCommit,
  };
}

function useChangesPRHandlers(
  gitOps: GitOps,
  toast: Toast,
  taskTitle: string | undefined,
  baseBranch: string | undefined,
) {
  const [prDialogOpen, setPrDialogOpen] = useState(false);
  const [prTitle, setPrTitle] = useState("");
  const [prBody, setPrBody] = useState("");
  const [prDraft, setPrDraft] = useState(true);

  const handleOpenPRDialog = useCallback(() => {
    setPrTitle(taskTitle || "");
    setPrBody("");
    setPrDialogOpen(true);
  }, [taskTitle]);
  const handleCreatePR = useCallback(async () => {
    if (!prTitle.trim()) return;
    setPrDialogOpen(false);
    try {
      const result = await gitOps.createPR(prTitle.trim(), prBody.trim(), baseBranch, prDraft);
      if (result.success) {
        toast({
          title: prDraft ? "Draft PR created" : "PR created",
          description: result.pr_url || "PR created successfully",
          variant: "success",
        });
        if (result.pr_url) window.open(result.pr_url, "_blank");
      } else {
        toast({
          title: "Create PR failed",
          description: result.error || "An error occurred",
          variant: "error",
        });
      }
    } catch (e) {
      toast({
        title: "Create PR failed",
        description: e instanceof Error ? e.message : "An error occurred",
        variant: "error",
      });
    }
    setPrTitle("");
    setPrBody("");
  }, [prTitle, prBody, baseBranch, prDraft, gitOps, toast]);

  return {
    prDialogOpen,
    setPrDialogOpen,
    prTitle,
    setPrTitle,
    prBody,
    setPrBody,
    prDraft,
    setPrDraft,
    handleOpenPRDialog,
    handleCreatePR,
  };
}

export function useChangesDialogHandlers(
  gitOps: GitOps,
  toast: Toast,
  handleGitOperation: GitOperationFn,
  taskTitle: string | undefined,
  baseBranch: string | undefined,
) {
  const discardCommit = useChangesDiscardCommitHandlers(gitOps, toast, handleGitOperation);
  const pr = useChangesPRHandlers(gitOps, toast, taskTitle, baseBranch);
  return { ...discardCommit, ...pr };
}
