import type { TaskDecisionDTO } from "@/lib/api/domains/office-api";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import type { Task, TaskDecision } from "./types";

export function mapDecisionDTO(d: TaskDecisionDTO): TaskDecision {
  return {
    id: d.id,
    taskId: d.task_id,
    deciderType: d.decider_type,
    deciderId: d.decider_id,
    deciderName: d.decider_name ?? "",
    role: d.role,
    decision: d.decision,
    comment: d.comment ?? "",
    createdAt: d.created_at,
  };
}

export function mapOfficeTaskToTask(
  raw: OfficeTask,
  supplement?: {
    repositories?: Task["repositories"];
    status_summary?: Task["statusSummary"];
  },
): Task {
  // The server DTO includes reviewers/approvers/decisions even though the
  // strongly-typed OfficeTask only declares the cross-cutting fields. We
  // read those extra props off the raw object.
  const extra = raw as OfficeTask & {
    reviewers?: string[];
    approvers?: string[];
    decisions?: TaskDecisionDTO[];
    blockedBy?: string[];
    status_summary?: Task["statusSummary"];
  };
  return {
    id: raw.id,
    workspaceId: raw.workspaceId,
    identifier: raw.identifier,
    title: raw.title,
    description: raw.description,
    status: raw.status as Task["status"],
    rawStatus: raw.rawStatus ?? raw.status,
    priority: (raw.priority || "medium") as Task["priority"],
    labels: (raw.labels ?? []).map((l) =>
      typeof l === "string" ? { name: l, color: "#6b7280" } : l,
    ),
    assigneeAgentProfileId: raw.assigneeAgentProfileId,
    assigneeUserId: raw.assigneeUserId,
    parentId: raw.parentId,
    projectId: raw.projectId,
    blockedBy: extra.blockedBy ?? [],
    blocking: [],
    children: (raw.children ?? []).map((child) => ({
      id: child.id,
      identifier: child.identifier,
      title: child.title,
      status: child.status as Task["status"],
      blockedBy: child.blockedBy ?? [],
      createdAt: child.createdAt,
    })),
    reviewers: extra.reviewers ?? [],
    approvers: extra.approvers ?? [],
    decisions: (extra.decisions ?? []).map(mapDecisionDTO),
    createdBy: "",
    startedAt: raw.startedAt,
    completedAt: raw.completedAt,
    createdAt: raw.createdAt,
    updatedAt: raw.updatedAt,
    executionPolicy: raw.executionPolicy,
    executionState: raw.executionState,
    statusSummary: raw.statusSummary ?? extra.status_summary ?? supplement?.status_summary ?? null,
    repositories: raw.repositories ?? supplement?.repositories,
  };
}
