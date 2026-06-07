"use client";

import { useEffect, useState } from "react";
import type {
  AgentUsageDTO,
  CompletedTaskActivityDTO,
  DailyActivityDTO,
  GitStatsDTO,
  GlobalStatsDTO,
  RepositoryStatsDTO,
  StatsResponse,
  TaskStatsDTO,
} from "@/lib/types/http";
import {
  fetchAgentUsage,
  fetchCompletedActivity,
  fetchDailyActivity,
  fetchGitStats,
  fetchGlobalStats,
  fetchRepositoryStats,
  fetchTaskStats,
  type TaskStatsResponse,
} from "@/lib/api/domains/stats-api";
import type { RangeKey } from "./stats-utils";

export type SectionStatus<T> =
  | { kind: "loading" }
  | { kind: "ready"; data: T }
  | { kind: "error"; message: string };

export type StatsSections = {
  global: SectionStatus<GlobalStatsDTO>;
  tasks: SectionStatus<TaskStatsResponse>;
  daily: SectionStatus<DailyActivityDTO[]>;
  completed: SectionStatus<CompletedTaskActivityDTO[]>;
  agents: SectionStatus<AgentUsageDTO[]>;
  repos: SectionStatus<RepositoryStatsDTO[]>;
  git: SectionStatus<GitStatsDTO>;
};

const LOADING: SectionStatus<never> = { kind: "loading" };

const INITIAL_SECTIONS: StatsSections = {
  global: LOADING,
  tasks: LOADING,
  daily: LOADING,
  completed: LOADING,
  agents: LOADING,
  repos: LOADING,
  git: LOADING,
};

function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : "Failed to load section";
}

export function useStatsSections(workspaceId: string | undefined, range: RangeKey): StatsSections {
  // Render-time reset: when (workspaceId, range) flips we wipe section state
  // before the effect runs so panels show skeletons while new fetches are in
  // flight, instead of stale data from the previous range. Matches the React
  // docs "reset state when a prop changes" pattern (no setState-in-effect).
  const fetchKey = workspaceId ? `${workspaceId}::${range}` : null;
  const [trackedKey, setTrackedKey] = useState<string | null>(null);
  const [sections, setSections] = useState<StatsSections>(INITIAL_SECTIONS);
  if (fetchKey !== trackedKey) {
    setTrackedKey(fetchKey);
    setSections(INITIAL_SECTIONS);
  }

  useEffect(() => {
    if (!workspaceId) return;
    const controller = new AbortController();

    const opts = { init: { signal: controller.signal } };
    const apply = <K extends keyof StatsSections>(key: K, next: StatsSections[K]) => {
      if (controller.signal.aborted) return;
      setSections((prev) => ({ ...prev, [key]: next }));
    };

    const run = <K extends keyof StatsSections>(
      key: K,
      promise: Promise<StatsSections[K] extends SectionStatus<infer U> ? U : never>,
    ) => {
      promise
        .then((data) => apply(key, { kind: "ready", data } as StatsSections[K]))
        .catch((e: unknown) => {
          if (controller.signal.aborted) return;
          apply(key, { kind: "error", message: errorMessage(e) } as StatsSections[K]);
        });
    };

    run("global", fetchGlobalStats(workspaceId, opts, range));
    run("tasks", fetchTaskStats(workspaceId, opts, range));
    run("daily", fetchDailyActivity(workspaceId, opts, range));
    run("completed", fetchCompletedActivity(workspaceId, opts, range));
    run("agents", fetchAgentUsage(workspaceId, opts, range));
    run("repos", fetchRepositoryStats(workspaceId, opts, range));
    run("git", fetchGitStats(workspaceId, opts, range));

    return () => controller.abort();
  }, [workspaceId, range]);

  return sections;
}

export function readyGlobal(sections: StatsSections): GlobalStatsDTO | null {
  return sections.global.kind === "ready" ? sections.global.data : null;
}

export function anyError(sections: StatsSections): string | null {
  for (const key of Object.keys(sections) as (keyof StatsSections)[]) {
    const s = sections[key];
    if (s.kind === "error") return s.message;
  }
  return null;
}

// composeStatsResponse builds a full StatsResponse when every section is ready.
// Returns null otherwise — used to gate the Copy Stats button.
export function composeStatsResponse(sections: StatsSections): StatsResponse | null {
  const { global, tasks, daily, completed, agents, repos, git } = sections;
  if (
    global.kind !== "ready" ||
    tasks.kind !== "ready" ||
    daily.kind !== "ready" ||
    completed.kind !== "ready" ||
    agents.kind !== "ready" ||
    repos.kind !== "ready" ||
    git.kind !== "ready"
  ) {
    return null;
  }
  return {
    global: global.data,
    task_stats: tasks.data.task_stats,
    daily_activity: daily.data,
    completed_activity: completed.data,
    agent_usage: agents.data,
    repository_stats: repos.data,
    git_stats: git.data,
  };
}

export type TaskStatsSectionStatus = SectionStatus<TaskStatsDTO[]>;

// flattenTaskStats unwraps the {task_stats, has_more} envelope into the bare
// list expected by render helpers and the WorkloadSection component.
export function flattenTaskStats(status: SectionStatus<TaskStatsResponse>): TaskStatsSectionStatus {
  if (status.kind === "ready") return { kind: "ready", data: status.data.task_stats };
  return status;
}
