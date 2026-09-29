"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Checkbox } from "@kandev/ui/checkbox";
import { CollapsibleFileHeader } from "@/components/diff/collapsible-file-header";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import type { ReviewFile } from "./types";
import { FileDiffToolbar } from "./review-diff-toolbar";
import { useTranslation } from "react-i18next";

export type ReviewExternalLinkContext = {
  baseBranchByRepo: Record<string, string>;
  fallbackBaseBranch?: string;
  taskId?: string | null;
  publishedPRBranch?: string;
  publishedPRRepositoryId?: string;
};

type ReviewDiffHeaderProps = ReviewExternalLinkContext & {
  file: ReviewFile;
  isReviewed: boolean;
  isStale: boolean;
  sessionId: string;
  collapsed: boolean;
  wordWrap: boolean;
  expandUnchanged: boolean;
  hasStickyRepoHeader?: boolean;
  onCheckboxChange: (checked: boolean | "indeterminate") => void;
  onDiscard: () => void;
  onCommentFile?: () => void;
  onOpenFile?: (filePath: string, repo?: string) => void;
  markdownPreview?: boolean;
  onToggleMarkdownPreview?: () => void;
  onToggleCollapse: () => void;
  onToggleExpandUnchanged: () => void;
  onToggleWordWrap: () => void;
};

function ReviewDiffStats({ file, compact = false }: { file: ReviewFile; compact?: boolean }) {
  return (
    <span
      className={cn(
        "shrink-0 whitespace-nowrap tabular-nums text-muted-foreground",
        compact ? "text-[11px]" : "text-xs",
      )}
    >
      {file.additions > 0 && <span className="text-emerald-500">+{file.additions}</span>}
      {file.additions > 0 && file.deletions > 0 && " / "}
      {file.deletions > 0 && <span className="text-rose-500">-{file.deletions}</span>}
    </span>
  );
}

function StaleIndicator({ compact = false }: { compact?: boolean }) {
  const { t } = useTranslation();
  return (
    <span
      className={cn(
        "flex shrink-0 items-center gap-1 text-yellow-500",
        compact ? "text-[11px]" : "text-xs",
      )}
    >
      <IconAlertTriangle className="size-3.5" />
      {t("review:staleChanged")}
    </span>
  );
}

function MobileReviewCheckbox({
  checked,
  onCheckedChange,
}: {
  checked: boolean;
  onCheckedChange: ReviewDiffHeaderProps["onCheckboxChange"];
}) {
  return (
    <span className="flex size-10 shrink-0 items-center justify-center">
      <Checkbox
        checked={checked}
        onCheckedChange={onCheckedChange}
        className="relative size-4 cursor-pointer after:absolute after:left-1/2 after:top-1/2 after:size-10 after:-translate-x-1/2 after:-translate-y-1/2 after:content-['']"
      />
    </span>
  );
}

export function ReviewDiffHeader({
  file,
  isReviewed,
  isStale,
  collapsed,
  wordWrap,
  expandUnchanged,
  hasStickyRepoHeader = false,
  sessionId,
  onCheckboxChange,
  onDiscard,
  onCommentFile,
  onOpenFile,
  markdownPreview,
  onToggleMarkdownPreview,
  onToggleCollapse,
  onToggleExpandUnchanged,
  onToggleWordWrap,
  baseBranchByRepo,
  fallbackBaseBranch,
  taskId,
  publishedPRBranch,
  publishedPRRepositoryId,
}: ReviewDiffHeaderProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const hasPublishedPR =
    file.source === "pr" && (!file.repository_id || file.repository_id === publishedPRRepositoryId);
  const publishedBranch = hasPublishedPR ? publishedPRBranch : undefined;
  const baseBranch =
    baseBranchByRepo[file.repository_name ?? ""] ??
    (file.repository_name ? undefined : fallbackBaseBranch);
  const toolbar = (
    <FileDiffToolbar
      filePath={file.path}
      sessionId={sessionId}
      source={file.source}
      previousPath={file.old_path}
      status={file.status}
      taskId={taskId}
      repositoryId={file.repository_id}
      publishedBranch={publishedBranch}
      baseBranch={baseBranch}
      wordWrap={wordWrap}
      expandUnchanged={expandUnchanged}
      onDiscard={onDiscard}
      onCommentFile={onCommentFile}
      onOpenFile={onOpenFile}
      markdownPreview={markdownPreview}
      onToggleMarkdownPreview={onToggleMarkdownPreview}
      onToggleExpandUnchanged={onToggleExpandUnchanged}
      onToggleWordWrap={onToggleWordWrap}
      repo={file.repository_name}
    />
  );

  return (
    <div
      data-testid="review-file-header"
      data-file-path={file.path}
      className={cn(
        "sticky z-10 border-b border-border/50 bg-card/95 backdrop-blur-sm",
        hasStickyRepoHeader ? "top-8" : "top-0",
        !isMobile && "flex items-center gap-2 px-4 py-2",
      )}
    >
      <CollapsibleFileHeader
        filePath={file.path}
        repositoryName={file.repository_name}
        status={file.status}
        oldPath={file.old_path}
        collapsed={collapsed}
        expandLabel={t("review:expandFile", { path: file.path })}
        collapseLabel={t("review:collapseFile", { path: file.path })}
        onToggleCollapse={onToggleCollapse}
        mobileLeading={
          <MobileReviewCheckbox checked={isReviewed} onCheckedChange={onCheckboxChange} />
        }
        desktopLeading={
          <Checkbox
            checked={isReviewed}
            onCheckedChange={onCheckboxChange}
            className="size-4 cursor-pointer"
          />
        }
        mobileMetadata={isStale ? <StaleIndicator compact /> : undefined}
        desktopMetadata={isStale ? <StaleIndicator /> : undefined}
        mobileStats={<ReviewDiffStats file={file} compact />}
        desktopStats={<ReviewDiffStats file={file} />}
        actions={toolbar}
        actionsTestId="review-file-actions"
        identityTestId="review-file-identity"
      />
    </div>
  );
}
