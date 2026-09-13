"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type {
  ModelUsageDTO,
  CompletedTaskActivityDTO,
  DailyActivityDTO,
  GitStatsDTO,
  GlobalStatsDTO,
  RepositoryStatsDTO,
  StatsResponse,
  TaskStatsDTO,
} from "@/lib/types/http";
import {
  fetchModelUsage,
  fetchCompletedActivity,
  fetchDailyActivity,
  fetchGitStats,
  fetchGlobalStats,
  fetchRepositoryStats,
  fetchTaskStats,
  type TaskStatsResponse,
} from "@/lib/api/domains/stats-api";
import type { ApiRequestOptions } from "@/lib/api/client";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import type { RangeKey } from "./stats-utils";

export type SectionStatus<T> =
  | { kind: "loading" }
  | { kind: "ready"; data: T }
  | {
      kind: "error";
      message: string;
      retryable: boolean;
      retryAfterMs?: number;
      retrying?: boolean;
      data?: T;
    };

export type StatsSections = {
  global: SectionStatus<GlobalStatsDTO>;
  tasks: SectionStatus<TaskStatsResponse>;
  daily: SectionStatus<DailyActivityDTO[]>;
  completed: SectionStatus<CompletedTaskActivityDTO[]>;
  models: SectionStatus<ModelUsageDTO[]>;
  repos: SectionStatus<RepositoryStatsDTO[]>;
  git: SectionStatus<GitStatsDTO>;
};

const LOADING: SectionStatus<never> = { kind: "loading" };

const INITIAL_SECTIONS: StatsSections = {
  global: LOADING,
  tasks: LOADING,
  daily: LOADING,
  completed: LOADING,
  models: LOADING,
  repos: LOADING,
  git: LOADING,
};

export const STATS_SECTION_KEYS = [
  "global",
  "tasks",
  "daily",
  "completed",
  "models",
  "repos",
  "git",
] as const satisfies readonly (keyof StatsSections)[];

export type StatsSectionKey = (typeof STATS_SECTION_KEYS)[number];

type SectionData = {
  global: GlobalStatsDTO;
  tasks: TaskStatsResponse;
  daily: DailyActivityDTO[];
  completed: CompletedTaskActivityDTO[];
  models: ModelUsageDTO[];
  repos: RepositoryStatsDTO[];
  git: GitStatsDTO;
};

type StatsSectionsResult = StatsSections & {
  retrySection: (key: StatsSectionKey) => void;
};

type StatsScope = {
  key: string;
  generation: number;
  controller: AbortController;
  active: Set<StatsSectionKey>;
  attempts: Partial<Record<StatsSectionKey, number>>;
  timers: Partial<Record<StatsSectionKey, ReturnType<typeof setTimeout>>>;
};

const RETRY_DELAYS_MS = [2_000, 5_000] as const;

function isAbortError(error: unknown): boolean {
  return (
    (typeof DOMException !== "undefined" &&
      error instanceof DOMException &&
      error.name === "AbortError") ||
    (error instanceof Error && error.name === "AbortError")
  );
}

function errorStatus(error: unknown): number | undefined {
  if (!error || typeof error !== "object") return undefined;
  const status = (error as { status?: unknown }).status;
  return typeof status === "number" ? status : undefined;
}

function errorCode(error: unknown): string | undefined {
  if (!error || typeof error !== "object") return undefined;
  const typed = error as {
    errorCode?: unknown;
    body?: unknown;
  };
  if (typeof typed.errorCode === "string") return typed.errorCode;
  if (typed.body && typeof typed.body === "object") {
    const code = (typed.body as { code?: unknown }).code;
    if (typeof code === "string") return code;
  }
  return undefined;
}

function retryAfterMs(error: unknown): number | undefined {
  if (!error || typeof error !== "object") return undefined;
  const seconds = (error as { retryAfterSeconds?: unknown }).retryAfterSeconds;
  return typeof seconds === "number" && seconds > 0 ? seconds * 1_000 : undefined;
}

function classifySectionError(
  error: unknown,
  t: (key: string) => string,
): Pick<
  Extract<SectionStatus<unknown>, { kind: "error" }>,
  "message" | "retryable" | "retryAfterMs"
