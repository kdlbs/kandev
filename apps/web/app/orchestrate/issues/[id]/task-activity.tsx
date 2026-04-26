"use client";

import { formatRelativeTime } from "@/lib/utils";
import type { IssueActivityEntry } from "./types";

type TaskActivityProps = {
  taskId: string;
  entries: IssueActivityEntry[];
};

function ActivityRow({ entry }: { entry: IssueActivityEntry }) {
  return (
    <div className="flex items-start gap-3 px-0 py-2 text-sm">
      <div className="h-6 w-6 rounded-full bg-muted flex items-center justify-center shrink-0 mt-0.5">
        <span className="text-[10px] font-medium text-muted-foreground">
          {entry.actorName.charAt(0).toUpperCase()}
        </span>
      </div>
      <div className="flex-1 min-w-0">
        <span className="font-medium">{entry.actorName}</span>
        <span className="text-muted-foreground"> {entry.actionVerb} </span>
        {entry.targetName && <span className="font-medium">{entry.targetName}</span>}
      </div>
      <span className="text-xs text-muted-foreground shrink-0">
        {formatRelativeTime(entry.createdAt)}
      </span>
    </div>
  );
}

export function TaskActivity({ taskId, entries }: TaskActivityProps) {
  void taskId;

  if (entries.length === 0) {
    return <p className="text-sm text-muted-foreground py-4">No activity yet</p>;
  }

  return (
    <div className="divide-y divide-border/50">
      {entries.map((entry) => (
        <ActivityRow key={entry.id} entry={entry} />
      ))}
    </div>
  );
}
