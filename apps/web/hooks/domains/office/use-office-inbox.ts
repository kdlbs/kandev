"use client";

import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import type { InboxItem } from "@/lib/state/slices/office/types";

const EMPTY_ITEMS: InboxItem[] = [];

/**
 * Inbox items for the active workspace, read from TanStack Query.
 *
 * Replaces the legacy `useAppStore(s => s.office.inboxItems)` mirror read.
 */
export function useOfficeInboxItems(): InboxItem[] {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data } = useQuery({
    ...officeQueryOptions.inbox(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  return data?.items ?? EMPTY_ITEMS;
}
