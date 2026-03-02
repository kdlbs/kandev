"use client";

import { createContext, useContext, useState, useCallback, useMemo, type ReactNode } from "react";
import {
  IconGitCommit,
  IconGitPullRequest,
  IconLoader2,
  IconCheck,
  IconSparkles,
} from "@tabler/icons-react";
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
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useGitOperations } from "@/hooks/use-git-operations";
import { useGitWithFeedback } from "@/hooks/use-git-with-feedback";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useToast } from "@/components/toast-provider";
import type { FileInfo } from "@/lib/state/slices";

type VcsDialogsContextValue = {
  openCommitDialog: () => void;
  openPRDialog: () => void;
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

function computeFileSummary(files: Record<string, FileInfo> | undefined): FileSummary {
  const count = files ? Object.keys(files).length : 0;
  let additions = 0;
  let deletions = 0;
  if (files && count > 0) {
    for (const file of Object.values(files) as FileInfo[]) {
      additions += file.additions || 0;
      deletions += file.deletions || 0;
    }
  }
  return { count, additions, deletions };
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

type GenerateButtonProps = {
  onClick: () => void;
  isGenerating: boolean;
  disabled?: boolean;
  tooltip: string;
  size?: "icon" | "sm";
  showLabel?: boolean;
};

function GenerateButton({
  onClick,
  isGenerating,
  disabled,
  tooltip,
  size = "icon",
  showLabel,
}: GenerateButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          size={size}
          variant="ghost"
          className={
            size === "icon" ? "h-7 w-7 cursor-pointer" : "h-6 px-2 cursor-pointer gap-1.5 text-xs"
          }
          onClick={onClick}
          disabled={disabled || isGenerating}
        >
          {isGenerating ? (
            <IconLoader2 className="h-4 w-4 animate-spin" />
          ) : (
            <IconSparkles className="h-4 w-4" />
          )}
          {showLabel && "Generate"}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

type CommitDialogProps = {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  fileSummary: FileSummary;
  commitMessage: string;
  onCommitMessageChange: (v: string) => void;
  stageAll: boolean;
  onStageAllChange: (v: boolean) => void;
  isGitLoading: boolean;
  onCommit: () => void;
  onGenerateMessage?: () => void;
  isGenerating?: boolean;
};

function CommitDialog({
  open,
  onOpenChange,
  fileSummary,
  commitMessage,
  onCommitMessageChange,
  stageAll,
  onStageAllChange,
  isGitLoading,
  onCommit,
  onGenerateMessage,
  isGenerating,
}: CommitDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconGitCommit className="h-5 w-5" />
            Commit Changes
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="text-sm text-muted-foreground">
            <FileSummaryText {...fileSummary} />
          </div>
          <div className="relative">
            <Input
              placeholder="Enter commit message..."
              value={commitMessage}
              onChange={(e) => onCommitMessageChange(e.target.value)}
              className="pr-10"
              autoFocus
            />
            {onGenerateMessage && (
              <div className="absolute right-1.5 top-1/2 -translate-y-1/2">
                <GenerateButton
                  onClick={onGenerateMessage}
                  isGenerating={isGenerating ?? false}
                  disabled={fileSummary.count === 0}
                  tooltip="Generate commit message with AI"
                />
              </div>
            )}
          </div>
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
            <Button type="button" variant="outline">
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
  onGenerateDescription?: () => void;
  isGenerating?: boolean;
};

function PRBranchSummary({
  displayBranch,
  baseBranch,
}: {
  displayBranch?: string | null;
  baseBranch?: string;
}) {
  if (!displayBranch) return null;
  return (
    <div className="text-sm text-muted-foreground">
      {baseBranch ? (
        <span>
          Creating PR from <span className="font-medium text-foreground">{displayBranch}</span>
          {" → "}
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

function PRDialog({
  open,
  onOpenChange,
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
  onGenerateDescription,
  isGenerating,
}: PRDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconGitPullRequest className="h-5 w-5" />
            Create Pull Request
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <PRBranchSummary displayBranch={displayBranch} baseBranch={baseBranch} />
          <div className="space-y-2">
            <Label htmlFor="vcs-pr-title" className="text-sm">
              Title
            </Label>
            <Input
              id="vcs-pr-title"
              placeholder="Pull request title..."
              value={prTitle}
              onChange={(e) => onPrTitleChange(e.target.value)}
              autoFocus
            />
          </div>
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="vcs-pr-body" className="text-sm">
                Description
              </Label>
              {onGenerateDescription && (
                <GenerateButton
                  onClick={onGenerateDescription}
                  isGenerating={isGenerating ?? false}
                  tooltip="Generate PR description with AI"
                  size="sm"
                  showLabel
                />
              )}
            </div>
            <Textarea
              id="vcs-pr-body"
              placeholder="Describe your changes..."
              value={prBody}
              onChange={(e) => onPrBodyChange(e.target.value)}
              rows={6}
              className="resize-none max-h-[200px] overflow-y-auto"
            />
          </div>
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
            <Button type="button" variant="outline">
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
  stageAll: boolean;
  setStageAll: (v: boolean) => void;
  openDialog: () => void;
};

function useCommitDialogState(): UseCommitDialogReturn {
  const [open, setOpen] = useState(false);
  const [message, setMessage] = useState("");
  const [stageAll, setStageAll] = useState(true);
  const openDialog = useCallback(() => {
    setMessage("");
    setStageAll(true);
    setOpen(true);
  }, []);
  return { open, setOpen, message, setMessage, stageAll, setStageAll, openDialog };
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
  openDialog: (taskTitle?: string) => void;
};

function usePRDialogState(): UsePRDialogReturn {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [draft, setDraft] = useState(true);
  const openDialog = useCallback((taskTitle?: string) => {
    setTitle(taskTitle || "");
    setBody("");
    setOpen(true);
  }, []);
  return { open, setOpen, title, setTitle, body, setBody, draft, setDraft, openDialog };
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
      const result = await createPR(ps.title.trim(), ps.body.trim(), baseBranch, ps.draft);
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

export function VcsDialogsProvider({
  sessionId,
  baseBranch,
  taskTitle,
  displayBranch,
  children,
}: VcsDialogsProviderProps) {
  const cs = useCommitDialogState();
  const ps = usePRDialogState();
  const { toast } = useToast();
  const gitWithFeedback = useGitWithFeedback();
  const gitStatus = useSessionGitStatus(sessionId);
  const { commit, createPR, isLoading: isGitLoading } = useGitOperations(sessionId);
  const isUtilityConfigured = useIsUtilityConfigured();
  const {
    isGeneratingCommitMessage,
    isGeneratingPRDescription,
    generateCommitMessage,
    generatePRDescription,
  } = useUtilityAgentGenerator({ sessionId, taskTitle });
  const fileSummary = computeFileSummary(gitStatus?.files);

  const handleCommit = useCallback(async () => {
    if (!cs.message.trim()) return;
    cs.setOpen(false);
    await gitWithFeedback(() => commit(cs.message.trim(), cs.stageAll), "Commit");
    cs.setMessage("");
  }, [cs, gitWithFeedback, commit]);

  const handleCreatePR = useCreatePRHandler(ps, baseBranch, createPR, toast);

  const contextValue = useMemo(
    () => ({
      openCommitDialog: cs.openDialog,
      openPRDialog: () => ps.openDialog(taskTitle),
    }),
    [cs.openDialog, ps, taskTitle],
  );

  return (
    <VcsDialogsContext.Provider value={contextValue}>
      {children}
      <CommitDialog
        open={cs.open}
        onOpenChange={cs.setOpen}
        fileSummary={fileSummary}
        commitMessage={cs.message}
        onCommitMessageChange={cs.setMessage}
        stageAll={cs.stageAll}
        onStageAllChange={cs.setStageAll}
        isGitLoading={isGitLoading}
        onCommit={handleCommit}
        onGenerateMessage={
          isUtilityConfigured ? () => generateCommitMessage(cs.setMessage) : undefined
        }
        isGenerating={isUtilityConfigured ? isGeneratingCommitMessage : undefined}
      />
      <PRDialog
        open={ps.open}
        onOpenChange={ps.setOpen}
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
        onGenerateDescription={
          isUtilityConfigured ? () => generatePRDescription(ps.setBody) : undefined
        }
        isGenerating={isUtilityConfigured ? isGeneratingPRDescription : undefined}
      />
    </VcsDialogsContext.Provider>
  );
}
