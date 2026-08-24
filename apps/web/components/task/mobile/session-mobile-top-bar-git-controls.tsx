"use client";

import { useCallback, useState } from "react";
import { ChangeRequestPartialStatus } from "@/components/vcs/vcs-dialog-fields";
import { getChangeRequestFailureFeedback } from "@/components/vcs/change-request-feedback";
import { IconGitCommit, IconGitPullRequest, IconCheck, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import {
  resolveChangeRequestTerminology,
  useChangeRequestTerminology,
  useGitOperations,
} from "@/hooks/use-git-operations";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import type { FileInfo } from "@/lib/state/slices";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { openExternalLink } from "@/lib/desktop/external-links";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";
import {
  CommitSummary,
  MobilePRBranchSummary,
  PRSubmitButton,
} from "./session-mobile-top-bar-dialog-parts";
import { useTranslation } from "react-i18next";

// A Git branch name, not copy: shown when the task has no recorded base branch.
const DEFAULT_BASE_BRANCH = "main";

export function computeUncommittedStats(files: Record<string, FileInfo> | undefined) {
  let additions = 0;
  let deletions = 0;
  if (files) {
    for (const file of Object.values(files) as FileInfo[]) {
      additions += file.additions || 0;
      deletions += file.deletions || 0;
    }
  }
  return { additions, deletions, count: files ? Object.keys(files).length : 0 };
}

type GitOperationRunner = () => Promise<{ success: boolean; output: string; error?: string }>;

function useGitToast() {
  const { t } = useTranslation();
  const { toast } = useToast();

  return useCallback(
    async (operation: GitOperationRunner, operationName: string) => {
      try {
        const result = await operation();
        if (result.success) {
          toast({
            title: t("task:successful", { operationName }),
            description:
              result.output.slice(0, 200) || t("task:completedSuccessfully", { operationName }),
            variant: "success",
          });
        } else {
          toast({
            title: t("task:failed2", { operationName }),
            description: result.error || t("task:anErrorOccurred"),
            variant: "error",
          });
        }
      } catch (error) {
        toast({
          title: t("task:failed2", { operationName }),
          description: error instanceof Error ? error.message : t("task:anUnexpectedErrorOccurred"),
          variant: "error",
        });
      }
    },
    [toast, t],
  );
}

function useCommitDialogForm(
  onOpenChange: (open: boolean) => void,
  onCommit: (message: string, stageAll: boolean) => void,
) {
  const [commitMessage, setCommitMessage] = useState("");
  const [commitBody, setCommitBody] = useState("");
  const [stageAll, setStageAll] = useState(false);

  const handleOpen = (isOpen: boolean) => {
    if (isOpen) {
      setCommitMessage("");
      setCommitBody("");
      setStageAll(false);
    }
    onOpenChange(isOpen);
  };

  const handleCommit = () => {
    const title = commitMessage.trim();
    const body = commitBody.trim();
    const fullMessage = body ? `${title}\n\n${body}` : title;
    onCommit(fullMessage, stageAll);
  };

  return {
    commitMessage,
    setCommitMessage,
    commitBody,
    setCommitBody,
    stageAll,
    setStageAll,
    handleOpen,
    handleCommit,
  };
}

export function CommitDialog({
  open,
  onOpenChange,
  uncommittedCount,
  uncommittedAdditions,
  uncommittedDeletions,
  isGitLoading,
  onCommit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  uncommittedCount: number;
  uncommittedAdditions: number;
  uncommittedDeletions: number;
  isGitLoading: boolean;
  onCommit: (message: string, stageAll: boolean) => void;
}) {
  const { t } = useTranslation();
  const form = useCommitDialogForm(onOpenChange, onCommit);

  return (
    <Dialog open={open} onOpenChange={form.handleOpen}>
      <DialogContent className="max-w-[90vw] sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconGitCommit className="h-5 w-5 text-amber-500" />
            {t("task:commitChanges")}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="text-sm text-muted-foreground">
            <CommitSummary
              uncommittedCount={uncommittedCount}
              uncommittedAdditions={uncommittedAdditions}
              uncommittedDeletions={uncommittedDeletions}
            />
          </div>
          <Input
            data-testid="commit-title-input"
            placeholder={t("task:enterCommitMessage")}
            value={form.commitMessage}
            onChange={(e) => form.setCommitMessage(e.target.value)}
            autoFocus
          />
          <div className="space-y-2">
            <Label htmlFor="commit-body-mobile" className="text-sm">
              {t("task:description2")}
            </Label>
            <Textarea
              id="commit-body-mobile"
              data-testid="commit-body-input"
              placeholder={t("task:addDetailsAboutThisChange")}
              value={form.commitBody}
              onChange={(e) => form.setCommitBody(e.target.value)}
              rows={3}
              className="resize-none max-h-[200px] overflow-y-auto"
            />
          </div>
          <div className="flex items-center gap-2">
            <Checkbox
              id="stage-all-mobile"
              checked={form.stageAll}
              onCheckedChange={(checked) => form.setStageAll(checked === true)}
            />
            <Label
              htmlFor="stage-all-mobile"
              className="text-sm text-muted-foreground cursor-pointer"
            >
              {t("task:stageAllChangesBeforeCommitting")}
            </Label>
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline">
              {t("common:cancel")}
            </Button>
          </DialogClose>
          <Button
            className="cursor-pointer"
            onClick={form.handleCommit}
            disabled={!form.commitMessage.trim() || isGitLoading}
          >
            {isGitLoading ? (
              <>
                <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
                {t("task:committingEllipsis")}
              </>
            ) : (
              <>
                <IconCheck className="h-4 w-4 mr-2" />
                {t("task:commit")}
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
  onOpenChange: (open: boolean) => void;
  displayBranch: string | undefined;
  baseBranch: string | undefined;
  isGitLoading: boolean;
  taskTitle: string | undefined;
  firstCommitMessage?: string;
  onCreatePR: (title: string, body: string, draft: boolean) => void;
  branchPushed: boolean;
};

export function PRDialog({
  open,
  onOpenChange,
  displayBranch,
  baseBranch,
  isGitLoading,
  taskTitle,
  firstCommitMessage,
  onCreatePR,
  branchPushed,
}: PRDialogProps) {
  const { t } = useTranslation();
  const [prTitle, setPrTitle] = useState("");
  const [prBody, setPrBody] = useState("");
  const [prDraft, setPrDraft] = useState(true);
  const terminology = useChangeRequestTerminology(useEnvironmentSessionId());

  const handleOpen = (isOpen: boolean) => {
    if (isOpen) {
      setPrTitle(firstCommitMessage || taskTitle || "");
      setPrBody("");
    }
    onOpenChange(isOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpen}>
      <DialogContent className="max-w-[90vw] sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconGitPullRequest className="h-5 w-5 text-cyan-500" />
            {t("task:createChangeRequestLong", { longName: terminology.longName })}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          {branchPushed && <ChangeRequestPartialStatus terminology={terminology} />}
          <MobilePRBranchSummary
            displayBranch={displayBranch}
            baseBranch={baseBranch}
            terminology={terminology}
          />
          <div className="space-y-2">
            <Label htmlFor="pr-title-mobile" className="text-sm">
              {t("common:title")}
            </Label>
            <input
              id="pr-title-mobile"
              type="text"
              aria-label={t("task:title2", { longName: terminology.longName })}
              placeholder={t("task:title3", { longName: terminology.longName })}
              value={prTitle}
              onChange={(e) => setPrTitle(e.target.value)}
              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              autoFocus
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="pr-body-mobile" className="text-sm">
              {t("task:description2")}
            </Label>
            <Textarea
              id="pr-body-mobile"
              placeholder={t("task:describeYourChanges")}
              value={prBody}
              onChange={(e) => setPrBody(e.target.value)}
              rows={4}
              className="resize-none max-h-[200px] overflow-y-auto"
            />
          </div>
          <div className="flex items-center space-x-2">
            <Checkbox
              id="pr-draft-mobile"
              checked={prDraft}
              onCheckedChange={(checked) => setPrDraft(checked === true)}
            />
            <Label htmlFor="pr-draft-mobile" className="text-sm cursor-pointer">
              {t("task:createAsDraft")}
            </Label>
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline">
              {t("common:cancel")}
            </Button>
          </DialogClose>
          <PRSubmitButton
            prTitle={prTitle}
            prBody={prBody}
            prDraft={prDraft}
            isGitLoading={isGitLoading}
            onCreatePR={onCreatePR}
            terminology={terminology}
            branchPushed={branchPushed}
          />
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export { GitActionsDropdown } from "./mobile-git-actions-dropdown";
export type { GitActionsDropdownProps } from "./mobile-git-actions-dropdown";

/** The success toast for a created change request. Extracted to keep
 *  `useMobileGitActions` under the function-length cap. */
function createPrSuccessToast(
  longName: string,
  prUrl: string | undefined,
  draft: boolean,
  t: (key: string, opts?: Record<string, unknown>) => string,
) {
  return {
    title: draft ? t("task:draftCreated", { longName }) : t("task:created2", { longName }),
    description: prUrl || t("task:createdSuccessfully", { longName }),
    variant: "success" as const,
  };
}

type MobileGitActionContext = {
  sessionId: string | null | undefined;
  baseBranch: string | undefined;
  pullRequestTarget: string | undefined;
};

export function useMobileGitActions(
  { sessionId, baseBranch, pullRequestTarget }: MobileGitActionContext,
  setCommitDialogOpen: (v: boolean) => void,
  setPrDialogOpen: (v: boolean) => void,
  setPrBranchPushed: (v: boolean) => void,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const setPendingPrUrlForTask = useAppStore((state) => state.setPendingPrUrlForTask);
  const defaultTerminology = useChangeRequestTerminology(sessionId);
  // SessionGit's commit fans out per-repo for multi-repo workspaces; the raw
  // useGitOperations.commit hits the workspace root and fails on multi-repo
  // tasks. Pull from useGitOperations only what SessionGit doesn't override.
  const gitOperations = useGitOperations(sessionId ?? null);
  const { pull, push, rebase, merge, createPR, isLoading: isGitLoading } = gitOperations;
  const { commit } = useSessionGit(sessionId ?? null);
  const handleGitOperation = useGitToast();

  // Hoisted out of the two callbacks below so both share one expression.
  const remoteTarget = baseBranch?.replace(/^origin\//, "") || DEFAULT_BASE_BRANCH;

  const handlePull = useCallback(
    () => handleGitOperation(() => pull(), t("task:pull")),
    [handleGitOperation, pull, t],
  );
  const handlePush = useCallback(
    (force = false) =>
      handleGitOperation(() => push({ force }), force ? t("task:forcePush") : t("task:push")),
    [handleGitOperation, push, t],
  );
  const handleRebase = useCallback(
    () => handleGitOperation(() => rebase(remoteTarget), t("task:rebase")),
    [handleGitOperation, rebase, remoteTarget, t],
  );
  const handleMerge = useCallback(
    () => handleGitOperation(() => merge(remoteTarget), t("task:merge")),
    [handleGitOperation, merge, remoteTarget, t],
  );

  const handleCommit = useCallback(
    async (message: string, stageAll: boolean) => {
      setCommitDialogOpen(false);
      await handleGitOperation(() => commit(message, stageAll), t("task:commit"));
    },
    [handleGitOperation, commit, setCommitDialogOpen, t],
  );

  const handleCreatePR = useCallback(
    async (title: string, body: string, draft: boolean) => {
      setPrDialogOpen(false);
      try {
        const result = await createPR(title, body, pullRequestTarget ?? baseBranch, draft);
        if (result.success) {
          const terms = resolveChangeRequestTerminology(result.provider, defaultTerminology);
          toast(createPrSuccessToast(terms.longName, result.pr_url, draft, t));
          if (result.pr_url) {
            if (activeTaskId) {
              setPendingPrUrlForTask(activeTaskId, "", result.pr_url);
            }
            void openExternalLink(result.pr_url).catch(() => undefined);
          }
        } else {
          const feedback = getChangeRequestFailureFeedback(result, defaultTerminology);
          toast(feedback);
          if (result.branch_pushed) {
            setPrBranchPushed(true);
            setPrDialogOpen(true);
            return;
          }
        }
        setPrBranchPushed(false);
      } catch (e) {
        toast({
          title: t("task:createFailed", { shortName: defaultTerminology.shortName }),
          description: e instanceof Error ? e.message : t("task:anErrorOccurred"),
          variant: "error",
        });
      }
    },
    [
      createPR,
      baseBranch,
      pullRequestTarget,
      toast,
      setPrDialogOpen,
      setPrBranchPushed,
      defaultTerminology,
      activeTaskId,
      setPendingPrUrlForTask,
      t,
    ],
  );

  return {
    isGitLoading,
    handlePull,
    handlePush,
    handleRebase,
    handleMerge,
    handleCommit,
    handleCreatePR,
  };
}
