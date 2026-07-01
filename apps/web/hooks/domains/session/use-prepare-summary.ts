import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";

import { useAppStore } from "@/components/state-provider";
import { prepareProgressQueryOptions } from "@/lib/query/query-options";
import { summarizePrepare, type PrepareSummary } from "@/lib/prepare/summarize";

export function usePrepareSummary(sessionId: string | null): PrepareSummary {
  const prepareQuery = useQuery(prepareProgressQueryOptions(sessionId ?? ""));
  const storePrepareState = useAppStore((state) =>
    sessionId ? (state.prepareProgress?.bySessionId?.[sessionId] ?? null) : null,
  );
  const sessionState = useAppStore((state) =>
    sessionId ? (state.taskSessions?.items?.[sessionId]?.state ?? null) : null,
  );
  const prepareState = prepareQuery.data ?? storePrepareState;
  return useMemo(() => summarizePrepare(prepareState, sessionState), [prepareState, sessionState]);
}
