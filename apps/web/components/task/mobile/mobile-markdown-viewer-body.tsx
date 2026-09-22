"use client";

import {
  HybridMarkdownEditor,
  type MarkdownComment,
  type MarkdownCommentSubmission,
} from "@/components/editors/markdown/hybrid-markdown-editor";
import type { OpenFileTab } from "@/lib/types/backend";
import { isMarkdownFile } from "@/lib/utils/file-types";
import { FileBinaryViewer } from "../file-binary-viewer";
import { FileImageViewer } from "../file-image-viewer";
import { FileViewerContent } from "../file-viewer-content";
import { HtmlPreviewContent } from "../html-preview-content";
import { MarkdownPreviewContent } from "../markdown-preview-content";
import type { MarkdownFileMode } from "../markdown-file-mode";
import type { FilePreviewKind } from "@/lib/utils/file-types";
import type { HtmlPreviewPublishState } from "@/hooks/use-html-preview-publisher";

export type MobileViewerKind = "image" | "binary" | "text";

// eslint-disable-next-line max-lines-per-function -- keeps the mobile file surface dispatch together.
export function MobileViewerBody({
  file,
  viewerKind,
  previewKind,
  renderedPreview,
  markdownMode,
  keepHybridMounted,
  keepPreviewMounted,
  worktreePath,
  sessionId,
  taskId,
  repositoryId,
  draftContent,
  comments,
  onChange,
  onComment,
  onSourceFallback,
  onTogglePreview,
  onRetryHtmlPreview,
  htmlPreview,
  onOpenFile,
  onOpenLink,
}: {
  file: OpenFileTab;
  viewerKind: MobileViewerKind;
  previewKind: FilePreviewKind;
  renderedPreview: boolean;
  markdownMode?: MarkdownFileMode;
  keepHybridMounted: boolean;
  keepPreviewMounted: boolean;
  worktreePath?: string;
  sessionId: string | null;
  taskId: string | null;
  repositoryId?: string;
  draftContent: string;
  comments: readonly MarkdownComment[];
  onChange: (content: string) => void;
  onComment: (comment: MarkdownCommentSubmission) => void;
  onSourceFallback?: () => void;
  onTogglePreview: () => void;
  onRetryHtmlPreview: () => void;
  htmlPreview: Pick<HtmlPreviewPublishState, "status" | "url" | "error"> & {
    isPublishing: boolean;
  };
  onOpenFile?: (path: string) => void;
  onOpenLink?: (url: string) => boolean | void;
}) {
  const markdownFile = isMarkdownFile(file.path);
  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="mobile-file-viewer-content">
      {viewerKind === "image" && (
        <FileImageViewer path={file.path} content={draftContent} worktreePath={worktreePath} />
      )}
      {viewerKind === "binary" && <FileBinaryViewer path={file.path} worktreePath={worktreePath} />}
      {viewerKind === "text" && markdownFile && (
        <MobileMarkdownSurface
          file={file}
          markdownMode={markdownMode}
          keepHybridMounted={keepHybridMounted}
          keepPreviewMounted={keepPreviewMounted}
          worktreePath={worktreePath}
          sessionId={sessionId}
          taskId={taskId}
          repositoryId={repositoryId}
          draftContent={draftContent}
          comments={comments}
          onChange={onChange}
          onComment={onComment}
          onSourceFallback={onSourceFallback}
          onOpenFile={onOpenFile}
          onOpenLink={onOpenLink}
        />
      )}
      {viewerKind === "text" && !markdownFile && renderedPreview && previewKind === "html" && (
        <HtmlPreviewContent
          path={file.path}
          worktreePath={worktreePath}
          sessionId={sessionId ?? undefined}
          taskId={taskId}
          repositoryId={repositoryId}
          repositoryName={file.repo}
          showExternalVcsLink={false}
          previewUrl={htmlPreview.url}
          isLoading={htmlPreview.isPublishing}
          error={htmlPreview.error}
          onRetry={onRetryHtmlPreview}
          onTogglePreview={onTogglePreview}
        />
      )}
      {viewerKind === "text" && !markdownFile && !(renderedPreview && previewKind === "html") && (
        <FileViewerContent
          path={file.path}
          repo={file.repo}
          content={draftContent}
          sessionId={sessionId ?? undefined}
          editable={false}
        />
      )}
    </div>
  );
}

function MobileMarkdownSurface({
  file,
  markdownMode,
  keepHybridMounted,
  keepPreviewMounted,
  worktreePath,
  sessionId,
  taskId,
  repositoryId,
  draftContent,
  comments,
  onChange,
  onComment,
  onSourceFallback,
  onOpenFile,
  onOpenLink,
}: {
  file: OpenFileTab;
  markdownMode?: MarkdownFileMode;
  keepHybridMounted: boolean;
  keepPreviewMounted: boolean;
  worktreePath?: string;
  sessionId: string | null;
  taskId: string | null;
  repositoryId?: string;
  draftContent: string;
  comments: readonly MarkdownComment[];
  onChange: (content: string) => void;
  onComment: (comment: MarkdownCommentSubmission) => void;
  onSourceFallback?: () => void;
  onOpenFile?: (path: string) => void;
  onOpenLink?: (url: string) => boolean | void;
}) {
  return (
    <>
      {keepPreviewMounted && (
        <div
          className={markdownMode === "preview" ? "h-full min-h-0" : "hidden"}
          aria-hidden={markdownMode !== "preview"}
          data-testid="mobile-markdown-preview-host"
        >
          <MarkdownPreviewContent
            path={file.path}
            content={draftContent}
            worktreePath={worktreePath}
            sessionId={sessionId ?? undefined}
            taskId={taskId}
            repositoryId={repositoryId}
            repositoryName={file.repo}
            enableComments={!!sessionId}
            showExternalVcsLink={false}
            onTogglePreview={undefined}
            onOpenFile={onOpenFile}
            onOpenLink={onOpenLink}
          />
        </div>
      )}
      {keepHybridMounted && (
        <div
          className={markdownMode === "edit" ? "min-h-0 flex-1 overflow-hidden" : "hidden"}
          aria-hidden={markdownMode !== "edit"}
          data-testid="mobile-markdown-hybrid-editor-host"
        >
          <HybridMarkdownEditor
            content={draftContent}
            readOnly={false}
            comments={comments}
            onChange={onChange}
            onOpenLink={onOpenLink}
            onComment={onComment}
            onSourceFallback={onSourceFallback}
          />
        </div>
      )}
      {markdownMode === "source" && (
        <FileViewerContent
          path={file.path}
          repo={file.repo}
          content={draftContent}
          sessionId={sessionId ?? undefined}
          editable
          onChange={onChange}
        />
      )}
    </>
  );
}
