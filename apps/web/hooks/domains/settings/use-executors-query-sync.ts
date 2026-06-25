import { useCallback } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { useSettingsData } from "./use-settings-data";

type ExecutorsQueryData = { executors: Executor[]; total?: number };

function writeExecutors(queryClient: QueryClient, nextExecutors: Executor[]) {
  queryClient.setQueryData<ExecutorsQueryData>(qk.settings.executors(), (previous) => ({
    ...(previous ?? {}),
    executors: nextExecutors,
    total: previous?.total ?? nextExecutors.length,
  }));
  queryClient.invalidateQueries({ queryKey: qk.settings.allExecutorProfiles() });
}

export function upsertExecutorInList(executors: Executor[], next: Executor): Executor[] {
  const found = executors.some((executor) => executor.id === next.id);
  return found
    ? executors.map((executor) => (executor.id === next.id ? { ...executor, ...next } : executor))
    : [...executors, next];
}

export function removeExecutorFromList(executors: Executor[], executorId: string): Executor[] {
  return executors.filter((executor) => executor.id !== executorId);
}

export function upsertExecutorProfileInList(
  executors: Executor[],
  executorId: string,
  profile: ExecutorProfile,
): Executor[] {
  return executors.map((executor) =>
    executor.id === executorId
      ? {
          ...executor,
          profiles: upsertProfile(executor.profiles ?? [], profile),
        }
      : executor,
  );
}

export function removeExecutorProfileFromList(
  executors: Executor[],
  executorId: string,
  profileId: string,
): Executor[] {
  return executors.map((executor) =>
    executor.id === executorId
      ? {
          ...executor,
          profiles: (executor.profiles ?? []).filter((profile) => profile.id !== profileId),
        }
      : executor,
  );
}

function upsertProfile(profiles: ExecutorProfile[], next: ExecutorProfile): ExecutorProfile[] {
  const found = profiles.some((profile) => profile.id === next.id);
  return found
    ? profiles.map((profile) => (profile.id === next.id ? next : profile))
    : [...profiles, next];
}

export function useExecutorsQuerySync() {
  const queryClient = useQueryClient();
  const { executors } = useSettingsData(true);

  const setExecutors = useCallback(
    (nextExecutors: Executor[]) => {
      writeExecutors(queryClient, nextExecutors);
    },
    [queryClient],
  );

  const upsertExecutor = useCallback(
    (executor: Executor) => {
      queryClient.setQueryData(qk.settings.executor(executor.id), executor);
      writeExecutors(queryClient, upsertExecutorInList(executors, executor));
    },
    [executors, queryClient],
  );

  const removeExecutor = useCallback(
    (executorId: string) => {
      writeExecutors(queryClient, removeExecutorFromList(executors, executorId));
      queryClient.removeQueries({ queryKey: qk.settings.executor(executorId) });
    },
    [executors, queryClient],
  );

  const upsertExecutorProfile = useCallback(
    (executorId: string, profile: ExecutorProfile) => {
      writeExecutors(queryClient, upsertExecutorProfileInList(executors, executorId, profile));
      queryClient.invalidateQueries({ queryKey: qk.settings.executorProfiles(executorId) });
    },
    [executors, queryClient],
  );

  const removeExecutorProfile = useCallback(
    (executorId: string, profileId: string) => {
      writeExecutors(queryClient, removeExecutorProfileFromList(executors, executorId, profileId));
      queryClient.invalidateQueries({ queryKey: qk.settings.executorProfiles(executorId) });
    },
    [executors, queryClient],
  );

  return {
    executors,
    removeExecutor,
    removeExecutorProfile,
    setExecutors,
    upsertExecutor,
    upsertExecutorProfile,
  };
}
