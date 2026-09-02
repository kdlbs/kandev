"use client";

import { useCallback } from "react";
import { TabsContent } from "@kandev/ui/tabs";
import { FileEditorContent } from "./file-editor-content";
import { FileImageViewer } from "./file-image-viewer";
import { FileBinaryViewer } from "./file-binary-viewer";
import type { OpenFileTab } from "@/lib/types/backend";
import { getFileCategory, isMarkdownFile } from "@/lib/utils/file-types";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import { FileViewerExternalLink } from "./file-viewer-header";
import { getFileTabKey } from "./task-center-panel-file-tabs";
import { defaultMarkdownFileMode, type MarkdownFileMode } from "./markdown-file-mode";
import { MarkdownFileEditor } from "./markdown-file-editor";
import { useMarkdownFileLinkHandler } from "./markdown-file-link-handler";

function resolveTabCategory(tab: OpenFileTab): "image" | "binary" | "text" {
  if (!tab.isBinary) return "text";
  return getFileCategory(tab.path) === "image" ? "image" : "binary";
}

type FileTabContentProps = {
  tab: OpenFileTab;
  activeSession: {
    workspace_path?: string | null;
    worktree_path?: string | null;
    repository_id?: string | null;
  } | null;
  activeSessionId: string | null;
  taskId?: string | null;
  isSaving: boolean;
  onFileChange: (path: string, content: string, repo?: string) => void;
  onFileSave: (path: string, repo?: string) => void;
  onFileDelete: (path: string, repo?: string) => void;
  onMarkdownModeChange?: (mode: MarkdownFileMode) => void;
  onOpenFile?: (path: string, repo?: string) => void;
};

function MarkdownFileTabContent({
  tab,
  activeSession,
  activeSessionId,
  taskId,
  isSaving,
  onFileChange,
  onFileSave,
  onFileDelete,
  onMarkdownModeChange,
  onOpenFile,
  workspacePath,
}: FileTabContentProps & { workspacePath?: string }) {
  const handleOpenFile = useCallback(
    (path: string) => onOpenFile?.(path, tab.repo),
    [onOpenFile, tab.repo],
  );
  const handleOpenLink = useMarkdownFileLinkHandler({
    path: tab.path,
    worktreePath: workspacePath,
    onOpenFile: onOpenFile ? handleOpenFile : undefined,
  });

  return (
    <MarkdownFileEditor
      path={tab.path}
      content={tab.content}
      originalContent={tab.originalContent}
      isDirty={tab.isDirty}
      isSaving={isSaving}
      sessionId={activeSessionId}
      taskId={taskId}
      repositoryId={activeSession?.repository_id}
      worktreePath={workspacePath}
      repo={tab.repo}
      enableComments={!!activeSessionId}
      mode={tab.markdownMode ?? defaultMarkdownFileMode(tab.path) ?? "source"}
      onModeChange={onMarkdownModeChange ?? (() => undefined)}
      onChange={(newContent) => onFileChange(tab.path, newContent, tab.repo)}
      onSave={() => onFileSave(tab.path, tab.repo)}
      onDelete={() => onFileDelete(tab.path, tab.repo)}
      onOpenFile={onOpenFile ? handleOpenFile : undefined}
      onOpenLink={onOpenFile ? handleOpenLink : undefined}
      onSourceFallback={() => onMarkdownModeChange?.("source")}
    />
  );
}

export function FileTabContent({
  tab,
  activeSession,
  activeSessionId,
  taskId,
  isSaving,
  onFileChange,
  onFileSave,
  onFileDelete,
  onMarkdownModeChange,
  onOpenFile,
}: FileTabContentProps) {
  const category = resolveTabCategory(tab);
  const workspacePath = getSessionWorkspacePath(activeSession);
  const externalLink = (
    <FileViewerExternalLink
      path={tab.path}
      sessionId={activeSessionId}
      taskId={taskId}
      repositoryId={activeSession?.repository_id}
      repositoryName={tab.repo}
    />
  );

  return (
    <TabsContent value={`file:${getFileTabKey(tab)}`} className="flex-1 min-h-0">
      {category === "image" && (
        <FileImageViewer
          path={tab.path}
          content={tab.content}
          worktreePath={workspacePath}
          headerActions={externalLink}
        />
      )}
      {category === "binary" && (
        <FileBinaryViewer
          path={tab.path}
          worktreePath={workspacePath}
          headerActions={externalLink}
        />
      )}
      {category === "text" &&
        (isMarkdownFile(tab.path) ? (
          <MarkdownFileTabContent
            tab={tab}
            activeSession={activeSession}
            activeSessionId={activeSessionId}
            taskId={taskId}
            isSaving={isSaving}
            onFileChange={onFileChange}
            onFileSave={onFileSave}
            onFileDelete={onFileDelete}
            onMarkdownModeChange={onMarkdownModeChange}
            onOpenFile={onOpenFile}
            workspacePath={workspacePath}
          />
        ) : (
          <FileEditorContent
            path={tab.path}
            content={tab.content}
            originalContent={tab.originalContent}
            isDirty={tab.isDirty}
            isSaving={isSaving}
            sessionId={activeSessionId || undefined}
            taskId={taskId}
            repositoryId={activeSession?.repository_id ?? undefined}
            worktreePath={workspacePath}
            repo={tab.repo}
            enableComments={!!activeSessionId}
            onChange={(newContent) => onFileChange(tab.path, newContent, tab.repo)}
            onSave={() => onFileSave(tab.path, tab.repo)}
            onDelete={() => onFileDelete(tab.path, tab.repo)}
          />
        ))}
    </TabsContent>
  );
}
