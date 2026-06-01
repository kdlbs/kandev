"use client";

import { useLayoutEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { AgentProfile, Project, InboxItem, OfficeMeta } from "@/lib/state/slices/office/types";

export type OfficeSsrSnapshot = {
  workspaceId: string | null;
  agents: AgentProfile[];
  projects: Project[];
  inboxItems: InboxItem[];
  inboxCount: number;
  meta: OfficeMeta | null;
};

type GetInboxResponse = { items: InboxItem[]; total_count: number };

/**
 * Seeds the office TanStack Query caches from the SSR snapshot the office
 * layout fetches (agents / projects / inbox / meta for the active
 * workspace). The office sidebar + pickers read these via `useQuery`, so
 * without a seed they'd mount with an empty cache and flash until the
 * client refetch lands.
 *
 * Seed-if-absent: a live WS/refetch result is never clobbered. This is
 * the office-scoped analogue of the turns seed in `state-hydrator.tsx`
 * (the SSR→TQ bridge for Zustand-hydrated pages).
 */
export function OfficeTqSeeder({ snapshot }: { snapshot: OfficeSsrSnapshot }) {
  const queryClient = useQueryClient();

  useLayoutEffect(() => {
    const { workspaceId } = snapshot;
    if (!workspaceId) return;

    const seed = <T,>(key: readonly unknown[], value: T) => {
      if (queryClient.getQueryData(key) === undefined) {
        queryClient.setQueryData(key, value);
      }
    };

    seed(qk.office.agents(workspaceId), snapshot.agents);
    seed(["office", workspaceId, "projects"] as const, snapshot.projects);
    seed(["office", workspaceId, "inbox"] as const, {
      items: snapshot.inboxItems,
      total_count: snapshot.inboxCount,
    } satisfies GetInboxResponse);
    if (snapshot.meta) {
      seed(["office", workspaceId, "meta"] as const, snapshot.meta);
    }
  }, [snapshot, queryClient]);

  return null;
}
