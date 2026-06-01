"use client";

import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import type { Routine } from "@/lib/state/slices/office/types";
import { RoutinesContent } from "./routines-content";

type RoutinesPageClientProps = {
  initialRoutines: Routine[];
};

export function RoutinesPageClient({ initialRoutines }: RoutinesPageClientProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const qc = useQueryClient();

  // Seed the TQ routines cache from the SSR snapshot so the first paint
  // isn't empty. Seed-if-absent: never clobber a live client result, and
  // let the office WS bridge keep the cache fresh thereafter.
  useEffect(() => {
    if (!workspaceId || initialRoutines.length === 0) return;
    const key = qk.office.routines(workspaceId);
    if (qc.getQueryData(key) === undefined) {
      qc.setQueryData(key, initialRoutines);
    }
  }, [workspaceId, initialRoutines, qc]);

  return <RoutinesContent />;
}
