"use client";

import type { ActivityEntry } from "@/lib/state/slices/orchestrate/types";
import { timeAgo } from "../../components/shared/time-ago";

function formatAction(action: string): string {
  return action.replace(/[._]/g, " ");
}

function actorInitial(actorType: string): string {
  if (actorType === "system") return "S";
  if (actorType === "agent") return "A";
  return "U";
}

function actorLabel(entry: ActivityEntry): string {
  if (entry.actorType === "system") return "System";
  return entry.actorId || entry.actorType;
}

type Props = {
  entry: ActivityEntry;
};

export function ActivityRow({ entry }: Props) {
  return (
    <div className="flex items-start gap-3 px-4 py-2.5 text-sm hover:bg-accent/50 transition-colors">
      <div className="h-6 w-6 rounded-full bg-muted flex items-center justify-center shrink-0 text-[10px] font-medium uppercase text-muted-foreground">
        {actorInitial(entry.actorType)}
      </div>
      <div className="flex-1 min-w-0">
        <span className="font-medium">{actorLabel(entry)}</span>
        <span className="text-muted-foreground"> {formatAction(entry.action)} </span>
        {entry.targetType && (
          <span className="font-medium">
            {entry.targetType}
            {entry.targetId ? ` ${entry.targetId}` : ""}
          </span>
        )}
      </div>
      <span className="text-xs text-muted-foreground shrink-0">{timeAgo(entry.createdAt)}</span>
    </div>
  );
}
