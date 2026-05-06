"use client";

import { useAppStore } from "@/components/state-provider";
import { formatRelativeTime } from "@/lib/utils";
import { AgentAvatar } from "@/app/office/components/agent-avatar";
import type { TaskActivityEntry } from "@/app/office/tasks/[id]/types";

type TaskActivityProps = {
  taskId: string;
  entries: TaskActivityEntry[];
};

function ActivityRow({ entry }: { entry: TaskActivityEntry }) {
  const agentName = useAppStore((s) =>
    entry.actorType === "agent"
      ? (s.office.agentProfiles.find((a) => a.id === entry.actorId)?.name ?? "Agent")
      : "",
  );
  let actorName = "System";
  if (entry.actorType === "user") actorName = "You";
  else if (entry.actorType === "agent") actorName = agentName;

  return (
    <div className="flex items-start gap-3 px-0 py-2 text-sm">
      {entry.actorType === "agent" ? (
        <AgentAvatar name={actorName} size="sm" className="mt-0.5" />
      ) : (
        <div className="h-6 w-6 rounded-md bg-muted flex items-center justify-center shrink-0 mt-0.5">
          <span className="text-[10px] font-medium text-muted-foreground">
            {actorName.charAt(0).toUpperCase()}
          </span>
        </div>
      )}
      <div className="flex-1 min-w-0">
        <span className="font-medium">{actorName}</span>
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
