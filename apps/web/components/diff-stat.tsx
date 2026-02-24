"use client";

import { cn } from "@/lib/utils";
import { Badge } from "@kandev/ui/badge";

type CommitStatBadgeProps = {
  label: string;
  tone: "ahead" | "behind";
  className?: string;
};

type LineStatProps = {
  added?: number;
  removed?: number;
  className?: string;
};

const commitTone = {
  ahead: "text-emerald-500 border-emerald-500/40",
  behind: "text-yellow-500 border-yellow-500/40",
};

const lineTone = {
  add: "text-emerald-600",
  remove: "text-rose-600",
};

export function CommitStatBadge({ label, tone, className }: CommitStatBadgeProps) {
  return (
    <Badge
      variant="outline"
      className={cn("h-6 px-1.5 text-xs font-medium rounded-md", commitTone[tone], className)}
    >
      {label}
    </Badge>
  );
}

export function LineStat({ added, removed, className }: LineStatProps) {
  return (
    <span className={cn("inline-flex items-center gap-2 text-xs font-semibold", className)}>
      {typeof added === "number" && <span className={lineTone.add}>+{added}</span>}
      {typeof removed === "number" && <span className={lineTone.remove}>-{removed}</span>}
    </span>
  );
}
