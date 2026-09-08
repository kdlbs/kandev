"use client";

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

type FileViewerHeaderProps = {
  path: string;
  worktreePath?: string;
  actions?: ReactNode;
};

export function FileViewerHeader({ path, worktreePath, actions }: FileViewerHeaderProps) {
  const label = toRelativePath(path, worktreePath);
  if (!actions) {
    return (
      <div className="flex items-center px-2 border-foreground/10 border-b">
        <div className="flex items-center gap-2 text-xs text-muted-foreground py-2">
          <span className="font-mono">{label}</span>
        </div>
      </div>
    );
  }
  return (
    <div className="flex items-center px-2 border-foreground/10 border-b">
      <div className="flex min-w-0 flex-1 items-center gap-2 py-2 text-xs text-muted-foreground">
        <span className="truncate font-mono">{label}</span>
      </div>
      <div className="ml-auto flex shrink-0 items-center gap-1">{actions}</div>
    </div>
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
          className="h-11 w-11 p-0 cursor-pointer sm:h-8 sm:w-8"
        >
          <IconDownload className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("editors:downloadFile")}</TooltipContent>
    </Tooltip>
  );
}
