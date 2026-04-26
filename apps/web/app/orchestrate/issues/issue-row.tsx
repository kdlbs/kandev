"use client";

import { useRouter } from "next/navigation";
import { IconChevronRight } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/utils";
import type { OrchestrateIssue } from "@/lib/state/slices/orchestrate/types";
import { StatusIcon } from "./status-icon";

type IssueRowProps = {
  issue: OrchestrateIssue;
  level: number;
  hasChildren: boolean;
  expanded: boolean;
  onToggleExpand: (id: string) => void;
  agentName?: string;
};

export function IssueRow({
  issue,
  level,
  hasChildren,
  expanded,
  onToggleExpand,
  agentName,
}: IssueRowProps) {
  const router = useRouter();

  const handleClick = () => {
    router.push(`/orchestrate/issues/${issue.id}`);
  };

  const handleToggle = (e: React.MouseEvent) => {
    e.stopPropagation();
    onToggleExpand(issue.id);
  };

  return (
    <div
      className="flex items-center gap-2 px-4 py-2.5 text-sm hover:bg-accent/50 transition-colors cursor-pointer"
      style={{ paddingLeft: `${16 + level * 24}px` }}
      onClick={handleClick}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === "Enter" && handleClick()}
    >
      {hasChildren ? (
        <button
          onClick={handleToggle}
          className="shrink-0 cursor-pointer p-0 border-0 bg-transparent"
          aria-label={expanded ? "Collapse" : "Expand"}
        >
          <IconChevronRight
            className={cn(
              "h-3.5 w-3.5 text-muted-foreground transition-transform",
              expanded && "rotate-90",
            )}
          />
        </button>
      ) : (
        <span className="w-3.5 shrink-0" />
      )}
      <StatusIcon status={issue.status} className="h-4 w-4 shrink-0" />
      <span className="text-xs text-muted-foreground font-mono shrink-0">
        {issue.identifier}
      </span>
      <span className="flex-1 truncate">{issue.title}</span>
      {agentName && (
        <span className="text-xs text-muted-foreground shrink-0 truncate max-w-[100px]">
          {agentName}
        </span>
      )}
      <span className="text-xs text-muted-foreground shrink-0">
        {formatRelativeTime(issue.updatedAt)}
      </span>
    </div>
  );
}
