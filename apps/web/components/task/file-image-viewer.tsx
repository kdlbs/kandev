"use client";

import type { ReactNode } from "react";
import { getImageMimeType } from "@/lib/utils/file-types";
import { FileViewerHeader } from "./file-viewer-header";

type FileImageViewerProps = {
  path: string;
  content: string; // base64-encoded
  worktreePath?: string;
  headerActions?: ReactNode;
};

export function FileImageViewer({
  path,
  content,
  worktreePath,
  headerActions,
}: FileImageViewerProps) {
  const mime = getImageMimeType(path);
  const src = `data:${mime};base64,${content}`;

  return (
    <div className="flex flex-col h-full">
      <FileViewerHeader path={path} worktreePath={worktreePath} actions={headerActions} />
      <div className="flex-1 flex items-center justify-center overflow-auto p-6">
        <img
          src={src}
          alt={path}
          className="max-w-full max-h-full object-contain rounded"
          draggable={false}
        />
      </div>
    </div>
  );
}
