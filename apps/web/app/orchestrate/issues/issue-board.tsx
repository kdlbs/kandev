"use client";

import { useRouter } from "next/navigation";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { useAppStore } from "@/components/state-provider";
import type {
  OrchestrateIssue,
  OrchestrateIssueStatus,
} from "@/lib/state/slices/orchestrate/types";
import { StatusIcon } from "./status-icon";

const FALLBACK_COLUMNS: { status: OrchestrateIssueStatus; label: string }[] = [
  { status: "backlog", label: "Backlog" },
  { status: "todo", label: "Todo" },
  { status: "in_progress", label: "In Progress" },
  { status: "in_review", label: "In Review" },
  { status: "blocked", label: "Blocked" },
  { status: "done", label: "Done" },
  { status: "cancelled", label: "Cancelled" },
];

type IssueBoardProps = {
  issues: OrchestrateIssue[];
};

function BoardCard({ issue }: { issue: OrchestrateIssue }) {
  const router = useRouter();

  return (
    <div
      className="rounded-md border border-border bg-card p-3 hover:bg-accent/50 transition-colors cursor-pointer"
      onClick={() => router.push(`/orchestrate/issues/${issue.id}`)}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === "Enter" && router.push(`/orchestrate/issues/${issue.id}`)}
    >
      <div className="flex items-center gap-1.5 mb-1">
        <StatusIcon status={issue.status} className="h-3.5 w-3.5 shrink-0" />
        <span className="text-[11px] text-muted-foreground font-mono">{issue.identifier}</span>
      </div>
      <p className="text-sm truncate">{issue.title}</p>
    </div>
  );
}

function BoardColumn({
  label,
  status,
  issues,
}: {
  label: string;
  status: OrchestrateIssueStatus;
  issues: OrchestrateIssue[];
}) {
  return (
    <div className="flex flex-col min-w-[240px] max-w-[300px] flex-1">
      <div className="flex items-center gap-2 px-2 py-2 mb-2">
        <StatusIcon status={status} className="h-3.5 w-3.5" />
        <span className="text-xs font-medium">{label}</span>
        <span className="text-xs text-muted-foreground">{issues.length}</span>
      </div>
      <ScrollArea className="flex-1">
        <div className="flex flex-col gap-1.5 px-1 pb-2">
          {issues.map((issue) => (
            <BoardCard key={issue.id} issue={issue} />
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}

export function IssueBoard({ issues }: IssueBoardProps) {
  const meta = useAppStore((s) => s.orchestrate.meta);
  const columns = meta
    ? meta.statuses.map((s) => ({ status: s.id as OrchestrateIssueStatus, label: s.label }))
    : FALLBACK_COLUMNS;

  const grouped = new Map<OrchestrateIssueStatus, OrchestrateIssue[]>();
  for (const col of columns) {
    grouped.set(col.status, []);
  }
  for (const issue of issues) {
    const list = grouped.get(issue.status);
    if (list) list.push(issue);
  }

  return (
    <div className="flex gap-3 overflow-x-auto pb-4">
      {columns.map((col) => (
        <BoardColumn
          key={col.status}
          label={col.label}
          status={col.status}
          issues={grouped.get(col.status) ?? []}
        />
      ))}
    </div>
  );
}
