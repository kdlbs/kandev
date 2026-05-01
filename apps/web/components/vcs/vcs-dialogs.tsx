"use client";

import { createContext, useContext, useState, useCallback, useMemo, type ReactNode } from "react";
import { IconGitCommit, IconGitPullRequest, IconLoader2, IconCheck } from "@tabler/icons-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import {
  GenerateButton,
  CommitBodyField,
  PRTitleField,
  PRDescriptionField,
  PRBranchSummary,
} from "./vcs-dialog-fields";
import {
  useSessionGitStatus,
  useSessionGitStatusByRepo,
} from "@/hooks/domains/session/use-session-git-status";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import { useRepoDisplayName } from "@/hooks/domains/session/use-repo-display-name";
import { useGitOperations } from "@/hooks/use-git-operations";
import { useGitWithFeedback } from "@/hooks/use-git-with-feedback";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useToast } from "@/components/toast-provider";
import type { FileInfo } from "@/lib/state/slices";

type VcsDialogsContextValue = {
  /** When `repo` is provided, the commit is scoped to that repo only. */
  openCommitDialog: (repo?: string) => void;
  /** When `repo` is provided, the PR is scoped to that repo only. */
  openPRDialog: (repo?: string) => void;
};

const VcsDialogsContext = createContext<VcsDialogsContextValue | null>(null);

export function useVcsDialogs() {
  const ctx = useContext(VcsDialogsContext);
  if (!ctx) throw new Error("useVcsDialogs must be used within VcsDialogsProvider");
  return ctx;
}

type VcsDialogsProviderProps = {
  sessionId: string | null;
  baseBranch?: string;
  taskTitle?: string;
  displayBranch?: string | null;
  children: ReactNode;
};

type FileSummary = { count: number; additions: number; deletions: number };

/**
 * Counts files for the commit dialog summary.
 * - When `stageAll=true` (the default), include every file (staged + unstaged)
 *   because the commit op stages them all before committing.
 * - When `stageAll=false`, count only staged files — those are the only files
 *   the commit will actually include. Counting all here would over-state what
 *   the commit produces and surprise the user post-commit.
 */
function computeFileSummary(
  files: Record<string, FileInfo> | undefined,
  stageAll: boolean = true,
): FileSummary {
  if (!files) return { count: 0, additions: 0, deletions: 0 };
  const considered = (Object.values(files) as FileInfo[]).filter((f) => stageAll || f.staged);
  let additions = 0;
  let deletions = 0;
  for (const file of considered) {
    additions += file.additions || 0;
    deletions += file.deletions || 0;
  }
  return { count: considered.length, additions, deletions };
}

function FileSummaryText({ count, additions, deletions }: FileSummary) {
  if (count === 0) return <span>No changes to commit</span>;
  return (
    <span>
      <span className="font-medium text-foreground">{count}</span> file{count !== 1 ? "s" : ""}{" "}
      changed
      {(additions > 0 || deletions > 0) && (
        <span className="ml-2">
          (<span className="text-green-600">+{additions}</span>
          {" / "}
          <span className="text-red-600">-{deletions}</span>)
        </span>
      )}
    </span>
  );
}

type CommitDialogProps = {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  /** When set, the dialog title shows the repo name and the summary is repo-scoped. */
  scopedRepo?: string;
  fileSummary: FileSummary;
  commitMessage: string;
  onCommitMessageChange: (v: string) => void;
  commitBody: string;
  onCommitBodyChange: (v: string) => void;
  stageAll: boolean;
  onStageAllChange: (v: boolean) => void;
  isGitLoading: boolean;
  onCommit: () => void;
  onGenerateMessage: () => void;
  isGenerating: boolean;
  onGenerateDescription: () => void;
  isGeneratingDescription: boolean;
  isUtilityConfigured: boolean;
};

