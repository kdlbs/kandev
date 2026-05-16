"use client";

import { useCallback, useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { listRoutines } from "@/lib/api/domains/office-api";
import type { Routine } from "@/lib/state/slices/office/types";
import { RoutinesContent } from "./routines-content";

type RoutinesPageClientProps = {
  initialRoutines: Routine[];
};

export function RoutinesPageClient({ initialRoutines }: RoutinesPageClientProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const setRoutines = useAppStore((s) => s.setRoutines);

  useEffect(() => {
    if (initialRoutines.length > 0) {
      setRoutines(initialRoutines);
    }
  }, [initialRoutines, setRoutines]);

  const refetchRoutines = useCallback(async () => {
    if (!workspaceId) return;
    const res = await listRoutines(workspaceId).catch(() => ({ routines: [] as Routine[] }));
    setRoutines(res.routines ?? []);
  }, [workspaceId, setRoutines]);

  useOfficeRefetch("routines", refetchRoutines);

  return <RoutinesContent />;
}
