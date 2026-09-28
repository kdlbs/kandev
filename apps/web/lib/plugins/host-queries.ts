"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import { fetchTaskUsageTotals } from "@/lib/api/domains/task-usage-api";
import type {
  PluginQueryState,
  PluginTaskStatusSnapshot,
  PluginTaskUsageSnapshot,
} from "@kandev/plugin-sdk";

type QueryValue = PluginTaskStatusSnapshot | PluginTaskUsageSnapshot;
type QueryLoader<Value extends QueryValue> = () => Promise<Value>;

function queryError(error: unknown): string {
  // i18n-exempt: query errors are machine/API diagnostics; host UI copy is localized at render time.
  return error instanceof Error ? error.message : "Unable to load task data";
}

function useHostQuery<Value extends QueryValue>(
  key: string | null,
  loader: QueryLoader<Value>,
): PluginQueryState<Value> {
  const [data, setData] = useState<Value | null>(null);
  const [loading, setLoading] = useState(Boolean(key));
  const [error, setError] = useState<string | null>(null);
  const generation = useRef(0);
  const inFlight = useRef<number | null>(null);

  const refetch = useCallback(async () => {
    if (!key || inFlight.current === generation.current) return;
    const requestGeneration = ++generation.current;
    inFlight.current = requestGeneration;
    setLoading(true);
    setError(null);
    try {
      const next = await loader();
      if (requestGeneration === generation.current) setData(next);
    } catch (cause) {
      if (requestGeneration === generation.current) setError(queryError(cause));
    } finally {
      if (inFlight.current === requestGeneration) inFlight.current = null;
      if (requestGeneration === generation.current) setLoading(false);
    }
  }, [key, loader]);

  useEffect(() => {
    if (!key) {
      generation.current += 1;
      setData(null);
      setError(null);
      setLoading(false);
      return;
    }
    void refetch();
    const timer = window.setInterval(() => void refetch(), 15_000);
    return () => {
      window.clearInterval(timer);
      generation.current += 1;
    };
  }, [key, refetch]);

  return { data, loading, error, refetch };
}

export function projectPluginTaskStatus(
  task: Awaited<ReturnType<typeof fetchTask>>,
): PluginTaskStatusSnapshot {
  const summary = task.status_summary;
  return {
    taskId: task.id,
    title: task.title,
    state: task.state,
    workflowStepId: task.workflow_step_id,
    statusSummary: summary
      ? {
          revision: summary.revision,
          foregroundActivity: summary.foreground_activity,
          activeSubagentCount: summary.active_subagent_count,
          queuedPromptCount: summary.queued_prompt_count,
          completionGate: summary.completion_gate
            ? {
                verifiedCount: summary.completion_gate.verified_count,
                criteriaCount: summary.completion_gate.criteria_count,
                blocked: summary.completion_gate.blocked,
              }
            : undefined,
          activeError: summary.active_error
            ? {
                preview: summary.active_error.preview,
                category: summary.active_error.category,
              }
            : undefined,
        }
      : null,
  };
}

export function hasPluginTaskUsage(usage: PluginTaskUsageSnapshot): boolean {
  return (
    usage.eventCount > 0 ||
    usage.estimatedEventCount > 0 ||
    usage.unpricedEventCount > 0 ||
    usage.tokensTotal > 0 ||
    usage.costSubcents > 0
  );
}

export function usePluginTaskStatus(
  taskId: string | null,
): PluginQueryState<PluginTaskStatusSnapshot> {
  const load = useCallback(async () => projectPluginTaskStatus(await fetchTask(taskId!)), [taskId]);
  return useHostQuery(taskId, load);
}

export function usePluginTaskUsage(
  taskId: string | null,
  sessionId?: string | null,
): PluginQueryState<PluginTaskUsageSnapshot> {
  const key = taskId ? `${taskId}:${sessionId ?? "task"}` : null;
  const load = useCallback(async () => {
    const totals = await fetchTaskUsageTotals(taskId!, sessionId);
    return {
      taskId: taskId!,
      sessionId: sessionId ?? undefined,
      tokensIn: totals.tokens_in,
      tokensCachedRead: totals.tokens_cached_read,
      tokensCachedWrite: totals.tokens_cached_write,
      tokensOut: totals.tokens_out,
      tokensThought: totals.tokens_thought,
      tokensTotal: totals.tokens_total,
      costSubcents: totals.cost_subcents,
      eventCount: totals.event_count,
      estimatedEventCount: totals.estimated_event_count,
      unpricedEventCount: totals.unpriced_event_count,
      outputTokensComplete: totals.output_tokens_complete,
      firstEventAt: totals.first_event_at,
      lastEventAt: totals.last_event_at,
    } satisfies PluginTaskUsageSnapshot;
  }, [taskId, sessionId]);
  return useHostQuery(key, load);
}