> {
  const status = errorStatus(error);
  const code = errorCode(error);
  const retryable =
    !isAbortError(error) &&
    (code === "analytics_busy" ||
      code === "persistence_unavailable" ||
      status === 429 ||
      status === 502 ||
      status === 503 ||
      status === 504 ||
      (status === undefined && !(error instanceof SyntaxError)));

  let message = t("stats:failedToLoadSection");
  if (status === 401 || status === 403) message = t("stats:statsAccessDenied");
  else if (retryable) message = t("stats:statsTemporarilyUnavailable");

  return {
    message,
    retryable,
    ...(retryAfterMs(error) !== undefined ? { retryAfterMs: retryAfterMs(error) } : {}),
  };
}

function fetchStatsSection(
  key: StatsSectionKey,
  workspaceId: string,
  options: ApiRequestOptions,
  range: RangeKey,
): Promise<unknown> {
  switch (key) {
    case "global":
      return fetchGlobalStats(workspaceId, options, range);
    case "tasks":
      return fetchTaskStats(workspaceId, options, range);
    case "daily":
      return fetchDailyActivity(workspaceId, options, range);
    case "completed":
      return fetchCompletedActivity(workspaceId, options, range);
    case "models":
      return fetchModelUsage(workspaceId, options, range);
    case "repos":
      return fetchRepositoryStats(workspaceId, options, range);
    case "git":
      return fetchGitStats(workspaceId, options, range);
  }
}

function clearRetryTimer(scope: StatsScope, key: StatsSectionKey): void {
  const timer = scope.timers[key];
  if (timer) clearTimeout(timer);
  delete scope.timers[key];
}

// eslint-disable-next-line max-lines-per-function -- one scope owns request, retry, and cancellation state
export function useStatsSections(
  workspaceId: string | undefined,
  range: RangeKey,
): StatsSectionsResult {
  const { t } = useTranslation();
  // Render-time reset: when (workspaceId, range) flips we wipe section state
  // before the effect runs so panels show skeletons while new fetches are in
  // flight, instead of stale data from the previous range. Matches the React
  // docs "reset state when a prop changes" pattern (no setState-in-effect).
  const fetchKey = workspaceId ? `${workspaceId}::${range}` : null;
  const [trackedKey, setTrackedKey] = useState<string | null>(null);
  const [sections, setSections] = useState<StatsSections>(INITIAL_SECTIONS);
  const sectionsRef = useRef(sections);
  sectionsRef.current = sections;
  const scopeRef = useRef<StatsScope | null>(null);
  const startRequestRef = useRef<(key: StatsSectionKey, resetAttempts: boolean) => void>(() => {});
  if (fetchKey !== trackedKey) {
    setTrackedKey(fetchKey);
    setSections(INITIAL_SECTIONS);
  }

  useEffect(() => {
    const previousScope = scopeRef.current;
    if (previousScope) {
      previousScope.controller.abort();
      STATS_SECTION_KEYS.forEach((key) => clearRetryTimer(previousScope, key));
    }
    startRequestRef.current = () => {};
    if (!workspaceId || !fetchKey) {
      scopeRef.current = null;
      return;
    }

    const scope: StatsScope = {
      key: fetchKey,
      generation: (previousScope?.generation ?? 0) + 1,
      controller: new AbortController(),
      active: new Set(),
      attempts: {},
      timers: {},
    };
    scopeRef.current = scope;

    const startRequest = (key: StatsSectionKey, resetAttempts = false) => {
      if (scopeRef.current !== scope || scope.controller.signal.aborted) return;
      if (scope.active.has(key)) return;
      if (resetAttempts) scope.attempts[key] = 0;
      clearRetryTimer(scope, key);
      scope.active.add(key);

      setSections((previous) => {
        const current = previous[key];
        if (current.kind !== "error") return previous;
        return {
          ...previous,
          [key]: { ...current, retrying: true },
        } as StatsSections;
      });

      const options = {
        cache: "no-store" as const,
        init: { signal: scope.controller.signal },
      };
      fetchStatsSection(key, workspaceId, options, range)
        .then((data) => {
          if (scopeRef.current !== scope || scope.controller.signal.aborted) return;
          scope.active.delete(key);
          scope.attempts[key] = 0;
          clearRetryTimer(scope, key);
          setSections(
            (previous) =>
              ({
                ...previous,
                [key]: { kind: "ready", data: data as SectionData[typeof key] },
              }) as StatsSections,
          );
        })
        .catch((error: unknown) => {
          if (
            scopeRef.current !== scope ||
            scope.controller.signal.aborted ||
            isAbortError(error)
          ) {
            return;
          }
          scope.active.delete(key);
          const classified = classifySectionError(error, t);
          setSections((previous) => {
            const current = previous[key];
            const data =
              current.kind === "ready" || current.kind === "error" ? current.data : undefined;
            return {
              ...previous,
              [key]: {
                kind: "error",
                ...classified,
                ...(data !== undefined ? { data } : {}),
              },
            } as StatsSections;
          });

          if (!classified.retryable || document.visibilityState === "hidden") return;
          const attempt = scope.attempts[key] ?? 0;
          if (attempt >= RETRY_DELAYS_MS.length) return;
          scope.attempts[key] = attempt + 1;
          const delay = Math.max(RETRY_DELAYS_MS[attempt], classified.retryAfterMs ?? 0);
          scope.timers[key] = setTimeout(() => {
            delete scope.timers[key];
            startRequest(key);
          }, delay);
        });
    };

    startRequestRef.current = startRequest;
    STATS_SECTION_KEYS.forEach((key) => startRequest(key));

    return () => {
      scope.controller.abort();
      STATS_SECTION_KEYS.forEach((key) => clearRetryTimer(scope, key));
      if (scopeRef.current === scope) {
        scopeRef.current = null;
        startRequestRef.current = () => {};
      }
    };
  }, [fetchKey, range, t, workspaceId]);

  const retrySection = useCallback(
    (key: StatsSectionKey) => {
      const scope = scopeRef.current;
      const status = sectionsRef.current[key];
      if (!scope || scope.key !== fetchKey || status.kind === "loading") return;
      if (status.kind === "error" && status.retrying) return;
      startRequestRef.current(key, true);
    },
    [fetchKey],
  );

  const recoverFailedSections = useCallback(() => {
    const scope = scopeRef.current;
    if (!scope || scope.key !== fetchKey) return;
    STATS_SECTION_KEYS.forEach((key) => {
      const status = sectionsRef.current[key];
      if (status.kind === "error" && status.retryable && !status.retrying) {
        startRequestRef.current(key, true);
      }
    });
  }, [fetchKey]);

  useForegroundRefresh(recoverFailedSections, Boolean(workspaceId), fetchKey);

  useEffect(() => {
    const onVisibilityChange = () => {
      if (document.visibilityState !== "hidden") return;
      const scope = scopeRef.current;
      if (!scope) return;
      STATS_SECTION_KEYS.forEach((key) => clearRetryTimer(scope, key));
    };
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => document.removeEventListener("visibilitychange", onVisibilityChange);
  }, [fetchKey]);

  return { ...sections, retrySection };
}

