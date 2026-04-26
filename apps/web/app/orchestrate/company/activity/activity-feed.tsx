"use client";

import { useEffect, useState } from "react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { listActivity } from "@/lib/api/domains/orchestrate-api";
import type { ActivityEntry } from "@/lib/state/slices/orchestrate/types";
import { ActivityRow } from "./activity-row";

const FILTER_OPTIONS = [
  { value: "all", label: "All types" },
  { value: "agent", label: "Agent" },
  { value: "task", label: "Task" },
  { value: "project", label: "Project" },
  { value: "budget", label: "Budget" },
  { value: "approval", label: "Approval" },
  { value: "system", label: "System" },
];

export function ActivityFeed({ workspaceId }: { workspaceId: string }) {
  const [entries, setEntries] = useState<ActivityEntry[]>([]);
  const [filterType, setFilterType] = useState("all");

  useEffect(() => {
    listActivity(workspaceId, filterType)
      .then((res) => setEntries(res.activity ?? []))
      .catch(() => {});
  }, [workspaceId, filterType]);

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h1 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          Activity
        </h1>
        <Select value={filterType} onValueChange={setFilterType}>
          <SelectTrigger className="w-[140px] h-8 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {FILTER_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {entries.length === 0 ? (
        <p className="text-sm text-muted-foreground">No activity recorded yet.</p>
      ) : (
        <div className="border border-border rounded-lg divide-y divide-border">
          {entries.map((entry) => (
            <ActivityRow key={entry.id} entry={entry} />
          ))}
        </div>
      )}
    </div>
  );
}
