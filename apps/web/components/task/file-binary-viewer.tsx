"use client";

import { IconFileOff } from "@tabler/icons-react";
import { toRelativePath } from "@/lib/utils";

type FileBinaryViewerProps = {
  path: string;
  worktreePath?: string;
};

export function FileBinaryViewer({ path, worktreePath }: FileBinaryViewerProps) {
  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center px-2 border-foreground/10 border-b">
        <div className="flex items-center gap-2 text-xs text-muted-foreground py-2">
          <span className="font-mono">{toRelativePath(path, worktreePath)}</span>
        </div>
      </div>
      <div className="flex-1 flex flex-col items-center justify-center gap-3 text-muted-foreground">
        <IconFileOff size={48} strokeWidth={1.2} />
        <div className="text-sm font-medium">Cannot preview this file</div>
        <div className="text-xs">Binary file</div>
      </div>
    </div>
  );
}
