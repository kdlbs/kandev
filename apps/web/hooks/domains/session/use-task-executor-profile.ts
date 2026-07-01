import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { fetchTaskEnvironment } from "@/lib/api/domains/task-environment-api";
import type { ExecutorProfile } from "@/lib/types/http";

/** Resolves the executor profile bound to a task's environment. */
export function useTaskExecutorProfile(taskId: string, enabled = true): ExecutorProfile | null {
  const { executors } = useSettingsData(enabled);
  const executorsFingerprint = useMemo(
    () =>
      executors
        .map(
          (executor) =>
            `${executor.id}|${executor.type}|${(executor.profiles ?? []).map((profile) => profile.id).join(",")}`,
        )
        .join(";"),
    [executors],
  );
  const executorsRef = useRef(executors);
  const [profile, setProfile] = useState<ExecutorProfile | null>(null);
  useLayoutEffect(() => {
    executorsRef.current = executors;
  }, [executors]);

  useEffect(() => {
    if (!enabled || !taskId) return;
    let active = true;
    void fetchTaskEnvironment(taskId)
      .then((env) => {
        if (!active || !env) return;
        let foundProfile: ExecutorProfile | undefined;
        for (const executor of executorsRef.current) {
          const match = (executor.profiles ?? []).find((p) => p.id === env.executor_profile_id);
          if (match) {
            foundProfile = {
              ...match,
              executor_type: match.executor_type ?? executor.type,
              executor_name: match.executor_name ?? executor.name,
            };
            break;
          }
        }
        setProfile(foundProfile ?? null);
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [enabled, taskId, executorsFingerprint]);

  if (!enabled || !taskId) return null;
  return profile;
}
