"use client";

import type { ReactNode } from "react";
import { IconAlertTriangle, IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { Checkbox } from "@kandev/ui/checkbox";
import { FileStatusIcon } from "@/components/shared/file-status-icon";
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
  onOpenFile?: (filePath: string, repo?: string) => void;
  markdownPreview?: boolean;
  onToggleMarkdownPreview?: () => void;
  onToggleCollapse: () => void;
  onToggleExpandUnchanged: () => void;
  onToggleWordWrap: () => void;
};

function splitFilePath(filePath: string): { directory: string; name: string } {
  const lastSlash = filePath.lastIndexOf("/");
  if (lastSlash === -1) return { directory: "", name: filePath };
  return {
    directory: filePath.slice(0, lastSlash),
    name: filePath.slice(lastSlash + 1),
  };
}

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

function ReviewFileDirectory({ directory, className }: { directory: string; className: string }) {
  return (
    <span aria-hidden="true" data-review-file-directory className={className}>
      <bdi dir="ltr">{directory}</bdi>
    </span>
  );
}

function DesktopReviewFilePath({ path }: { path: string }) {
  const { directory, name } = splitFilePath(path);
  return (
    <span
      className="flex min-w-0 flex-1 items-baseline overflow-hidden text-[13px] font-medium"
      title={path}
    >
      {directory && (
        <>
          <ReviewFileDirectory
            directory={directory}
            className="min-w-0 truncate text-muted-foreground [direction:rtl] [unicode-bidi:isolate]"
          />
          <span aria-hidden="true" className="shrink-0 text-muted-foreground">
            /
          </span>
        </>
      )}
      <span data-review-file-name className="max-w-full shrink-0 truncate">
        {name}
      </span>
    </span>
  );
}

function MobileReviewFileDetails({ file, isStale }: { file: ReviewFile; isStale: boolean }) {
  const { directory, name } = splitFilePath(file.path);
  return (
    <span
      className="flex min-w-0 flex-1 flex-col justify-center overflow-hidden text-left leading-none"
      title={file.repository_name ? `${file.repository_name}/${file.path}` : file.path}
    >
      {file.repository_name && (
        <span
          data-testid="review-file-repository"
          className="truncate text-[10px] font-medium leading-3 text-primary"
        >
          {file.repository_name}
        </span>
      )}
      <span data-review-file-name className="truncate text-[13px] font-medium leading-4">
        {name}
      </span>
      <span className="flex min-w-0 items-center gap-1 leading-4">
        {directory && (
          <ReviewFileDirectory
            directory={directory}
            className="min-w-0 flex-1 truncate text-[11px] text-muted-foreground [direction:rtl] [unicode-bidi:isolate]"
          />
        )}
        <FileStatusIcon
          status={file.status}
          oldPath={file.old_path}
          className="size-3.5 shrink-0"
        />
        {isStale && <StaleIndicator compact />}
        <ReviewDiffStats file={file} compact />
      </span>
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

type ResponsiveHeaderIdentityProps = Pick<
  ReviewDiffHeaderProps,
  "file" | "isReviewed" | "isStale" | "collapsed" | "onCheckboxChange" | "onToggleCollapse"
> & {
  toolbar: ReactNode;
};

function MobileHeaderIdentity({
  file,
  isReviewed,
  isStale,
  collapsed,
  onCheckboxChange,
  onToggleCollapse,
  toolbar,
}: ResponsiveHeaderIdentityProps) {
  const { t } = useTranslation();
  return (
    <div
      data-testid="review-file-identity"
      className="flex min-h-14 w-full min-w-0 items-center gap-1 px-2"
    >
      <MobileReviewCheckbox checked={isReviewed} onCheckedChange={onCheckboxChange} />
      <button
        type="button"
        aria-expanded={!collapsed}
        aria-label={
          collapsed
            ? t("review:expandFile", { path: file.path })
            : t("review:collapseFile", { path: file.path })
        }
        onClick={onToggleCollapse}
        className="flex min-h-11 min-w-0 flex-1 cursor-pointer items-center gap-1.5 text-left transition-colors duration-150 ease-out hover:text-foreground"
      >
        {collapsed ? (
          <IconChevronRight className="size-3.5 shrink-0 text-muted-foreground" />
        ) : (
          <IconChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
        )}
        <MobileReviewFileDetails file={file} isStale={isStale} />
      </button>
      {toolbar}
    </div>
  );
}

function DesktopHeaderIdentity({
  file,
  isReviewed,
  isStale,
  collapsed,
  onCheckboxChange,
  onToggleCollapse,
  toolbar,
}: ResponsiveHeaderIdentityProps) {
  const { t } = useTranslation();
  return (
    <>
      <div data-testid="review-file-identity" className="flex min-w-0 flex-1 items-center gap-2">
        <Checkbox
          checked={isReviewed}
          onCheckedChange={onCheckboxChange}
          className="size-4 cursor-pointer"
        />
        <button
          type="button"
          aria-expanded={!collapsed}
          aria-label={
            collapsed
              ? t("review:expandFile", { path: file.path })
              : t("review:collapseFile", { path: file.path })
          }
          onClick={onToggleCollapse}
          className="flex min-w-0 flex-1 cursor-pointer items-center gap-1.5 text-left hover:text-foreground"
        >
          {collapsed ? (
            <IconChevronRight className="size-3.5 shrink-0 text-muted-foreground" />
          ) : (
            <IconChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
          )}
          <DesktopReviewFilePath path={file.path} />
        </button>
        {isStale && <StaleIndicator />}
        <ReviewDiffStats file={file} />
      </div>
      <div data-testid="review-file-actions" className="flex items-center">
        {toolbar}
      </div>
    </>
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
  const { isMobile } = useResponsiveBreakpoint();
  const hasPublishedPR =
    file.source === "pr" && (!file.repository_id || file.repository_id === publishedPRRepositoryId);
  const publishedBranch = hasPublishedPR ? publishedPRBranch : undefined;
  const baseBranch =
    baseBranchByRepo[file.repository_name ?? ""] ??
    (file.repository_name ? undefined : fallbackBaseBranch);
  const toolbar = (
    <FileDiffToolbar
      diff={file.diff}
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
      {isMobile ? (
        <MobileHeaderIdentity
          file={file}
          isReviewed={isReviewed}
          isStale={isStale}
          collapsed={collapsed}
          onCheckboxChange={onCheckboxChange}
          onToggleCollapse={onToggleCollapse}
          toolbar={toolbar}
        />
      ) : (
        <DesktopHeaderIdentity
          file={file}
          isReviewed={isReviewed}
          isStale={isStale}
          collapsed={collapsed}
          onCheckboxChange={onCheckboxChange}
          onToggleCollapse={onToggleCollapse}
          toolbar={toolbar}
        />
      )}
    </div>
  );
}
