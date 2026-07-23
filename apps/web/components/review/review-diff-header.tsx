"use client";

import { IconAlertTriangle, IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { Checkbox } from "@kandev/ui/checkbox";
import { FileStatusIcon } from "@/components/shared/file-status-icon";
import type { ReviewFile } from "./types";
import { FileDiffToolbar } from "./review-diff-toolbar";

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
  onCheckboxChange: (checked: boolean | "indeterminate") => void;
  onDiscard: () => void;
  onOpenFile?: (filePath: string, repo?: string) => void;
  onPreviewMarkdown?: (filePath: string) => void;
  onToggleCollapse: () => void;
  onToggleExpandUnchanged: () => void;
  onToggleWordWrap: () => void;
};

export function ReviewDiffHeader({
  file,
  isReviewed,
  isStale,
  collapsed,
  wordWrap,
  expandUnchanged,
  sessionId,
  onCheckboxChange,
  onDiscard,
  onOpenFile,
  onPreviewMarkdown,
  onToggleCollapse,
  onToggleExpandUnchanged,
  onToggleWordWrap,
  baseBranchByRepo,
  fallbackBaseBranch,
  taskId,
  publishedPRBranch,
  publishedPRRepositoryId,
}: ReviewDiffHeaderProps) {
  const hasPublishedPR =
    file.source === "pr" && (!file.repository_id || file.repository_id === publishedPRRepositoryId);
  const publishedBranch = hasPublishedPR ? publishedPRBranch : undefined;
  const baseBranch =
    baseBranchByRepo[file.repository_name ?? ""] ??
    (file.repository_name ? undefined : fallbackBaseBranch);

  return (
    <div
      data-testid="review-file-header"
      data-file-path={file.path}
      className="sticky top-0 z-10 flex items-center gap-2 px-4 py-2 bg-card/95 backdrop-blur-sm border-b border-border/50"
    >
      <Checkbox
        checked={isReviewed}
        onCheckedChange={onCheckboxChange}
        className="h-4 w-4 cursor-pointer"
      />
      <button
        onClick={onToggleCollapse}
        className="flex items-center gap-1.5 flex-1 min-w-0 cursor-pointer text-left hover:text-foreground"
      >
        {collapsed ? (
          <IconChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        ) : (
          <IconChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        )}
        <span className="text-[13px] font-medium truncate">{file.path}</span>
      </button>
      <FileStatusIcon status={file.status} oldPath={file.old_path} className="sm:hidden" />
      {isStale && (
        <span className="flex items-center gap-1 text-xs text-yellow-500">
          <IconAlertTriangle className="h-3.5 w-3.5" />
          changed
        </span>
      )}
      <span className="text-xs text-muted-foreground">
        {file.additions > 0 && <span className="text-emerald-500">+{file.additions}</span>}
        {file.additions > 0 && file.deletions > 0 && " / "}
        {file.deletions > 0 && <span className="text-rose-500">-{file.deletions}</span>}
      </span>
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
        onPreviewMarkdown={onPreviewMarkdown}
        onToggleExpandUnchanged={onToggleExpandUnchanged}
        onToggleWordWrap={onToggleWordWrap}
        repo={file.repository_name}
      />
    </div>
  );
}
