"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { FilterDimension } from "@/lib/state/slices/ui/sidebar-view-types";

type Option = { value: string; label: string };
type Snapshots = AppState["kanbanMulti"]["snapshots"];
type ReposByWorkspace = AppState["repositories"]["itemsByWorkspaceId"];

function workflowOptions(snapshots: Snapshots): Option[] {
  return Object.entries(snapshots).map(([id, snap]) => ({
    value: id,
    label: snap.workflowName || id,
  }));
}

function workflowStepOptions(snapshots: Snapshots): Option[] {
  const seen = new Map<string, string>();
  for (const snap of Object.values(snapshots)) {
    for (const step of snap.steps) {
      if (!seen.has(step.id)) seen.set(step.id, step.title);
    }
  }
  return [...seen.entries()].map(([value, label]) => ({ value, label }));
}

function executorTypeOptions(snapshots: Snapshots): Option[] {
  const seen = new Set<string>();
  for (const snap of Object.values(snapshots)) {
    for (const task of snap.tasks) {
      if (task.primaryExecutorType) seen.add(task.primaryExecutorType);
    }
  }
  return [...seen].sort().map((v) => ({ value: v, label: v }));
}

function repositoryOptions(repositoriesByWorkspace: ReposByWorkspace): Option[] {
  const repos = Object.values(repositoriesByWorkspace).flat();
  return repos.map((r) => {
    const slug =
      r.provider_owner && r.provider_name
        ? `${r.provider_owner}/${r.provider_name}`
        : r.local_path;
    return { value: slug, label: slug };
  });
}

export function useFilterValueOptions(dimension: FilterDimension): Option[] {
  const snapshots = useAppStore((s) => s.kanbanMulti.snapshots);
  const repositoriesByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);

  return useMemo(() => {
    if (dimension === "workflow") return workflowOptions(snapshots);
    if (dimension === "workflowStep") return workflowStepOptions(snapshots);
    if (dimension === "executorType") return executorTypeOptions(snapshots);
    if (dimension === "repository") return repositoryOptions(repositoriesByWorkspace);
    return [];
  }, [dimension, snapshots, repositoriesByWorkspace]);
}
