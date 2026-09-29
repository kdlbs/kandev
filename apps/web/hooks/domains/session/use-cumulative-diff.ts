import { useEffect } from "react";
import type { StoreApi } from "zustand";
import { useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { getSessionReadScope } from "@/lib/state/session-read-coordinator";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { CumulativeDiff } from "@/lib/state/slices/session-runtime/types";
import { useSessionRead } from "./use-session-read";
import { t } from "@/lib/i18n";

export function invalidateCumulativeDiffCache(store: StoreApi<AppState>, envKey: string) {
  getSessionReadScope(store).invalidate("diff", envKey, 200);
}

export function useCumulativeDiff(sessionId: string | null) {
  const envKey = useAppStore((state) =>
    sessionId ? (state.environmentIdBySessionId[sessionId] ?? sessionId) : null,
  );
  const { read, snapshot, scope } = useSessionRead<CumulativeDiff | null>(
    sessionId && envKey
      ? {
          resource: "diff",
          key: JSON.stringify([sessionId, envKey]),
          environmentId: envKey,
          sessionId,
          fetch: async (isCurrent) => {
            const response = await getWebSocketClient()!.request<{
              cumulative_diff?: CumulativeDiff | null;
              ready?: boolean;
              reason?: string;
            }>("session.cumulative_diff", { session_id: sessionId });
            if (!isCurrent()) return {};
            if (response?.ready === false) {
              return response.reason === "session_terminal" ? {} : { retryAfter: 2000 };
            }
            return { data: response?.cumulative_diff ?? null };
          },
        }
      : null,
  );
  useEffect(() => read?.ensure(), [read, scope]);
  let error: string | null = null;
  if (snapshot.error) {
    error =
      snapshot.error instanceof Error
        ? snapshot.error.message
        : t("task:failedToFetchCumulativeDiff");
  }
  return {
    diff: snapshot.data ?? null,
    loading: snapshot.loading,
    error,
    refetch: read?.refetch ?? noRefetch,
  };
}

async function noRefetch() {}
