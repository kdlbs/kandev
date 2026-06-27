"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { useRepositoriesByWorkspace } from "@/hooks/domains/workspace/use-repository-cache";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { FilterDimension } from "@/lib/state/slices/ui/sidebar-view-types";
import { getExecutorLabel } from "@/lib/executor-icons";
import { repositorySlug } from "@/lib/repository-slug";
import type { Repository } from "@/lib/types/http";

type Option = { value: string; label: string; color?: string; group?: string };
type Snapshots = Record<string, WorkflowSnapshotData>;
type ReposByWorkspace = Record<string, Repository[]>;

function workflowOptions(snapshots: Snapshots): Option[] {
  return Object.entries(snapshots).map(([id, snap]) => ({
    value: id,
    label: snap.workflowName || id,
  }));
}

export function workflowStepOptions(snapshots: Snapshots): Option[] {
  const out: Option[] = [];
  const seen = new Set<string>();
  const workflows = Object.values(snapshots).sort((a, b) =>
    (a.workflowName || a.workflowId).localeCompare(b.workflowName || b.workflowId),
  );
  for (const snap of workflows) {
    const group = snap.workflowName || snap.workflowId;
    const steps = [...snap.steps].sort((a, b) => a.position - b.position);
    for (const step of steps) {
      if (seen.has(step.id)) continue;
      seen.add(step.id);
      out.push({ value: step.id, label: step.title, color: step.color, group });
    }
  }
  return out;
}

function executorTypeOptions(snapshots: Snapshots): Option[] {
  const seen = new Set<string>();
  for (const snap of Object.values(snapshots)) {
    for (const task of snap.tasks) {
      if (task.primaryExecutorType) seen.add(task.primaryExecutorType);
    }
  }
  return [...seen].sort().map((v) => ({ value: v, label: getExecutorLabel(v) }));
}

export function repositoryOptions(repositoriesByWorkspace: ReposByWorkspace): Option[] {
  const repos = Object.values(repositoriesByWorkspace).flat();
  return repos.map((r) => {
    const slug = repositorySlug(r);
    return { value: slug, label: slug };
  });
}

export function useFilterValueOptions(dimension: FilterDimension): Option[] {
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const { snapshots } = useAllWorkflowSnapshots(activeWorkspaceId);
  const repositoriesByWorkspace = useRepositoriesByWorkspace();

  return useMemo(() => {
    if (dimension === "workflow") return workflowOptions(snapshots);
    if (dimension === "workflowStep") return workflowStepOptions(snapshots);
    if (dimension === "executorType") return executorTypeOptions(snapshots);
    if (dimension === "repository") return repositoryOptions(repositoriesByWorkspace);
    return [];
  }, [dimension, snapshots, repositoriesByWorkspace]);
}
