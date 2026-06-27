"use client";

import { useState } from "react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useOfficeActivityData } from "@/hooks/domains/office/use-office-data";
import type { ActivityEntry } from "@/lib/state/slices/office/types";
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

export function ActivityFeed({
  workspaceId,
  initialActivity = [],
}: {
  workspaceId: string;
  initialActivity?: ActivityEntry[];
}) {
  const [filterType, setFilterType] = useState("all");
  const activityQuery = useOfficeActivityData(workspaceId, filterType, initialActivity);
  const entries = activityQuery.data?.activity ?? (filterType === "all" ? initialActivity : []);

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
        <EmptyState
          message="No activity yet."
          description="Actions by agents and users are logged here."
        />
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
