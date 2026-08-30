"use client";

import { useRef, useState, useCallback } from "react";
import { t } from "@/lib/i18n";
import { gitOperationLabel } from "@/hooks/use-git-with-feedback";
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
export function describePerRepo(
  perRepo: PerRepoOperationResult[],
  operationName: string,
): { title: string; description: string; variant: "success" | "error" } {
  const succeeded = perRepo.filter((r) => r.success);
  const failed = perRepo.filter((r) => !r.success);
  const repos = succeeded.map((r) => r.repository_name).join(", ");
  const summary = failed
    .map((r) =>
      t("common:gitOperationRepoFailure", {
        repo: r.repository_name,
        error: r.error || t("common:unknownError"),
      }),
    )
    .join("; ");
  if (failed.length === 0) {
    return {
      title: t("common:gitOperationSucceeded", { operation: operationName }),
      description: t("common:gitOperationSucceededInRepos", {
        count: succeeded.length,
        operation: operationName,
        repos,
      }),
      variant: "success",
    };
  }
  if (succeeded.length === 0) {
    return {
      title: t("common:gitOperationFailed", { operation: operationName }),
      description: t("common:gitOperationFailedInRepos", { count: failed.length, summary }),
      variant: "error",
    };
  }
  // Partial success: surface as error so the user notices, but include the
  // succeeded list in the description so they don't retry the whole op.
  //
  // `gitOperationPartialDescription` carries `_one` as well as `_other` because
  // i18next requires both, but `_one` is unreachable here by construction: this
  // branch needs at least one success AND one failure, so `count` is never 1.
  // Translators can treat the singular as a formality.
  return {
    title: t("common:gitOperationPartiallySucceeded", { operation: operationName }),
    description: t("common:gitOperationPartialDescription", {
      count: perRepo.length,
      operation: operationName,
      succeeded: succeeded.length,
      repos,
      summary,
    }),
    variant: "error",
  };
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
        const title = result.success
          ? t("common:gitOperationSucceeded", { operation: operationName })
          : t("common:gitOperationFailed", { operation: operationName });
        const description = result.success
          ? result.output.slice(0, 200) ||
            t("common:gitOperationCompleted", { operation: operationName })
          : result.error || t("common:anErrorOccurred");
        toast({ title, description, variant });
      } catch (e) {
        toast({
          title: t("common:gitOperationFailed", { operation: operationName }),
          description: e instanceof Error ? e.message : t("common:anUnexpectedErrorOccurred"),
          variant: "error",
        });
      }
    },
    [toast],
  );

  const handlePull = useCallback(
    (repo?: string) => {
      handleGitOperation(
        () => gitOps.pull(false, repo),
        gitOperationLabel(t, "common:gitOpPull", repo),
      );
    },
    [handleGitOperation, gitOps],
  );
  const handleRebase = useCallback(
    (repo?: string) => {
      const targetBranch = baseBranch?.replace(/^origin\//, "") || "main";
      handleGitOperation(
        () => gitOps.rebase(targetBranch, repo),
        gitOperationLabel(t, "common:gitOpRebase", repo),
      );
    },
    [handleGitOperation, gitOps, baseBranch],
  );
  const handleMerge = useCallback(
    (repo?: string) => {
      const targetBranch = baseBranch?.replace(/^origin\//, "") || "main";
      handleGitOperation(
        () => gitOps.merge(targetBranch, repo),
        gitOperationLabel(t, "common:gitOpMerge", repo),
      );
    },
    [handleGitOperation, gitOps, baseBranch],
  );
  const handlePush = useCallback(
    (repo?: string) => {
      handleGitOperation(
        () => gitOps.push(undefined, repo),
        gitOperationLabel(t, "common:gitOpPush", repo),
      );
    },
    [handleGitOperation, gitOps],
  );
  const handleForcePush = useCallback(
    (repo?: string) => {
      handleGitOperation(
        () => gitOps.push({ force: true }, repo),
        gitOperationLabel(t, "common:gitOpForcePush", repo),
      );
    },
    [handleGitOperation, gitOps],
  );
  const handleRevertCommit = useCallback(
    (sha: string, repo?: string) => {
      handleGitOperation(() => gitOps.revertCommit(sha, repo), t("common:gitOpRevertCommit"));
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
  const discardAnchorRef = useRef<HTMLElement | null>(null);
  // Multi-repo: remember the clicked file's repo so the discard op routes to
  // the right git repo. Path alone is ambiguous when two repos share a name.
  const [repoToDiscard, setRepoToDiscard] = useState<string | undefined>(undefined);

  const handleDiscardClick = useCallback(
    (filePath: string, repo?: string, anchor?: HTMLElement) => {
      discardAnchorRef.current = anchor ?? null;
      setFileToDiscard(filePath);
      setRepoToDiscard(repo);
      setFilesToDiscard(null);
      setShowDiscardDialog(true);
    },
    [],
  );
  const handleBulkDiscardClick = useCallback((paths: string[], anchor?: HTMLElement) => {
    discardAnchorRef.current = anchor ?? null;
    setFilesToDiscard(paths);
    setFileToDiscard(null);
    setRepoToDiscard(undefined);
    setShowDiscardDialog(true);
  }, []);
  const handleDiscardOpenChange = useCallback((open: boolean) => {
    setShowDiscardDialog(open);
    // Keep the anchor through the popover's close autofocus. A later discard
    // replaces it, while confirmation clears it after the mutation settles.
  }, []);
  const handleDiscardConfirm = useCallback(async () => {
    const paths = filesToDiscard ?? (fileToDiscard ? [fileToDiscard] : null);
    if (!paths) return;
    try {
      const result = await gitOps.discard(paths, repoToDiscard);
      if (!result.success)
        toast({
          title: t("task:failedToDiscardChanges"),
          description: result.error || t("common:anUnknownErrorOccurred"),
          variant: "error",
        });
    } catch (error) {
      toast({
        title: t("task:failedToDiscardChanges"),
        description: error instanceof Error ? error.message : t("common:anUnknownErrorOccurred"),
        variant: "error",
      });
    } finally {
      setShowDiscardDialog(false);
      discardAnchorRef.current = null;
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
      t("common:gitOpAmendCommit"),
    );
    setAmendMessage("");
    setAmendRepo(undefined);
  }, [amendMessage, amendRepo, handleGitOperation, gitOps]);

  return {
    showDiscardDialog,
    setShowDiscardDialog,
    discardAnchorRef,
    handleDiscardOpenChange,
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
      const operationName =
        mode === "hard" ? t("common:gitOpHardReset") : t("common:gitOpSoftReset");
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
