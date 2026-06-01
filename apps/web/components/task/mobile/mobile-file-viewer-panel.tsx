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
import { useTaskSessionById } from "@/hooks/domains/session/use-task-session-by-id";
import type { OpenFileTab } from "@/lib/types/backend";

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

export function MobileFileViewerPanel({ file, sessionId, onClose }: MobileFileViewerPanelProps) {
  const activeSession = useTaskSessionById(sessionId);
  const worktreePath = activeSession?.worktree_path ?? undefined;
  const viewerKind = useMemo(() => resolveViewerKind(file), [file]);
  const markdownFile = isMarkdownFile(file.path);

  const [markdownPreview, setMarkdownPreview] = useState(false);
  const [lastPath, setLastPath] = useState(file.path);
  const content = file.content;

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
        left={<span className="truncate font-mono text-xs">{file.path}</span>}
        right={
          <div className="flex items-center gap-1">
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
        <div className="flex h-full min-h-0 flex-col" data-testid="mobile-file-viewer-content">
          {viewerKind === "image" && (
            <FileImageViewer path={file.path} content={file.content} worktreePath={worktreePath} />
          )}
          {viewerKind === "binary" && (
            <FileBinaryViewer path={file.path} worktreePath={worktreePath} />
          )}
          {viewerKind === "text" &&
            (markdownFile && markdownPreview ? (
              <MarkdownPreviewContent
                path={file.path}
                content={content}
                worktreePath={worktreePath}
                onTogglePreview={() => setMarkdownPreview((current) => !current)}
              />
            ) : (
              <FileViewerContent path={file.path} content={content} />
            ))}
        </div>
      </PanelBody>
    </PanelRoot>
  );
}