export function readyGlobal(sections: StatsSections): GlobalStatsDTO | null {
  return sections.global.kind === "ready" ? sections.global.data : null;
}

// firstError returns the message of the first errored section in object-iteration
// order (insertion order: global → tasks → daily → completed → models → repos →
// git). Callers use it only as a boolean "any section failed?" signal driving
// the header's "Failed to load stats" subtitle, so the deterministic ordering
// is fine — surface a specific section's error inside its own panel instead.
export function firstError(sections: StatsSections): string | null {
  for (const key of STATS_SECTION_KEYS) {
    const s = sections[key];
    if (s.kind === "error") return s.message;
  }
  return null;
}

// composeStatsResponse builds a full StatsResponse when every section is ready.
// Returns null otherwise — used to gate the Copy Stats button.
export function composeStatsResponse(sections: StatsSections): StatsResponse | null {
  const { global, tasks, daily, completed, models, repos, git } = sections;
  if (
    global.kind !== "ready" ||
    tasks.kind !== "ready" ||
    daily.kind !== "ready" ||
    completed.kind !== "ready" ||
    models.kind !== "ready" ||
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
    model_usage: models.data,
    repository_stats: repos.data,
    git_stats: git.data,
  };
}

export type TaskStatsSectionStatus = SectionStatus<TaskStatsDTO[]>;

// flattenTaskStats unwraps the {task_stats, has_more} envelope into the bare
// list expected by render helpers and the WorkloadSection component.
export function flattenTaskStats(status: SectionStatus<TaskStatsResponse>): TaskStatsSectionStatus {
  if (status.kind === "ready") return { kind: "ready", data: status.data.task_stats };
  if (status.kind === "error") {
    const result: Extract<TaskStatsSectionStatus, { kind: "error" }> = {
      kind: "error",
      message: status.message,
      retryable: status.retryable,
      ...(status.retryAfterMs !== undefined ? { retryAfterMs: status.retryAfterMs } : {}),
      ...(status.retrying !== undefined ? { retrying: status.retrying } : {}),
    };
    if (status.data !== undefined) result.data = status.data.task_stats;
    return result;
  }
  return status;
}
