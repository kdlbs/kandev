"use client";

import type { FlatIssueNode } from "./use-issues-tree";
import type { IssueViewMode, OrchestrateIssue } from "@/lib/state/slices/orchestrate/types";
import { IssueRow } from "./issue-row";
import { IssueBoard } from "./issue-board";

type IssuesContentProps = {
  viewMode: IssueViewMode;
  isLoading: boolean;
  flatNodes: FlatIssueNode[];
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  agentMap: Map<string, string>;
};

function EmptyState({ message }: { message: string }) {
  return (
    <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
      {message}
    </div>
  );
}

function IssueListView({
  flatNodes,
  expandedIds,
  onToggleExpand,
  agentMap,
}: {
  flatNodes: FlatIssueNode[];
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  agentMap: Map<string, string>;
}) {
  if (flatNodes.length === 0) return <EmptyState message="No issues found" />;

  return (
    <div className="border border-border rounded-lg divide-y divide-border">
      {flatNodes.map((node) => (
        <IssueRow
          key={node.issue.id}
          issue={node.issue}
          level={node.level}
          hasChildren={node.hasChildren}
          expanded={expandedIds.has(node.issue.id)}
          onToggleExpand={onToggleExpand}
          agentName={resolveAgent(node.issue, agentMap)}
        />
      ))}
    </div>
  );
}

function resolveAgent(issue: OrchestrateIssue, agentMap: Map<string, string>): string | undefined {
  if (!issue.assigneeAgentInstanceId) return undefined;
  return agentMap.get(issue.assigneeAgentInstanceId);
}

export function IssuesContent({
  viewMode,
  isLoading,
  flatNodes,
  expandedIds,
  onToggleExpand,
  agentMap,
}: IssuesContentProps) {
  if (isLoading) return <EmptyState message="Loading issues..." />;

  if (viewMode === "board") {
    return <IssueBoard issues={flatNodes.map((n) => n.issue)} />;
  }

  return (
    <IssueListView
      flatNodes={flatNodes}
      expandedIds={expandedIds}
      onToggleExpand={onToggleExpand}
      agentMap={agentMap}
    />
  );
}
