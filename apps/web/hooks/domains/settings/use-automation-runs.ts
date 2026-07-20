"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  listAutomationRuns,
  deleteAutomationRun,
  deleteAllAutomationRuns,
} from "@/lib/api/domains/automation-api";
import { qk } from "@/lib/query/keys";
import { automationRunsQueryOptions } from "@/lib/query/query-options/automations";
import type { AutomationRun } from "@/lib/types/automation";

const EMPTY_RUNS: AutomationRun[] = [];

export function useAutomationRuns(automationId: string | null, workspaceId: string) {
  const queryClient = useQueryClient();
  const queryKey = qk.automations.runs(automationId);
  const query = useQuery({
    ...automationRunsQueryOptions(automationId ?? ""),
    enabled: Boolean(automationId),
  });
  const runs = query.data ?? EMPTY_RUNS;
  const refetch = query.refetch;

  const refresh = useCallback(() => {
    if (!automationId) return Promise.resolve();
    return refetch();
  }, [automationId, refetch]);

  const deleteRun = useCallback(
    (runId: string) => {
      if (!automationId) return;
      const previousRuns = queryClient.getQueryData<AutomationRun[]>(queryKey) ?? runs;
      const deletedRun = previousRuns.find((run) => run.id === runId);
      queryClient.setQueryData<AutomationRun[]>(queryKey, (current = []) =>
        current.filter((run) => run.id !== runId),
      );
      deleteAutomationRun(runId, workspaceId)
        .then(() => {
          queryClient.setQueryData<AutomationRun[]>(queryKey, (current = []) =>
            current.filter((run) => run.id !== runId),
          );
        })
        .catch((err: unknown) => {
          const msg = err instanceof Error ? err.message : "Failed to delete run";
          toast.error(msg);
          listAutomationRuns(automationId)
            .then((result) => queryClient.setQueryData(queryKey, result ?? []))
            .catch(() => {
              if (deletedRun) {
                queryClient.setQueryData<AutomationRun[]>(queryKey, (current = []) =>
                  current.some((run) => run.id === deletedRun.id)
                    ? current
                    : [...current, deletedRun],
                );
              }
              toast.error("Could not refresh runs — restored from local cache");
            });
        });
    },
    [automationId, queryClient, queryKey, runs, workspaceId],
  );

  const deleteAllRuns = useCallback(() => {
    if (!automationId) return;
    const previousRuns = queryClient.getQueryData<AutomationRun[]>(queryKey) ?? runs;
    queryClient.setQueryData<AutomationRun[]>(queryKey, []);
    deleteAllAutomationRuns(automationId, workspaceId)
      .then(() => {
        queryClient.setQueryData<AutomationRun[]>(queryKey, []);
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : "Failed to delete runs";
        toast.error(msg);
        listAutomationRuns(automationId)
          .then((result) => queryClient.setQueryData(queryKey, result ?? []))
          .catch(() => {
            queryClient.setQueryData(queryKey, previousRuns);
            toast.error("Could not refresh runs — restored from local cache");
          });
      });
  }, [automationId, queryClient, queryKey, runs, workspaceId]);

  return { runs, loading: query.isFetching && !query.isSuccess, refresh, deleteRun, deleteAllRuns };
}
