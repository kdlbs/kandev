"use client";

import { TabsContent } from "@kandev/ui/tabs";
import { FileEditorContent } from "./file-editor-content";
import { FileImageViewer } from "./file-image-viewer";
import { FileBinaryViewer } from "./file-binary-viewer";
import type { OpenFileTab } from "@/lib/types/backend";
import { getFileCategory } from "@/lib/utils/file-types";
import { FileViewerExternalLink } from "./file-viewer-header";

function resolveTabCategory(tab: OpenFileTab): "image" | "binary" | "text" {
  if (!tab.isBinary) return "text";
  return getFileCategory(tab.path) === "image" ? "image" : "binary";
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
}: {
  tab: OpenFileTab;
  activeSession: { worktree_path?: string | null; repository_id?: string | null } | null;
  activeSessionId: string | null;
  taskId?: string | null;
  isSaving: boolean;
  onFileChange: (path: string, content: string) => void;
  onFileSave: (path: string) => void;
  onFileDelete: (path: string) => void;
}) {
  const category = resolveTabCategory(tab);
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
    <TabsContent value={`file:${tab.path}`} className="flex-1 min-h-0">
      {category === "image" && (
        <FileImageViewer
          path={tab.path}
          content={tab.content}
          worktreePath={activeSession?.worktree_path ?? undefined}
          headerActions={externalLink}
        />
      )}
      {category === "binary" && (
        <FileBinaryViewer
          path={tab.path}
          worktreePath={activeSession?.worktree_path ?? undefined}
          headerActions={externalLink}
        />
      )}
      {category === "text" && (
        <FileEditorContent
          path={tab.path}
          content={tab.content}
          originalContent={tab.originalContent}
          isDirty={tab.isDirty}
          isSaving={isSaving}
          sessionId={activeSessionId || undefined}
          taskId={taskId}
          repositoryId={activeSession?.repository_id ?? undefined}
          worktreePath={activeSession?.worktree_path ?? undefined}
          repo={tab.repo}
          enableComments={!!activeSessionId}
          onChange={(newContent) => onFileChange(tab.path, newContent)}
          onSave={() => onFileSave(tab.path)}
          onDelete={() => onFileDelete(tab.path)}
        />
      )}
    </TabsContent>
  );
}
