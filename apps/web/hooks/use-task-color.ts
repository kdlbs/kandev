"use client";

import { useCallback, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useTranslation } from "react-i18next";
import { type TaskColor } from "@/lib/task-colors";
import { getTaskColorMutation, type TaskColorResult } from "./task-color-mutation";

export function useTaskColor(taskId: string | undefined): TaskColor | null {
  return useAppStore((state) => {
    if (!taskId) return null;
    return state.userSettings.sidebarTaskColors[taskId] ?? null;
  });
}

function useColorMutation() {
  const store = useAppStoreApi();
  const { toast } = useToast();
  const { t } = useTranslation();
  return useCallback(
    async (ids: string[], color: TaskColor | null) => {
      const result = await getTaskColorMutation(store)(ids, color);
      if (result.saved < result.total) {
        const description =
          result.saved === 0
            ? t("task:manualColorSaveError")
            : t("task:bulkColorPartialError", { saved: result.saved, total: result.total });
        toast({ variant: "error", description });
      }
      return result;
    },
    [store, t, toast],
  );
}

export function useSetTaskColor(): (taskId: string, color: TaskColor | null) => void {
  const mutate = useColorMutation();
  return useCallback(
    (taskId, color) => {
      void mutate([taskId], color);
    },
    [mutate],
  );
}

export function useSetTaskColors() {
  const mutate = useColorMutation();
  const [isPending, setPending] = useState(false);
  const inFlight = useRef<Promise<TaskColorResult> | null>(null);
  const setColors = useCallback(
    (ids: string[], color: TaskColor | null) => {
      if (inFlight.current) return inFlight.current;
      setPending(true);
      const promise = mutate([...ids], color).finally(() => {
        inFlight.current = null;
        setPending(false);
      });
      inFlight.current = promise;
      return promise;
    },
    [mutate],
  );
  return { setColors, isPending };
}
