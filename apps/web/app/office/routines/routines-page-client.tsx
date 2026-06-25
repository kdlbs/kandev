"use client";

import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { qk } from "@/lib/query/keys";
import type { Routine } from "@/lib/state/slices/office/types";
import { RoutinesContent } from "./routines-content";

type RoutinesPageClientProps = {
  initialRoutines: Routine[];
};

export function RoutinesPageClient({ initialRoutines }: RoutinesPageClientProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const setRoutines = useAppStore((s) => s.setRoutines);
  const queryClient = useQueryClient();

  useEffect(() => {
    if (initialRoutines.length > 0) {
      if (workspaceId) {
        queryClient.setQueryData(qk.office.routines(workspaceId), { routines: initialRoutines });
      }
      setRoutines(initialRoutines);
    }
  }, [initialRoutines, queryClient, setRoutines, workspaceId]);

  useOfficeRefetch("routines", () => {
    if (!workspaceId) return;
    void queryClient.invalidateQueries({ queryKey: qk.office.routines(workspaceId) });
    void queryClient.invalidateQueries({ queryKey: qk.office.routineRuns(workspaceId) });
  });

  return <RoutinesContent />;
}
