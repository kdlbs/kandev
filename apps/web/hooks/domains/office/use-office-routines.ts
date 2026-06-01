"use client";

import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import type { Routine } from "@/lib/state/slices/office/types";

const EMPTY_ROUTINES: Routine[] = [];

/**
 * Routines for the active workspace, read from TanStack Query.
 *
 * Replaces the legacy `useAppStore(s => s.office.routines)` mirror read.
 */
export function useOfficeRoutines(): Routine[] {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data } = useQuery({
    ...officeQueryOptions.routines(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  return data ?? EMPTY_ROUTINES;
}
