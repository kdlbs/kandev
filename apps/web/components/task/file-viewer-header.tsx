"use client";

import type { ReactNode } from "react";
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
