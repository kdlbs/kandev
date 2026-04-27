"use client";

import { useEffect, useState } from "react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { toast } from "sonner";
import { listActivity } from "@/lib/api/domains/orchestrate-api";
import type { ActivityEntry } from "@/lib/state/slices/orchestrate/types";
import { ActivityRow } from "./activity-row";
import { EmptyState } from "../../components/shared/empty-state";
import { PageHeader } from "../../components/shared/page-header";

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
      .catch((err) => {
        toast.error(err instanceof Error ? err.message : "Failed to load activity");
      });
  }, [workspaceId, filterType]);

  return (
    <div className="space-y-4">
      <PageHeader
        title="Activity"
        action={
          <Select value={filterType} onValueChange={setFilterType}>
            <SelectTrigger className="w-[140px] h-8 text-xs cursor-pointer">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {FILTER_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value} className="cursor-pointer">
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />

      {entries.length === 0 ? (
        <EmptyState message="No activity yet." description="Actions by agents and users are logged here." />
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
