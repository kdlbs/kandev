"use client";

import { useDockviewStore } from "@/lib/state/dockview-store";

export function useFileEditorBuffer(fileKey: string) {
  const hasFile = useDockviewStore((s) => s.openFiles.has(fileKey));
  const isSymlink = useDockviewStore((s) => !!s.openFiles.get(fileKey)?.resolvedPath);
  const content = useDockviewStore((s) => s.openFiles.get(fileKey)?.content ?? "");
  const isDirty = useDockviewStore((s) => s.openFiles.get(fileKey)?.isDirty ?? false);
  const hasRemoteUpdate = useDockviewStore(
    (s) => s.openFiles.get(fileKey)?.hasRemoteUpdate ?? false,
  );
  const isBinary = useDockviewStore((s) => s.openFiles.get(fileKey)?.isBinary ?? false);
  const originalContent = useDockviewStore((s) => s.openFiles.get(fileKey)?.originalContent ?? "");
  const originalHash = useDockviewStore((s) => s.openFiles.get(fileKey)?.originalHash ?? "");
  const renderedPreview = useDockviewStore(
    (s) => s.openFiles.get(fileKey)?.renderedPreview ?? false,
  );
  const markdownMode = useDockviewStore((s) => s.openFiles.get(fileKey)?.markdownMode);
  return {
    hasFile,
    isSymlink,
    content,
    isDirty,
    hasRemoteUpdate,
    isBinary,
    originalContent,
    originalHash,
    renderedPreview,
    markdownMode,
  };
}
