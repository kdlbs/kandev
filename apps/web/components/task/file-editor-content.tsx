"use client";

import { memo, useCallback, useEffect, useState, type ReactNode } from "react";
import { useEditorProvider } from "@/hooks/use-editor-resolver";
import { useHtmlPreviewPublisher } from "@/hooks/use-html-preview-publisher";
import { MonacoCodeEditor } from "@/components/editors/monaco/monaco-code-editor";
import { CodeMirrorCodeEditor } from "@/components/editors/codemirror/codemirror-code-editor";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { FilePreviewKind } from "@/lib/utils/file-types";
import { HtmlPreviewContent } from "./html-preview-content";
import { MarkdownPreviewContent } from "./markdown-preview-content";

export type FileEditorContentProps = {
  path: string;
  isSymlink?: boolean;
  content: string;
  originalContent: string;
  isDirty: boolean;
  hasRemoteUpdate?: boolean;
  vcsDiff?: string;
  isSaving: boolean;
  sessionId?: string;
  taskId?: string | null;
  repositoryId?: string | null;
  worktreePath?: string;
  repo?: string;
  enableComments?: boolean;
  previewKind?: FilePreviewKind;
  renderedPreview?: boolean;
  onTogglePreview?: () => void;
  markdownPreview?: boolean;
  onToggleMarkdownPreview?: () => void;
  toolbarModeControl?: ReactNode;
  onChange: (newContent: string) => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
  onDownload?: () => void;
};

// eslint-disable-next-line complexity -- selects the compatible preview and editor surface.
export const FileEditorContent = memo(function FileEditorContent(props: FileEditorContentProps) {
  const provider = useEditorProvider("code-editor");
  const [htmlPreviewIdentity, setHtmlPreviewIdentity] = useState<string | null>(null);
  const htmlPreview = useHtmlPreviewPublisher(props.sessionId ?? null);
  const openBrowserPanel = useDockviewStore((state) => state.openBrowserPanel);
  const fileIdentity = `${props.sessionId ?? ""}\u0000${props.repo ?? ""}\u0000${props.path}`;

  useEffect(() => {
    setHtmlPreviewIdentity(null);
    htmlPreview.reset();
  }, [fileIdentity, htmlPreview.reset]);

  const publishCurrentHtmlPreview = useCallback(() => {
    setHtmlPreviewIdentity(fileIdentity);
    void htmlPreview.publish({
      path: props.path,
      content: props.content,
      ...(props.repo ? { repo: props.repo } : {}),
    });
  }, [fileIdentity, htmlPreview.publish, props.content, props.path, props.repo]);

  const showHtmlSource = useCallback(() => {
    setHtmlPreviewIdentity(null);
    htmlPreview.reset();
  }, [htmlPreview.reset]);

  const openHtmlPreviewInBrowserPanel = useCallback(() => {
    if (htmlPreview.url) openBrowserPanel(htmlPreview.url);
  }, [htmlPreview.url, openBrowserPanel]);

  if (
    (props.renderedPreview && props.previewKind === "markdown" && props.onTogglePreview) ||
    (props.markdownPreview && props.onToggleMarkdownPreview)
  ) {
    return (
      <MarkdownPreviewContent
        path={props.path}
        isSymlink={props.isSymlink}
        content={props.content}
        worktreePath={props.worktreePath}
        sessionId={props.sessionId}
        taskId={props.taskId}
        repositoryId={props.repositoryId}
        repositoryName={props.repo}
        enableComments={props.enableComments}
        onDownload={props.onDownload}
        onTogglePreview={props.onTogglePreview ?? props.onToggleMarkdownPreview}
        toolbarModeControl={props.toolbarModeControl}
      />
    );
  }

  if (htmlPreviewIdentity === fileIdentity && props.previewKind === "html") {
    return (
      <HtmlPreviewContent
        path={props.path}
        isSymlink={props.isSymlink}
        previewUrl={htmlPreview.url}
        isLoading={htmlPreview.isPublishing}
        error={htmlPreview.error}
        worktreePath={props.worktreePath}
        sessionId={props.sessionId}
        taskId={props.taskId}
        repositoryId={props.repositoryId}
        repositoryName={props.repo}
        onDownload={props.onDownload}
        onRefresh={publishCurrentHtmlPreview}
        onOpenInBrowser={htmlPreview.url ? openHtmlPreviewInBrowserPanel : undefined}
        onRetry={publishCurrentHtmlPreview}
        onTogglePreview={showHtmlSource}
      />
    );
  }

  return provider === "monaco" ? (
    <MonacoCodeEditor
      {...props}
      onPreviewHtml={props.previewKind === "html" ? publishCurrentHtmlPreview : undefined}
      isPublishingHtmlPreview={htmlPreview.isPublishing}
    />
  ) : (
    <CodeMirrorCodeEditor
      {...props}
      onPreviewHtml={props.previewKind === "html" ? publishCurrentHtmlPreview : undefined}
      isPublishingHtmlPreview={htmlPreview.isPublishing}
    />
  );
});
