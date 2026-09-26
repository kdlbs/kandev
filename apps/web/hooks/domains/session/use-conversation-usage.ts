"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  getSessionUsageTotals,
  getSessionUsageTurn,
  listSessionUsageTurns,
} from "@/lib/api/domains/conversation-usage-api";
import type { SessionUsageTotals, UsageTurn } from "@/lib/types/conversation-usage";
import { latestUsageTurn } from "@/lib/utils/conversation-usage";

type UsageSummaryState = {
  key: string;
  totals: SessionUsageTotals | null;
  latestTurn: UsageTurn | null;
  loading: boolean;
  error: boolean;
};

type UsageDetailState = {
  key: string;
  turnId: string;
  turn: UsageTurn | null;
  loading: boolean;
  error: boolean;
};

const emptySummary = (key: string): UsageSummaryState => ({
  key,
  totals: null,
  latestTurn: null,
  loading: false,
  error: false,
});

export function useConversationUsage(taskId: string | null, sessionId: string | null) {
  const connectionStatus = useAppStore((state) => state.connection.status);
  const invalidation = useAppStore((state) =>
    sessionId ? (state.usageInvalidation.bySessionId[sessionId] ?? 0) : 0,
  );
  const [summary, setSummary] = useState<UsageSummaryState>(() => emptySummary(""));
  const [detail, setDetail] = useState<UsageDetailState | null>(null);
  const summaryRequest = useRef(0);
  const detailRequest = useRef(0);
  const key = taskId && sessionId ? `${taskId}\u0000${sessionId}` : "";

  const refresh = useCallback(async () => {
    if (!taskId || !sessionId) return;
    const request = ++summaryRequest.current;
    const requestKey = `${taskId}\u0000${sessionId}`;
    setSummary((current) =>
      current.key === requestKey
        ? { ...current, loading: true, error: false }
        : { ...emptySummary(requestKey), loading: true },
    );
    try {
      const [totals, page] = await Promise.all([
        getSessionUsageTotals(taskId, sessionId),
        listSessionUsageTurns(taskId, sessionId),
      ]);
      if (request !== summaryRequest.current) return;
      setSummary({
        key: requestKey,
        totals,
        latestTurn: latestUsageTurn(page.turns) ?? null,
        loading: false,
        error: false,
      });
    } catch {
      if (request !== summaryRequest.current) return;
      setSummary((current) =>
        current.key === requestKey
          ? { ...current, loading: false, error: true }
          : { ...emptySummary(requestKey), error: true },
      );
    }
  }, [taskId, sessionId]);

  useEffect(() => {
    if (!taskId || !sessionId || connectionStatus !== "connected") return;
    void refresh();
    return () => {
      summaryRequest.current += 1;
    };
  }, [connectionStatus, invalidation, refresh, sessionId, taskId]);

  const loadTurnDetail = useCallback(
    async (turnId: string) => {
      if (!taskId || !sessionId) return;
      const request = ++detailRequest.current;
      const requestKey = `${taskId}\u0000${sessionId}`;
      setDetail({ key: requestKey, turnId, turn: null, loading: true, error: false });
      try {
        const turn = await getSessionUsageTurn(taskId, sessionId, turnId);
        if (request !== detailRequest.current) return;
        setDetail({ key: requestKey, turnId, turn, loading: false, error: false });
      } catch {
        if (request !== detailRequest.current) return;
        setDetail({ key: requestKey, turnId, turn: null, loading: false, error: true });
      }
    },
    [taskId, sessionId],
  );

  const currentSummary = summary.key === key ? summary : emptySummary(key);
  const currentDetail = detail?.key === key ? detail : null;
  return {
    ...currentSummary,
    detail: currentDetail,
    refresh,
    loadTurnDetail,
  };
}
