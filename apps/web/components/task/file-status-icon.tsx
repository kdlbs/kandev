"use client";

import { IconPlus, IconCircleFilled, IconMinus } from "@tabler/icons-react";
import type { FileInfo } from "@/lib/state/store";

type FileStatusIconProps = {
  status: FileInfo["status"];
};

export function FileStatusIcon({ status }: FileStatusIconProps) {
  switch (status) {
    case "added":
    case "untracked":
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-emerald-600">
          <IconPlus className="h-2 w-2 text-emerald-600" />
        </div>
      );
    case "modified":
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-yellow-600">
          <IconCircleFilled className="h-1 w-1 text-yellow-600" />
        </div>
      );
    case "deleted":
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-rose-600">
          <IconMinus className="h-2 w-2 text-rose-600" />
        </div>
      );
    case "renamed":
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-purple-600">
          <IconCircleFilled className="h-1 w-1 text-purple-600" />
        </div>
      );
    default:
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-muted-foreground">
          <IconCircleFilled className="h-1 w-1 text-muted-foreground" />
        </div>
      );
  }
}
