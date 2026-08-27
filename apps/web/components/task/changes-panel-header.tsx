"use client";

import { useState } from "react";
import {
  IconCloudDownload,
  IconEye,
  IconChevronDown,
  IconGitBranch,
  IconGitMerge,
  IconArrowRight,
  IconLoader2,
  IconEdit,
  IconRoute,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { PanelHeaderBarSplit } from "./panel-primitives";
import { BaseBranchPicker } from "./base-branch-picker";
import type { GitCredentialDisplay } from "./changes-git-credential-display";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useTranslation } from "react-i18next";
import { PerRepoPullMenu } from "./changes-panel-per-repo-menu";
import type { RemoteContributionRelation } from "@/hooks/domains/session/remote-contribution-relation";
import {
  type RemoteContributionResolutionTarget,
  type useRemoteContributionResolution,
} from "./use-remote-contribution-resolution";
import { RemoteContributionHeaderActions } from "./remote-contribution-header-actions";
import { ComparisonTargetDisplay } from "./changes-panel-comparison-target";

export type PerRepoStatus = {
  repository_name: string;
  branch: string | null;
  ahead: number;
  behind: number;
  pullBehind?: number;
  hasStaged: boolean;
  hasUnstaged: boolean;
};

type BranchRow = {
  repoLabel: string | null;
  branch: string;
  baseBranch: string;
  repositoryName: string;
};

type RenameBranchResult = {
  success: boolean;
  error?: string;
};

function buildBranchRows(
  perRepoStatus: PerRepoStatus[],
  baseBranchByRepo: Record<string, string> | undefined,
  baseBranchFallback: string,
  repoDisplayName: ((name: string) => string | undefined) | undefined,
): BranchRow[] {
  const named = perRepoStatus.filter((s) => s.repository_name !== "" && s.branch);
  if (named.length <= 1) return [];
  return named.map((s) => ({
    repoLabel: repoDisplayName?.(s.repository_name) || s.repository_name,
    branch: s.branch ?? "",
    baseBranch: baseBranchByRepo?.[s.repository_name] || baseBranchFallback,
    repositoryName: s.repository_name,
  }));
}

