"use client";

import { toRelativePath } from "@/lib/utils";
import { getImageMimeType } from "@/lib/utils/file-types";

type FileImageViewerProps = {
  path: string;
  content: string; // base64-encoded
  worktreePath?: string;
};

export function FileImageViewer({ path, content, worktreePath }: FileImageViewerProps) {
  const mime = getImageMimeType(path);
  const src = `data:${mime};base64,${content}`;

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center px-2 border-foreground/10 border-b">
        <div className="flex items-center gap-2 text-xs text-muted-foreground py-2">
          <span className="font-mono">{toRelativePath(path, worktreePath)}</span>
        </div>
      </div>
      <div className="flex-1 flex items-center justify-center overflow-auto p-6">
        {/* eslint-disable-next-line @next/next/no-img-element -- dynamic blob/data URLs from the agent workspace */}
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
