"use client";

import { createContext, useContext, useState, useCallback, useMemo, type ReactNode } from "react";
import { IconGitCommit, IconLoader2, IconCheck } from "@tabler/icons-react";
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
import { GenerateButton, CommitBodyField } from "./vcs-dialog-fields";
import { VcsChangeRequestDialog } from "./vcs-change-request-dialog";
import {
  useSessionGitStatus,
  useSessionGitStatusByRepo,
} from "@/hooks/domains/session/use-session-git-status";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import { useRepoDisplayName } from "@/hooks/domains/session/use-repo-display-name";
import { useChangeRequestTerminology } from "@/hooks/use-git-operations";
import { gitOperationLabel, useGitWithFeedback } from "@/hooks/use-git-with-feedback";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import type { FileInfo } from "@/lib/state/slices";
import { Trans, useTranslation } from "react-i18next";
import { createChangeRequestWithProvider } from "@/lib/plugins/change-request-creation";
import { useChangeRequestProviderTarget } from "@/hooks/use-change-request-provider-target";
import {
  useCreateChangeRequestHandler,
  type CreateChangeRequestInput,
} from "./use-create-change-request-handler";

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
  pullRequestBaseBranch?: string;
  pullRequestTargetsByRepository?: Record<string, string>;
  taskTitle?: string;
  displayBranch?: string | null;
  children: ReactNode;
};

export function resolvePullRequestBaseBranch(
  selectedRepo: string | undefined,
  targetsByRepository: Record<string, string> | undefined,
  fallback: string | undefined,
): string | undefined {
  if (!selectedRepo || !targetsByRepository) return fallback;
  const direct = targetsByRepository[selectedRepo];
  if (direct) return direct;
  const selectedBase = selectedRepo.split(" · ")[0];
  const baseTarget = targetsByRepository[selectedBase];
  if (baseTarget) return baseTarget;
  return fallback;
}

type FileSummary = { count: number; additions: number; deletions: number };

/**
 * Counts files for the commit dialog summary.
 * - When `stageAll=true`, include every file (staged + unstaged) because the
 *   commit op stages them all before committing.
 * - When `stageAll=false` (the default), count only staged files — those are
 *   the only files the commit will actually include. Counting all here would
 *   over-state what the commit produces and surprise the user post-commit.
 */
