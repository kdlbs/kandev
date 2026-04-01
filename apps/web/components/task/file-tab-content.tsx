"use client";

import { TabsContent } from "@kandev/ui/tabs";
import { FileEditorContent } from "./file-editor-content";
import { FileImageViewer } from "./file-image-viewer";
import { FileBinaryViewer } from "./file-binary-viewer";
import type { OpenFileTab } from "@/lib/types/backend";
import { getFileCategory } from "@/lib/utils/file-types";

export function FileTabContent({
  tab,
  activeSession,
  activeSessionId,
  isSaving,
  onFileChange,
  onFileSave,
  onFileDelete,
}: {
  tab: OpenFileTab;
  activeSession: { worktree_path?: string | null } | null;
  activeSessionId: string | null;
  isSaving: boolean;
  onFileChange: (path: string, content: string) => void;
  onFileSave: (path: string) => void;
  onFileDelete: (path: string) => void;
}) {
  const extCategory = getFileCategory(tab.path);
  let category: "image" | "binary" | "text" = "text";
  if (tab.isBinary) category = extCategory === "image" ? "image" : "binary";

  return (
    <TabsContent value={`file:${tab.path}`} className="flex-1 min-h-0">
      {category === "image" && (
        <FileImageViewer
          path={tab.path}
          content={tab.content}
          worktreePath={activeSession?.worktree_path ?? undefined}
        />
      )}
      {category === "binary" && (
        <FileBinaryViewer
          path={tab.path}
          worktreePath={activeSession?.worktree_path ?? undefined}
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
          worktreePath={activeSession?.worktree_path ?? undefined}
          enableComments={!!activeSessionId}
          onChange={(newContent) => onFileChange(tab.path, newContent)}
          onSave={() => onFileSave(tab.path)}
          onDelete={() => onFileDelete(tab.path)}
        />
      )}
    </TabsContent>
  );
}
