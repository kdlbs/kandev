import { useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchTaskEnvironment } from "@/lib/api/domains/task-environment-api";
import type { ExecutorProfile } from "@/lib/types/http";

/** Resolves the executor profile bound to a task's environment. */
export function useTaskExecutorProfile(taskId: string, enabled = true): ExecutorProfile | null {
  const executors = useAppStore((state) => state.executors.items);
  const [profile, setProfile] = useState<ExecutorProfile | null>(null);

  useEffect(() => {
    if (!enabled || !taskId) return;
    let active = true;
    void fetchTaskEnvironment(taskId)
      .then((env) => {
        if (!active || !env) return;
        for (const executor of executors) {
          const match = (executor.profiles ?? []).find((p) => p.id === env.executor_profile_id);
          if (match) {
            setProfile({
              ...match,
              executor_type: match.executor_type ?? executor.type,
              executor_name: match.executor_name ?? executor.name,
            });
            return;
          }
        }
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [enabled, taskId, executors]);

  if (!enabled || !taskId) return null;
  return profile;
}
