"use client";

import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import type { Message } from "@/lib/types/http";
import { sessionMessagesLatestQueryOptions } from "@/lib/query/query-options";

export function useSidebarMessagesBySession(
  sessionIds: string[],
): Record<string, Message[] | undefined> {
  const queries = useQueries({ queries: sessionIds.map(sessionMessagesLatestQueryOptions) });

  return useMemo<Record<string, Message[] | undefined>>(
    () => Object.fromEntries(sessionIds.map((id, i) => [id, queries[i]?.data?.messages])),
    [queries, sessionIds],
  );
}