function computeFileSummary(
  files: Record<string, FileInfo> | undefined,
  stageAll: boolean = false,
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
  const { t } = useTranslation();
  if (count === 0) return <span>{t("integrations:noChangesToCommit")}</span>;
  return (
    <span>
      <Trans i18nKey="integrations:filesChanged" count={count} values={{ count }}>
        <span className="font-medium text-foreground">{count}</span> files changed
      </Trans>
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
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconGitCommit className="h-5 w-5" />
            {scopedRepo
              ? t("integrations:commitChangesScoped", { scopedRepo })
              : t("integrations:commitChanges")}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="text-sm text-muted-foreground">
            <FileSummaryText {...fileSummary} />
          </div>
          <div className="relative min-w-0">
            <Input
              data-testid="commit-title-input"
              placeholder={t("integrations:enterCommitMessage")}
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
                tooltip={t("integrations:generateCommitMessageWithAi")}
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
              {t("integrations:stageAllChangesBeforeCommitting")}
            </Label>
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline" className="cursor-pointer">
              {t("common:cancel")}
            </Button>
          </DialogClose>
          <Button onClick={onCommit} disabled={!commitMessage.trim() || isGitLoading}>
            {isGitLoading ? (
              <>
                <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
                {t("integrations:committingEllipsis")}
              </>
            ) : (
              <>
                <IconCheck className="h-4 w-4 mr-2" />
                {t("integrations:commit")}
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
  /** Undefined means all repos; "" is an explicit workspace-root scope. */
  repo: string | undefined;
  setRepo: (v: string | undefined) => void;
  openDialog: (repo?: string) => void;
};

function useCommitDialogState(): UseCommitDialogReturn {
  const [open, setOpen] = useState(false);
  const [message, setMessage] = useState("");
  const [body, setBody] = useState("");
  const [stageAll, setStageAll] = useState(false);
  const [repo, setRepo] = useState<string | undefined>(undefined);
  const openDialog = useCallback((nextRepo?: string) => {
    setMessage("");
    setBody("");
    setStageAll(false);
    // Defensive: callers binding `openDialog` directly to onClick can leak the
    // React MouseEvent into nextRepo. Only accept actual repo strings.
    setRepo(typeof nextRepo === "string" ? nextRepo : undefined);
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
  branchPushed: boolean;
  setBranchPushed: (v: boolean) => void;
  /** Undefined means the default scope; "" is an explicit workspace-root scope. */
  repo: string | undefined;
  setRepo: (v: string | undefined) => void;
  openDialog: (taskTitle?: string, repo?: string) => void;
};

function usePRDialogState(): UsePRDialogReturn {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [draft, setDraft] = useState(true);
  const [branchPushed, setBranchPushed] = useState(false);
  const [repo, setRepo] = useState<string | undefined>(undefined);
  const openDialog = useCallback((taskTitle?: string, nextRepo?: string) => {
    setTitle(taskTitle || "");
    setBody("");
    setBranchPushed(false);
    // Defensive: callers binding `openDialog` directly to onClick can leak the
    // React MouseEvent into nextRepo. Only accept actual repo strings.
    setRepo(typeof nextRepo === "string" ? nextRepo : undefined);
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
    branchPushed,
    setBranchPushed,
    repo,
    setRepo,
    openDialog,
  };
}

/**
 * Computes the file summary for the commit dialog: an explicit repo scope
 * uses that repo's files; no scope in multi-repo sums across every repo
 * (showing the fan-out total); single-repo falls back to the legacy
 * workspace-level status.
 */
function useScopedFileSummary({
  scopedRepo,
  statusByRepo,
  gitStatus,
  isMultiRepo,
  stageAll,
}: {
  scopedRepo: string | undefined;
  statusByRepo: ReturnType<typeof useSessionGitStatusByRepo>;
  gitStatus: ReturnType<typeof useSessionGitStatus>;
  isMultiRepo: boolean;
  /** Mirrors the dialog's "Stage all changes before committing" checkbox. */
  stageAll: boolean;
}): FileSummary {
  return useMemo(() => {
    if (scopedRepo !== undefined) {
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
export function pickRepoLabel(
  scopedRepo: string | undefined,
  isMultiRepo: boolean,
  resolveDisplayName: (name: string) => string | undefined,
  // Structural, matching the other translate-taking helpers in the app: this
  // only ever looks a key up, so both the hook's `t` and the module-level one
  // from `@/lib/i18n` qualify. The branded `TFunction` excluded the latter.
  t: (key: string) => string,
): string {
  if (scopedRepo !== undefined) {
    return scopedRepo
      ? resolveDisplayName(scopedRepo) || scopedRepo
      : resolveDisplayName("") || t("integrations:repository");
  }
  if (isMultiRepo) return t("integrations:allRepos");
  return resolveDisplayName("") || t("integrations:repository");
}

// eslint-disable-next-line max-lines-per-function -- Coordinates the shared commit and change-request surfaces.
function useVcsDialogsState(
  sessionId: string | null,
  taskTitle: string | undefined,
  baseBranch: string | undefined,
  pullRequestBaseBranch: string | undefined,
  pullRequestTargetsByRepository: Record<string, string> | undefined,
) {
  const { t } = useTranslation();
  const cs = useCommitDialogState();
  const ps = usePRDialogState();
  const defaultPullRequestBaseBranch = pullRequestBaseBranch ?? baseBranch;
  const effectivePullRequestBaseBranch = resolvePullRequestBaseBranch(
    ps.repo,
    pullRequestTargetsByRepository,
    defaultPullRequestBaseBranch,
  );
  const gitWithFeedback = useGitWithFeedback();
  const gitStatus = useSessionGitStatus(sessionId);
  const statusByRepo = useSessionGitStatusByRepo(sessionId);
  // Use SessionGit so commit fans out per-repo for multi-repo workspaces.
  // useGitOperations.commit hits the workspace root, which fails for multi-repo
  // tasks because the task root isn't itself a git repo (exit 1).
  const {
    commit,
    createPR: createBuiltInPR,
    push,
    repoNames,
    isLoading: isGitLoading,
  } = useSessionGit(sessionId);
  const registeredCreateTarget = useChangeRequestProviderTarget(sessionId, ps.repo);
  const createPR = useCallback(
    (input: CreateChangeRequestInput) => {
      if (!registeredCreateTarget || !sessionId) {
        return createBuiltInPR(
          input.title,
          input.body,
          input.baseBranch,
          input.draft,
          input.repositoryScope,
        );
      }
      return createChangeRequestWithProvider({
        target: registeredCreateTarget,
        push,
        repositoryScope: input.repositoryScope,
        title: input.title,
        body: input.body,
        baseBranch: input.baseBranch,
        draft: input.draft,
        branchAlreadyPushed: input.branchAlreadyPushed,
        sessionId,
        signal: input.signal,
      });
    },
    [createBuiltInPR, push, registeredCreateTarget, sessionId],
  );
  const supportsDraft = registeredCreateTarget?.provider.supportsDraft !== false;
  const repoDisplayName = useRepoDisplayName(sessionId);
  const isMultiRepo = repoNames.length > 1;
  const changeRequestTerminology = useChangeRequestTerminology(sessionId, ps.repo);
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
    const label = gitOperationLabel(t, "common:gitOpCommit", cs.repo);
    await gitWithFeedback(() => commit(fullMessage, cs.stageAll, false, cs.repo), label);
    cs.setMessage("");
    cs.setBody("");
    cs.setRepo(undefined);
  }, [cs, gitWithFeedback, commit, t]);
  const handleCreatePR = useCreateChangeRequestHandler({
    dialog: ps,
    baseBranch: effectivePullRequestBaseBranch,
    createChangeRequest: createPR,
    defaultTerminology: changeRequestTerminology,
    supportsDraft,
  });
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
    pullRequestBaseBranch: effectivePullRequestBaseBranch,
    repoDisplayName,
    isMultiRepo,
    changeRequestTerminology,
    supportsDraft,
  };
}

export function VcsDialogsProvider({
  sessionId,
  baseBranch,
  pullRequestBaseBranch,
  pullRequestTargetsByRepository,
  taskTitle,
  displayBranch,
  children,
}: VcsDialogsProviderProps) {
  const { t } = useTranslation();
  const state = useVcsDialogsState(
    sessionId,
    taskTitle,
    baseBranch,
    pullRequestBaseBranch,
    pullRequestTargetsByRepository,
  );
  const { cs, ps, isGitLoading, fileSummary, handleCommit, handleCreatePR, contextValue } = state;
  const effectiveRepoLabel = pickRepoLabel(cs.repo, state.isMultiRepo, state.repoDisplayName, t);
  const effectivePRLabel = pickRepoLabel(ps.repo, state.isMultiRepo, state.repoDisplayName, t);
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
      <VcsChangeRequestDialog
        open={ps.open}
        onOpenChange={ps.setOpen}
        scopedRepo={effectivePRLabel}
        displayBranch={displayBranch}
        baseBranch={state.pullRequestBaseBranch}
        title={ps.title}
        onTitleChange={ps.setTitle}
        body={ps.body}
        onBodyChange={ps.setBody}
        draft={ps.draft}
        onDraftChange={ps.setDraft}
        supportsDraft={state.supportsDraft}
        loading={isGitLoading}
        branchPushed={ps.branchPushed}
        onCreate={handleCreatePR}
        onGenerateTitle={() => generatePRTitle(ps.setTitle)}
        generatingTitle={isGeneratingPRTitle}
        onGenerateDescription={() => generatePRDescription(ps.setBody)}
        generatingDescription={isGeneratingPRDescription}
        utilityConfigured={isUtilityConfigured}
        terminology={state.changeRequestTerminology}
      />
    </VcsDialogsContext.Provider>
  );
}
