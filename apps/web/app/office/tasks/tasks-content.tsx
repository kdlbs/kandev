"use client";

import type { FlatTaskNode } from "./use-tasks-tree";
import type { TaskViewMode, OfficeTask } from "@/lib/state/slices/office/types";
import { TaskRow } from "./task-row";
import { TaskBoard } from "./task-board";
import { EmptyState } from "../components/shared/empty-state";
import { useTranslation } from "react-i18next";

type IssuesContentProps = {
  viewMode: TaskViewMode;
  isLoading: boolean;
  flatNodes: FlatTaskNode[];
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  agentMap: Map<string, string>;
  onTaskPatch?: (taskId: string, patch: Partial<OfficeTask>) => void;
};

function IssueListView({
  flatNodes,
  expandedIds,
  onToggleExpand,
  agentMap,
}: {
  flatNodes: FlatTaskNode[];
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  agentMap: Map<string, string>;
}) {
  const { t } = useTranslation();
  if (flatNodes.length === 0)
    return (
      <EmptyState
        message={t("office:noTasksFound")}
        description={t("office:createATaskOrLetAgents")}
      />
    );

  return (
    <div className="border border-border rounded-lg divide-y divide-border">
      {flatNodes.map((node) => (
        <TaskRow
          key={node.task.id}
          task={node.task}
          level={node.level}
          hasChildren={node.hasChildren}
          expanded={expandedIds.has(node.task.id)}
          onToggleExpand={onToggleExpand}
          agentName={resolveAgent(node.task, agentMap)}
        />
      ))}
    </div>
  );
}

function resolveAgent(task: OfficeTask, agentMap: Map<string, string>): string | undefined {
  if (!task.assigneeAgentProfileId) return undefined;
  return agentMap.get(task.assigneeAgentProfileId);
}

export function TasksContent({
  viewMode,
  isLoading,
  flatNodes,
  expandedIds,
  onToggleExpand,
  agentMap,
  onTaskPatch,
}: IssuesContentProps) {
  const { t } = useTranslation();
  if (isLoading) return <EmptyState message={t("office:loadingTasks")} />;

  if (viewMode === "board") {
    return <TaskBoard tasks={flatNodes.map((n) => n.task)} onTaskPatch={onTaskPatch} />;
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