function RenameBranchButton({
  branch,
  repositoryName,
  onRenameBranch,
  isRenaming,
}: {
  branch: string;
  repositoryName: string;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  isRenaming: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [newBranchName, setNewBranchName] = useState(branch);
  const [error, setError] = useState<string | null>(null);
  const trimmedBranchName = newBranchName.trim();
  const canRename = !!onRenameBranch && trimmedBranchName !== "" && trimmedBranchName !== branch;
  const submitRename = async () => {
    if (!onRenameBranch || !canRename) return;
    setError(null);
    try {
      const result = await onRenameBranch(trimmedBranchName, repositoryName);
      if (!result.success) {
        setError(result.error || t("task:failedToRenameBranch"));
        return;
      }
      setOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("task:failedToRenameBranch"));
    }
  };
  const openDialog = () => {
    setNewBranchName(branch);
    setError(null);
    setOpen(true);
  };
  return (
    <>
      <Button
        type="button"
        size="icon"
        variant="ghost"
        className="h-6 w-6 shrink-0 text-muted-foreground hover:text-foreground"
        disabled={!onRenameBranch || isRenaming}
        aria-label={t("task:editBranch", { branch })}
        onClick={openDialog}
      >
        <IconEdit className="h-3.5 w-3.5" />
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("task:editBranch2")}</DialogTitle>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor={`branch-name-${repositoryName || "default"}`}>
              {t("task:branchName")}
            </Label>
            <Input
              id={`branch-name-${repositoryName || "default"}`}
              value={newBranchName}
              onChange={(event) => setNewBranchName(event.target.value)}
              autoFocus
            />
            {error && <p className="text-xs text-destructive">{error}</p>}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              {t("common:cancel")}
            </Button>
            <Button
              type="button"
              data-dialog-default-action
              disabled={!canRename || isRenaming}
              onClick={submitRename}
            >
              {t("common:save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function BranchRowView({
  repoLabel,
  branch,
  baseBranch,
  repositoryName,
  taskId,
  onRenameBranch,
  isRenaming,
}: BranchRow & {
  taskId: string | null;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  isRenaming: boolean;
}) {
  return (
    <div className="flex items-center gap-2">
      {repoLabel && (
        <span className="shrink-0 rounded-sm bg-muted/60 px-1 py-px text-[10px] font-medium text-muted-foreground max-w-[8rem] truncate">
          {repoLabel}
        </span>
      )}
      <span className="flex min-w-0 items-center gap-1.5 text-foreground font-medium">
        <IconGitBranch className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">{branch}</span>
      </span>
      <RenameBranchButton
        branch={branch}
        repositoryName={repositoryName}
        onRenameBranch={onRenameBranch}
        isRenaming={isRenaming}
      />
      <div className="flex min-w-8 flex-1 items-center text-muted-foreground/40">
        <div className="h-px flex-1 bg-muted-foreground/20" />
        <IconArrowRight className="-ml-px h-3 w-3 shrink-0" />
      </div>
      <BaseBranchPicker
        taskId={taskId}
        repositoryName={repositoryName}
        fallbackBaseBranch={baseBranch}
      />
    </div>
  );
}

function BranchHoverCard({
  displayBranch,
  baseBranchDisplay,
  rows,
  taskId,
  onRenameBranch,
  isRenaming,
  credentialDisplay,
  comparisonTargets,
}: {
  displayBranch: string;
  baseBranchDisplay: string;
  rows?: BranchRow[];
  taskId: string | null;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  isRenaming: boolean;
  credentialDisplay: GitCredentialDisplay | null;
  comparisonTargets: string[];
}) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const trigger = (
    <button
      type="button"
      className="flex items-center justify-center size-5 rounded hover:bg-muted/60 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
      aria-label={t("task:showBranchAndGitCredentialDetails")}
    >
      <IconGitBranch className="h-3.5 w-3.5" />
    </button>
  );
  const content = (
    <div className="flex flex-col gap-2.5 text-xs">
      <div className="flex items-center justify-between gap-6">
        <span className="text-muted-foreground/60">
          {rows && rows.length > 0 ? t("task:yourBranchesLabel") : t("task:yourCodeLivesInLabel")}
        </span>
        <span className="text-muted-foreground/60">{t("task:comparingAgainstLabel")}</span>
      </div>
      {rows && rows.length > 0 ? (
        <div className="flex flex-col gap-1.5">
          {rows!.map((row) => (
            <BranchRowView
              key={row.repoLabel ?? row.branch}
              {...row}
              taskId={taskId}
              onRenameBranch={onRenameBranch}
              isRenaming={isRenaming}
            />
          ))}
        </div>
      ) : (
        <BranchRowView
          repoLabel={null}
          branch={displayBranch}
          baseBranch={baseBranchDisplay}
          repositoryName=""
          taskId={taskId}
          onRenameBranch={onRenameBranch}
          isRenaming={isRenaming}
        />
      )}
      {credentialDisplay && (
        <div className="border-t pt-2 text-muted-foreground" data-testid="changes-git-credential">
          <p className="font-medium text-foreground">{credentialDisplay.source}</p>
          <p>{credentialDisplay.detail}</p>
          <p>{credentialDisplay.transport}</p>
        </div>
      )}
      <ComparisonTargetDisplay targets={comparisonTargets} />
    </div>
  );
  if (usesTouchDrawer) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle>{t("task:branchDetails")}</DrawerTitle>
            <DrawerDescription>{t("task:branchComparisonAndTaskGitCredentials")}</DrawerDescription>
          </DrawerHeader>
          <div className="px-4 pb-6">{content}</div>
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <HoverCard openDelay={200} closeDelay={100}>
      <HoverCardTrigger asChild>{trigger}</HoverCardTrigger>
      <HoverCardContent forceMount side="bottom" align="end" className="w-auto p-3">
        {content}
      </HoverCardContent>
    </HoverCard>
  );
}

function PullTriggerContent({
  behindCount,
  isPulling,
  isRebasing,
}: {
  behindCount: number;
  isPulling: boolean;
  isRebasing: boolean;
}) {
  let label: string;
  if (isPulling) label = "Pulling…";
  else if (isRebasing) label = "Rebasing…";
  else label = "Pull";
  return (
    <>
      {isPulling || isRebasing ? (
        <IconLoader2 className="h-3 w-3 animate-spin" />
      ) : (
        <IconCloudDownload className="h-3 w-3" />
      )}
      {label}
      {behindCount > 0 && !(isPulling || isRebasing) && (
        <span className="text-yellow-500 text-[10px]">{behindCount}</span>
      )}
      {!(isPulling || isRebasing) && (
        <IconChevronDown className="h-2.5 w-2.5 text-muted-foreground" />
      )}
    </>
  );
}

