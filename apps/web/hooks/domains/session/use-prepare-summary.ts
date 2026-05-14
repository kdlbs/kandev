import { useMemo } from "react";

import { useAppStore } from "@/components/state-provider";
import { summarizePrepare, type PrepareSummary } from "@/lib/prepare/summarize";

export function usePrepareSummary(sessionId: string | null): PrepareSummary {
  const prepareState = useAppStore((state) =>
    sessionId ? (state.prepareProgress?.bySessionId?.[sessionId] ?? null) : null,
  );
  const sessionState = useAppStore((state) =>
    sessionId ? (state.taskSessions?.items?.[sessionId]?.state ?? null) : null,
  );
  return useMemo(() => summarizePrepare(prepareState, sessionState), [prepareState, sessionState]);
}
