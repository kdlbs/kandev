"use client";

import { IconArrowRight, IconCircleFilled, IconMinus, IconPlus } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { fileChangeStatusLabel, type FileChangeStatus } from "@/lib/utils/file-change-status";

type FileStatusIconProps = {
  status: FileChangeStatus;
  oldPath?: string;
  className?: string;
};

const markerClassName: Record<FileChangeStatus, string> = {
  added: "border-emerald-600 text-emerald-600",
  untracked: "border-emerald-600 text-emerald-600",
  modified: "border-amber-600 text-amber-600",
  deleted: "border-rose-600 text-rose-600",
  renamed: "border-purple-600 text-purple-600",
};

function StatusGlyph({ status }: { status: FileChangeStatus }) {
  switch (status) {
    case "added":
    case "untracked":
      return <IconPlus aria-hidden="true" className="h-2 w-2" />;
    case "modified":
      return <IconCircleFilled aria-hidden="true" className="h-1 w-1" />;
    case "deleted":
      return <IconMinus aria-hidden="true" className="h-2 w-2" />;
    case "renamed":
      return <IconArrowRight aria-hidden="true" className="h-2 w-2" />;
    default: {
      const exhaustiveStatus: never = status;
      return exhaustiveStatus;
    }
  }
}

export function FileStatusIcon({ status, oldPath, className }: FileStatusIconProps) {
  const label = fileChangeStatusLabel(status, oldPath);

  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      data-file-status={status}
      className={cn(
        "flex h-3 w-3 shrink-0 items-center justify-center rounded border",
        markerClassName[status],
        className,
      )}
    >
      <StatusGlyph status={status} />
    </span>
  );
}