export function PullDropdown({
  behindCount,
  pullDisabled,
  isLoading,
  loadingOperation,
  repoNames,
  perRepoStatus,
  onRepoPull,
  onRepoRebase,
  onRepoMerge,
  repoDisplayName,
  pullDisabledReason,
}: {
  behindCount: number;
  pullDisabled?: boolean;
  pullDisabledReason?: string;
  isLoading: boolean;
  loadingOperation: string | null;
  /** Always non-empty (single-repo includes the empty-name entry). */
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  onRepoPull: (repo: string) => void;
  onRepoRebase: (repo: string) => void;
  onRepoMerge: (repo: string) => void;
  /** Maps a repository_name to its display label. */
  repoDisplayName?: (repositoryName: string) => string | undefined;
}) {
  const { t } = useTranslation();
  const triggerBehind =
    perRepoStatus.length > 0
      ? Math.max(behindCount, ...perRepoStatus.map((s) => s.pullBehind ?? 0))
      : behindCount;
  const pullButton = (
    <Button
      size="sm"
      variant="ghost"
      className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
      disabled={isLoading || pullDisabled}
    >
      <PullTriggerContent
        behindCount={triggerBehind}
        isPulling={loadingOperation === "pull"}
        isRebasing={loadingOperation === "rebase"}
      />
    </Button>
  );
  if (pullDisabled) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span tabIndex={0} className="inline-flex">
            {pullButton}
          </span>
        </TooltipTrigger>
        <TooltipContent>
          {pullDisabledReason ?? t("task:divergedActionsUnavailable")}
        </TooltipContent>
      </Tooltip>
    );
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{pullButton}</DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <PerRepoPullMenu
          repoNames={repoNames}
          perRepoStatus={perRepoStatus}
          onRepoPull={onRepoPull}
          onRepoRebase={onRepoRebase}
          onRepoMerge={onRepoMerge}
          pullDisabled={pullDisabled}
          pullDisabledReason={pullDisabledReason}
          repoDisplayName={repoDisplayName}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ChangesPanelHeaderLeft({
  showDiffReview,
  onOpenDiffAll,
  onOpenReview,
  onRequestWalkthrough,
  requestWalkthroughDisabled,
}: {
  showDiffReview: boolean;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
  onRequestWalkthrough?: () => void;
  requestWalkthroughDisabled?: boolean;
}) {
  const { t } = useTranslation();
  if (!showDiffReview) return null;
  return (
    <>
      <Button
        size="sm"
        variant="ghost"
        className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
        onClick={onOpenDiffAll}
      >
        <IconGitMerge className="h-3 w-3" />
        {t("task:diff")}
      </Button>
      <Button
        size="sm"
        variant="ghost"
        className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
        onClick={onOpenReview}
      >
        <IconEye className="h-3 w-3" />
        {t("task:filterStateReview")}
      </Button>
      {onRequestWalkthrough ? (
        <ChangesPanelWalkthroughButton
          onRequestWalkthrough={onRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      ) : null}
    </>
  );
}

function ChangesPanelWalkthroughButton({
  onRequestWalkthrough,
  requestWalkthroughDisabled,
}: {
  onRequestWalkthrough: () => void;
  requestWalkthroughDisabled?: boolean;
}) {
  const { t } = useTranslation();
  const tooltip = requestWalkthroughDisabled
    ? t("task:loadingChangedFiles")
    : t("task:walkMeThroughTheseChanges");
  return (
    <Tooltip>
      <TooltipTrigger asChild className="order-first @[350px]/changes-panel:order-none">
        <span className="inline-flex" tabIndex={requestWalkthroughDisabled ? 0 : undefined}>
          <Button
            size="sm"
            variant="ghost"
            className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
            aria-label={t("task:walkMeThroughTheseChanges")}
            data-testid="changes-request-walkthrough"
            disabled={requestWalkthroughDisabled}
            onClick={onRequestWalkthrough}
          >
            <IconRoute className="h-3 w-3" />
            <span className="hidden @[350px]/changes-panel:inline">{t("task:walkthrough2")}</span>
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

type ChangesPanelHeaderProps = {
  hasChanges: boolean;
  hasCommits: boolean;
  hasPRFiles?: boolean;
  displayBranch: string | null;
  baseBranchDisplay: string;
  /** Per-repo merge target, keyed by repository_name. Undefined entries fall
   *  back to baseBranchDisplay. Empty/missing for single-repo workspaces. */
  baseBranchByRepo?: Record<string, string>;
  behindCount: number;
  pullDisabled?: boolean;
  pullDisabledReason?: string;
  isLoading: boolean;
  loadingOperation: string | null;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
  onRequestWalkthrough?: () => void;
  requestWalkthroughDisabled?: boolean;
  /** Always non-empty (single-repo includes the empty-name entry). */
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  onRepoPull: (repo: string) => void;
  onRepoRebase: (repo: string) => void;
  onRepoMerge: (repo: string) => void;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  credentialDisplay: GitCredentialDisplay | null;
  comparisonTargets: string[];
  repoDisplayName?: (repositoryName: string) => string | undefined;
  /** Active task id; piped into the base-branch picker so it can resolve
   *  the right task_repositories row to PATCH. Null while task data is
   *  hydrating — the picker falls back to a static label. */
  taskId: string | null;
  relation?: RemoteContributionRelation;
  resolution?: ReturnType<typeof useRemoteContributionResolution>;
  resolutionTarget?: RemoteContributionResolutionTarget | null;
  remoteContributionUrl?: string;
  remoteContributionNumber?: number;
};

export function ChangesPanelHeader(props: ChangesPanelHeaderProps) {
  const {
    hasChanges,
    hasCommits,
    hasPRFiles,
    displayBranch,
    baseBranchDisplay,
    baseBranchByRepo,
    behindCount,
    pullDisabled,
    pullDisabledReason,
    isLoading,
    loadingOperation,
    onOpenDiffAll,
    onOpenReview,
    onRequestWalkthrough,
    requestWalkthroughDisabled,
    repoNames,
    perRepoStatus,
    onRepoPull,
    onRepoRebase,
    onRepoMerge,
    repoDisplayName,
    taskId,
    onRenameBranch,
    credentialDisplay,
    comparisonTargets,
    relation,
    resolution,
    resolutionTarget,
    remoteContributionUrl,
    remoteContributionNumber,
  } = props;
  const branchRows = buildBranchRows(
    perRepoStatus,
    baseBranchByRepo,
    baseBranchDisplay,
    repoDisplayName,
  );
  return (
    <PanelHeaderBarSplit
      left={
        <ChangesPanelHeaderLeft
          showDiffReview={hasChanges || hasCommits || !!hasPRFiles}
          onOpenDiffAll={onOpenDiffAll}
          onOpenReview={onOpenReview}
          onRequestWalkthrough={onRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      }
      right={
        <>
          {(displayBranch || branchRows.length > 0) && (
            <BranchHoverCard
              displayBranch={displayBranch ?? ""}
              baseBranchDisplay={baseBranchDisplay}
              rows={branchRows}
              taskId={taskId}
              onRenameBranch={onRenameBranch}
              isRenaming={loadingOperation === "rename_branch"}
              credentialDisplay={credentialDisplay}
              comparisonTargets={comparisonTargets}
            />
          )}
          <RemoteContributionHeaderActions
            relation={relation}
            resolution={resolution}
            resolutionTarget={resolutionTarget}
            prUrl={remoteContributionUrl}
            prNumber={remoteContributionNumber}
          />
          <PullDropdown
            behindCount={behindCount}
            pullDisabled={pullDisabled}
            pullDisabledReason={pullDisabledReason}
            isLoading={isLoading}
            loadingOperation={loadingOperation}
            repoNames={repoNames}
            perRepoStatus={perRepoStatus}
            onRepoPull={onRepoPull}
            onRepoRebase={onRepoRebase}
            onRepoMerge={onRepoMerge}
            repoDisplayName={repoDisplayName}
          />
        </>
      }
    />
  );
}
