"use client";

import { SymlinkIndicator } from "@/components/shared/symlink-indicator";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconDownload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { toRelativePath } from "@/lib/utils";
import {
  ExternalVcsFileLink,
  useExternalVcsFileStatus,
} from "@/components/editors/external-vcs-file-link";
import { PanelHeaderBarSplit } from "@/components/task/panel-primitives";

type FileViewerHeaderProps = {
  path: string;
  isSymlink?: boolean;
  worktreePath?: string;
  actions?: ReactNode;
};

export function FileViewerHeader({
  path,
  isSymlink,
  worktreePath,
  actions,
}: FileViewerHeaderProps) {
  const label = toRelativePath(path, worktreePath);
  return (
    <PanelHeaderBarSplit
      left={
        <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
          <span className="truncate font-mono">{label}</span>
          <SymlinkIndicator isSymlink={isSymlink} showLabel />
        </div>
      }
      right={actions}
    />
  );
}

type FileViewerExternalLinkProps = {
  path: string;
  sessionId?: string | null;
  taskId?: string | null;
  repositoryId?: string | null;
  repositoryName?: string;
};

export function FileViewerExternalLink({
  path,
  sessionId,
  taskId,
  repositoryId,
  repositoryName,
}: FileViewerExternalLinkProps) {
  const fileStatus = useExternalVcsFileStatus(path, sessionId, repositoryName);
  return (
    <ExternalVcsFileLink
      filePath={path}
      previousPath={fileStatus?.old_path}
      status={fileStatus?.status}
      taskId={taskId}
      sessionId={sessionId}
      repositoryId={repositoryName ? undefined : repositoryId}
      repositoryName={repositoryName}
      size="sm"
    />
  );
}

type FileViewerDownloadButtonProps = {
  onDownload?: () => void;
};

/**
 * Download control for the viewer header. Shared by the binary and image
 * viewers so both screens, and the mobile viewer that renders the same
 * `headerActions`, gain the action from one place.
 */
export function FileViewerDownloadButton({ onDownload }: FileViewerDownloadButtonProps) {
  const { t } = useTranslation();
  if (!onDownload) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          onClick={onDownload}
          aria-label={t("editors:downloadFile")}
          className="h-6 w-6 p-0 cursor-pointer max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:w-11 [@media(pointer:coarse)]:w-11"
        >
          <IconDownload className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("editors:downloadFile")}</TooltipContent>
    </Tooltip>
  );
}