function CommitDialog({
  open,
  onOpenChange,
  scopedRepo,
  fileSummary,
  commitMessage,
  onCommitMessageChange,
  commitBody,
  onCommitBodyChange,
  stageAll,
  onStageAllChange,
  isGitLoading,
  onCommit,
  onGenerateMessage,
  isGenerating,
  onGenerateDescription,
  isGeneratingDescription,
  isUtilityConfigured,
}: CommitDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconGitCommit className="h-5 w-5" />
            {scopedRepo ? `Commit Changes — ${scopedRepo}` : "Commit Changes"}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="text-sm text-muted-foreground">
            <FileSummaryText {...fileSummary} />
          </div>
          <div className="relative min-w-0">
            <Input
              data-testid="commit-title-input"
              placeholder="Enter commit message..."
              value={commitMessage}
              onChange={(e) => onCommitMessageChange(e.target.value)}
              className="pr-10"
              autoFocus
            />
            <div className="absolute right-1.5 top-1/2 -translate-y-1/2">
              <GenerateButton
                onClick={onGenerateMessage}
                isGenerating={isGenerating}
                disabled={fileSummary.count === 0}
                tooltip="Generate commit message with AI"
                isConfigured={isUtilityConfigured}
              />
            </div>
          </div>
          <CommitBodyField
            commitBody={commitBody}
            onCommitBodyChange={onCommitBodyChange}
            onGenerateDescription={onGenerateDescription}
            isGeneratingDescription={isGeneratingDescription}
            isUtilityConfigured={isUtilityConfigured}
            disabled={fileSummary.count === 0}
          />
          <div className="flex items-center gap-2">
            <Checkbox
              id="vcs-stage-all"
              checked={stageAll}
              onCheckedChange={(checked) => onStageAllChange(checked === true)}
            />
            <Label htmlFor="vcs-stage-all" className="text-sm text-muted-foreground cursor-pointer">
              Stage all changes before committing
            </Label>
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline" className="cursor-pointer">
              Cancel
            </Button>
          </DialogClose>
          <Button onClick={onCommit} disabled={!commitMessage.trim() || isGitLoading}>
            {isGitLoading ? (
              <>
                <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
                Committing...
              </>
            ) : (
              <>
                <IconCheck className="h-4 w-4 mr-2" />
                Commit
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type PRDialogProps = {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  /** When set, the dialog title shows the repo name. */
  scopedRepo?: string;
  displayBranch?: string | null;
  baseBranch?: string;
  prTitle: string;
  onPrTitleChange: (v: string) => void;
  prBody: string;
  onPrBodyChange: (v: string) => void;
  prDraft: boolean;
  onPrDraftChange: (v: boolean) => void;
  isGitLoading: boolean;
  onCreatePR: () => void;
  onGenerateTitle: () => void;
  isGeneratingTitle: boolean;
  onGenerateDescription: () => void;
  isGeneratingDescription: boolean;
  isUtilityConfigured: boolean;
};

function PRDialog({
  open,
  onOpenChange,
  scopedRepo,
  displayBranch,
  baseBranch,
  prTitle,
  onPrTitleChange,
  prBody,
  onPrBodyChange,
  prDraft,
  onPrDraftChange,
  isGitLoading,
  onCreatePR,
  onGenerateTitle,
  isGeneratingTitle,
  onGenerateDescription,
  isGeneratingDescription,
  isUtilityConfigured,
}: PRDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconGitPullRequest className="h-5 w-5" />
            {scopedRepo ? `Create Pull Request — ${scopedRepo}` : "Create Pull Request"}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <PRBranchSummary displayBranch={displayBranch} baseBranch={baseBranch} />
          <PRTitleField
            prTitle={prTitle}
            onPrTitleChange={onPrTitleChange}
            onGenerateTitle={onGenerateTitle}
            isGeneratingTitle={isGeneratingTitle}
            isUtilityConfigured={isUtilityConfigured}
          />
          <PRDescriptionField
            prBody={prBody}
            onPrBodyChange={onPrBodyChange}
            onGenerateDescription={onGenerateDescription}
            isGeneratingDescription={isGeneratingDescription}
            isUtilityConfigured={isUtilityConfigured}
          />
          <div className="flex items-center space-x-2">
            <Checkbox
              id="vcs-pr-draft"
              checked={prDraft}
              onCheckedChange={(checked) => onPrDraftChange(checked === true)}
            />
            <Label htmlFor="vcs-pr-draft" className="text-sm cursor-pointer">
              Create as draft
            </Label>
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline" className="cursor-pointer">
              Cancel
            </Button>
          </DialogClose>
          <Button onClick={onCreatePR} disabled={!prTitle.trim() || isGitLoading}>
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
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type UseCommitDialogReturn = {
  open: boolean;
  setOpen: (v: boolean) => void;
  message: string;
  setMessage: (v: string) => void;
  body: string;
  setBody: (v: string) => void;
  stageAll: boolean;
  setStageAll: (v: boolean) => void;
  /** Repo this commit is scoped to in multi-repo mode; "" = all repos with staged. */
  repo: string;
  setRepo: (v: string) => void;
  openDialog: (repo?: string) => void;
};

function useCommitDialogState(): UseCommitDialogReturn {
  const [open, setOpen] = useState(false);
  const [message, setMessage] = useState("");
  const [body, setBody] = useState("");
  const [stageAll, setStageAll] = useState(true);
  const [repo, setRepo] = useState("");
  const openDialog = useCallback((nextRepo?: string) => {
    setMessage("");
    setBody("");
    setStageAll(true);
    // Defensive: callers binding `openDialog` directly to onClick can leak the
    // React MouseEvent into nextRepo. Only accept actual repo strings.
    setRepo(typeof nextRepo === "string" ? nextRepo : "");
    setOpen(true);
  }, []);
  return {
    open,
    setOpen,
    message,
    setMessage,
    body,
    setBody,
    stageAll,
    setStageAll,
    repo,
    setRepo,
    openDialog,
  };
}

type UsePRDialogReturn = {
  open: boolean;
  setOpen: (v: boolean) => void;
  title: string;
  setTitle: (v: string) => void;
  body: string;
  setBody: (v: string) => void;
  draft: boolean;
  setDraft: (v: boolean) => void;
  /** Repo this PR is scoped to in multi-repo mode; "" = workspace root. */
  repo: string;
  setRepo: (v: string) => void;
  openDialog: (taskTitle?: string, repo?: string) => void;
};

function usePRDialogState(): UsePRDialogReturn {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [draft, setDraft] = useState(true);
  const [repo, setRepo] = useState("");
  const openDialog = useCallback((taskTitle?: string, nextRepo?: string) => {
    setTitle(taskTitle || "");
    setBody("");
    // Defensive: callers binding `openDialog` directly to onClick can leak the
    // React MouseEvent into nextRepo. Only accept actual repo strings.
    setRepo(typeof nextRepo === "string" ? nextRepo : "");
    setOpen(true);
  }, []);
  return {
    open,
    setOpen,
    title,
    setTitle,
    body,
    setBody,
    draft,
    setDraft,
    repo,
    setRepo,
    openDialog,
  };
}

/**
 * Computes the file summary for the commit dialog: an explicit repo scope
 * uses that repo's files; an empty scope in multi-repo sums across every
 * repo (showing the fan-out total); single-repo falls back to the legacy
 * workspace-level status.
 */
function useScopedFileSummary({
  scopedRepo,
  statusByRepo,
  gitStatus,
  isMultiRepo,
  stageAll,
}: {
  scopedRepo: string;
  statusByRepo: ReturnType<typeof useSessionGitStatusByRepo>;
  gitStatus: ReturnType<typeof useSessionGitStatus>;
  isMultiRepo: boolean;
  /** Mirrors the dialog's "Stage all changes before committing" checkbox. */
  stageAll: boolean;
}): FileSummary {
  return useMemo(() => {
    if (scopedRepo) {
      const scoped = statusByRepo.find((s) => s.repository_name === scopedRepo);
      return computeFileSummary(scoped?.status?.files, stageAll);
    }
    if (isMultiRepo) {
      let count = 0;
      let additions = 0;
      let deletions = 0;
      for (const { status } of statusByRepo) {
        const s = computeFileSummary(status?.files, stageAll);
        count += s.count;
        additions += s.additions;
        deletions += s.deletions;
      }
      return { count, additions, deletions };
    }
    return computeFileSummary(gitStatus?.files, stageAll);
  }, [scopedRepo, statusByRepo, gitStatus, isMultiRepo, stageAll]);
}

/**
 * Resolves the label shown in dialog titles. Explicit repo wins; otherwise
 * empty scope resolves to the primary single-repo display name, or "All
 * repos" when the workspace has multiple repos and the dialog is fanning out.
 */
function pickRepoLabel(
  scopedRepo: string,
  isMultiRepo: boolean,
  resolveDisplayName: (name: string) => string | undefined,
): string {
  if (scopedRepo) return resolveDisplayName(scopedRepo) || scopedRepo;
  if (isMultiRepo) return "All repos";
  return resolveDisplayName("") || "Repository";
}

function useCreatePRHandler(
  ps: UsePRDialogReturn,
  baseBranch: string | undefined,
  createPR: ReturnType<typeof useGitOperations>["createPR"],
  toast: ReturnType<typeof useToast>["toast"],
) {
  return useCallback(async () => {
    if (!ps.title.trim()) return;
    ps.setOpen(false);
    try {
      const result = await createPR(
        ps.title.trim(),
        ps.body.trim(),
        baseBranch,
        ps.draft,
        ps.repo || undefined,
      );
      if (result.success) {
        const title = ps.draft ? "Draft PR created" : "PR created";
        toast({
          title,
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
    ps.setTitle("");
    ps.setBody("");
  }, [ps, baseBranch, createPR, toast]);
}

function useVcsDialogsState(
  sessionId: string | null,
  taskTitle: string | undefined,
  baseBranch: string | undefined,
) {
  const cs = useCommitDialogState();
  const ps = usePRDialogState();
  const { toast } = useToast();
  const gitWithFeedback = useGitWithFeedback();
  const gitStatus = useSessionGitStatus(sessionId);
  const statusByRepo = useSessionGitStatusByRepo(sessionId);
  // Use SessionGit so commit fans out per-repo for multi-repo workspaces.
  // useGitOperations.commit hits the workspace root, which fails for multi-repo
  // tasks because the task root isn't itself a git repo (exit 1).
  const { commit, createPR, repoNames, isLoading: isGitLoading } = useSessionGit(sessionId);
  const repoDisplayName = useRepoDisplayName(sessionId);
  const isMultiRepo = repoNames.filter((r) => r !== "").length > 1;
  const fileSummary = useScopedFileSummary({
    scopedRepo: cs.repo,
    statusByRepo,
    gitStatus,
    isMultiRepo,
    stageAll: cs.stageAll,
  });
  const handleCommit = useCallback(async () => {
    if (!cs.message.trim()) return;
    cs.setOpen(false);
    const title = cs.message.trim();
    const body = cs.body.trim();
    const fullMessage = body ? `${title}\n\n${body}` : title;
    const label = cs.repo ? `Commit (${cs.repo})` : "Commit";
    await gitWithFeedback(
      () => commit(fullMessage, cs.stageAll, false, cs.repo || undefined),
      label,
    );
    cs.setMessage("");
    cs.setBody("");
    cs.setRepo("");
  }, [cs, gitWithFeedback, commit]);
  const handleCreatePR = useCreatePRHandler(ps, baseBranch, createPR, toast);
  const contextValue = useMemo(
    () => ({
      openCommitDialog: cs.openDialog,
      openPRDialog: (repo?: string) => ps.openDialog(taskTitle, repo),
    }),
    [cs.openDialog, ps, taskTitle],
  );
  return {
    cs,
    ps,
    isGitLoading,
    fileSummary,
    handleCommit,
    handleCreatePR,
    contextValue,
    repoDisplayName,
    isMultiRepo,
  };
}

export function VcsDialogsProvider({
  sessionId,
  baseBranch,
  taskTitle,
  displayBranch,
  children,
}: VcsDialogsProviderProps) {
  const state = useVcsDialogsState(sessionId, taskTitle, baseBranch);
  const { cs, ps, isGitLoading, fileSummary, handleCommit, handleCreatePR, contextValue } = state;
  const effectiveRepoLabel = pickRepoLabel(cs.repo, state.isMultiRepo, state.repoDisplayName);
  const effectivePRLabel = pickRepoLabel(ps.repo, state.isMultiRepo, state.repoDisplayName);
  const isUtilityConfigured = useIsUtilityConfigured();
  const {
    isGeneratingCommitMessage,
    isGeneratingCommitDescription,
    isGeneratingPRTitle,
    isGeneratingPRDescription,
    generateCommitMessage,
    generateCommitDescription,
    generatePRTitle,
    generatePRDescription,
  } = useUtilityAgentGenerator({ sessionId, taskTitle });

  return (
    <VcsDialogsContext.Provider value={contextValue}>
      {children}
      <CommitDialog
        open={cs.open}
        onOpenChange={cs.setOpen}
        scopedRepo={effectiveRepoLabel}
        fileSummary={fileSummary}
        commitMessage={cs.message}
        onCommitMessageChange={cs.setMessage}
        commitBody={cs.body}
        onCommitBodyChange={cs.setBody}
        stageAll={cs.stageAll}
        onStageAllChange={cs.setStageAll}
        isGitLoading={isGitLoading}
        onCommit={handleCommit}
        onGenerateMessage={() => generateCommitMessage(cs.setMessage)}
        isGenerating={isGeneratingCommitMessage}
        onGenerateDescription={() => generateCommitDescription(cs.setBody)}
        isGeneratingDescription={isGeneratingCommitDescription}
        isUtilityConfigured={isUtilityConfigured}
      />
      <PRDialog
        open={ps.open}
        onOpenChange={ps.setOpen}
        scopedRepo={effectivePRLabel}
        displayBranch={displayBranch}
        baseBranch={baseBranch}
        prTitle={ps.title}
        onPrTitleChange={ps.setTitle}
        prBody={ps.body}
        onPrBodyChange={ps.setBody}
        prDraft={ps.draft}
        onPrDraftChange={ps.setDraft}
        isGitLoading={isGitLoading}
        onCreatePR={handleCreatePR}
        onGenerateTitle={() => generatePRTitle(ps.setTitle)}
        isGeneratingTitle={isGeneratingPRTitle}
        onGenerateDescription={() => generatePRDescription(ps.setBody)}
        isGeneratingDescription={isGeneratingPRDescription}
        isUtilityConfigured={isUtilityConfigured}
      />
    </VcsDialogsContext.Provider>
  );
}
