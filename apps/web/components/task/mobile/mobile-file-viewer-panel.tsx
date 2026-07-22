"use client";

import { useMemo, useState } from "react";
import { IconEye } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { PanelBody, PanelHeaderBarSplit, PanelRoot } from "../panel-primitives";
import { FileViewerContent } from "../file-viewer-content";
import { MarkdownPreviewContent } from "../markdown-preview-content";
import { FileImageViewer } from "../file-image-viewer";
import { FileBinaryViewer } from "../file-binary-viewer";
import { getFileCategory } from "@/lib/utils/file-types";
import { useAppStore } from "@/components/state-provider";
import type { OpenFileTab } from "@/lib/types/backend";
import {
  ExternalVcsFileLink,
  useExternalVcsFileStatus,
} from "@/components/editors/external-vcs-file-link";

type MobileFileViewerPanelProps = {
  file: OpenFileTab;
  sessionId: string | null;
  onClose: () => void;
};

function isMarkdownFile(path: string): boolean {
  const ext = path.split(".").pop()?.toLowerCase();
  return ext === "md" || ext === "mdx";
}

function resolveViewerKind(file: OpenFileTab): "image" | "binary" | "text" {
  if (!file.isBinary) return "text";
  return getFileCategory(file.path) === "image" ? "image" : "binary";
}

function MobileViewerBody({
  file,
  viewerKind,
  markdownFile,
  markdownPreview,
  worktreePath,
  sessionId,
  taskId,
  repositoryId,
  onToggleMarkdownPreview,
}: {
  file: OpenFileTab;
  viewerKind: "image" | "binary" | "text";
  markdownFile: boolean;
  markdownPreview: boolean;
  worktreePath?: string;
  sessionId: string | null;
  taskId: string | null;
  repositoryId?: string;
  onToggleMarkdownPreview: () => void;
}) {
  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="mobile-file-viewer-content">
      {viewerKind === "image" && (
        <FileImageViewer path={file.path} content={file.content} worktreePath={worktreePath} />
      )}
      {viewerKind === "binary" && <FileBinaryViewer path={file.path} worktreePath={worktreePath} />}
      {viewerKind === "text" &&
        (markdownFile && markdownPreview ? (
          <MarkdownPreviewContent
            path={file.path}
            content={file.content}
            worktreePath={worktreePath}
            sessionId={sessionId ?? undefined}
            taskId={taskId}
            repositoryId={repositoryId}
            repositoryName={file.repo}
            enableComments={!!sessionId}
            showExternalVcsLink={false}
            onTogglePreview={onToggleMarkdownPreview}
          />
        ) : (
          <FileViewerContent path={file.path} repo={file.repo} content={file.content} />
        ))}
    </div>
  );
}

export function MobileFileViewerPanel({ file, sessionId, onClose }: MobileFileViewerPanelProps) {
  const activeSession = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const worktreePath = activeSession?.worktree_path ?? undefined;
  const repositoryId = activeSession?.repository_id ?? undefined;
  const fileStatus = useExternalVcsFileStatus(file.path, sessionId, file.repo);
  const viewerKind = useMemo(() => resolveViewerKind(file), [file]);
  const markdownFile = isMarkdownFile(file.path);

  const [markdownPreview, setMarkdownPreview] = useState(false);
  const [lastPath, setLastPath] = useState(file.path);

  // Reset preview mode when the file changes so reopening a markdown file
  // always starts in editor view, not the previous preview state.
  // Adjust state during render per React docs recommendation.
  if (lastPath !== file.path) {
    setLastPath(file.path);
    setMarkdownPreview(false);
  }

  return (
    <PanelRoot data-testid="mobile-file-viewer-panel">
      <PanelHeaderBarSplit
        className="h-11 px-2"
        left={<span className="truncate font-mono text-xs">{file.path}</span>}
        right={
          <div className="flex items-center gap-1">
            <ExternalVcsFileLink
              filePath={file.path}
              previousPath={fileStatus?.old_path}
              status={fileStatus?.status}
              taskId={activeTaskId}
              sessionId={sessionId}
              repositoryId={file.repo ? undefined : repositoryId}
              repositoryName={file.repo}
              size="touch"
            />
            {markdownFile && !markdownPreview && (
              <Button
                variant="ghost"
                size="sm"
                className="cursor-pointer px-2"
                onClick={() => setMarkdownPreview(true)}
                data-testid="markdown-preview-toggle"
                aria-label="Open markdown preview"
              >
                <IconEye className="h-4 w-4" />
              </Button>
            )}
            <Button variant="ghost" size="sm" className="cursor-pointer px-2" onClick={onClose}>
              Close
            </Button>
          </div>
        }
      />
      <PanelBody padding={false} scroll={false} className="overflow-hidden">
        <MobileViewerBody
          file={file}
          viewerKind={viewerKind}
          markdownFile={markdownFile}
          markdownPreview={markdownPreview}
          worktreePath={worktreePath}
          sessionId={sessionId}
          taskId={activeTaskId}
          repositoryId={repositoryId}
          onToggleMarkdownPreview={() => setMarkdownPreview((current) => !current)}
        />
      </PanelBody>
    </PanelRoot>
  );
}
