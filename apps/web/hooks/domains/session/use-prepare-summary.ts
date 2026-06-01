import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";

import { prepareProgressQueryOptions } from "@/lib/query/query-options/session-runtime";
import { useTaskSessionById } from "@/hooks/domains/session/use-task-session-by-id";
import { summarizePrepare, type PrepareSummary } from "@/lib/prepare/summarize";

export function usePrepareSummary(sessionId: string | null): PrepareSummary {
  // Observe-only read of the prepare-progress TQ cache (populated by the
  // session-runtime bridge live + the SSR seed). `enabled: false` because this
  // hook can be called for a null/non-active session; the cache is fed
  // elsewhere, never fetched here.
  const prepareState =
    useQuery({
      ...prepareProgressQueryOptions(sessionId ?? ""),
      enabled: false,
    }).data ?? null;
  const session = useTaskSessionById(sessionId);
  const sessionState = session?.state ?? null;
  return useMemo(() => summarizePrepare(prepareState, sessionState), [prepareState, sessionState]);
}
