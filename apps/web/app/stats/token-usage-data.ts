"use client";

import { useEffect, useState } from "react";
import {
  fetchTokenUsage,
  type TokenUsageGroup,
  type TokenUsageQuery,
} from "@/lib/api/domains/stats-api";
import type { TokenUsageResponse } from "@/lib/types/http";
import type { RangeKey } from "./stats-utils";
import { t } from "@/lib/i18n";

export type TokenUsageLoadState =
  | { status: "no-workspace"; key: null }
  | { status: "loading"; key: string }
  | { status: "ready"; key: string; data: TokenUsageResponse }
  | { status: "error"; key: string; message: string };

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : t("stats:failedToLoadTokenUsage");
}

export type TokenUsageLoadOptions = TokenUsageQuery & {
  // Trend charts need every server page. Tables leave this false so their
  // server-side page controls remain meaningful.
  allPages?: boolean;
};

async function fetchUsage(
  workspaceId: string,
  range: RangeKey,
  group: TokenUsageGroup,
  requestOptions: RequestInit,
  queryOptions: TokenUsageLoadOptions,
): Promise<TokenUsageResponse> {
  const first = await fetchTokenUsage(
    workspaceId,
    { cache: "no-store", init: requestOptions },
    range,
    group,
    queryOptions,
  );
  if (!queryOptions.allPages || !first.has_more) return first;

  const pageSize = queryOptions.limit ?? 200;
  const { allPages: _allPages, ...pageQuery } = queryOptions;
  const rows = [...first.rows];
  let offset = queryOptions.offset ?? 0;
  let page = first;
  while (page.has_more) {
    offset += page.rows.length;
    page = await fetchTokenUsage(
      workspaceId,
      { cache: "no-store", init: requestOptions },
      range,
      group,
      { ...pageQuery, limit: pageSize, offset },
    );
    rows.push(...page.rows);
    if (page.rows.length === 0) break;
  }
  return { ...first, rows, has_more: false };
}

export function useTokenUsage(
  workspaceId: string | undefined,
  range: RangeKey,
  retryKey = 0,
  group: TokenUsageGroup = "model",
  queryOptions: TokenUsageLoadOptions = {},
): TokenUsageLoadState {
  const queryKey = JSON.stringify(queryOptions);
  const fetchKey = workspaceId
    ? `${workspaceId}::${range}::${group}::${retryKey}::${queryKey}`
    : null;
  const [state, setState] = useState<TokenUsageLoadState>({
    status: "no-workspace",
    key: null,
  });

  if (fetchKey === null && state.status !== "no-workspace") {
    setState({ status: "no-workspace", key: null });
  } else if (fetchKey !== null && state.key !== fetchKey) {
    setState({ status: "loading", key: fetchKey });
  }

  useEffect(() => {
    if (!workspaceId || !fetchKey) return;
    const controller = new AbortController();
    const requestOptions = { signal: controller.signal };

    fetchUsage(workspaceId, range, group, requestOptions, queryOptions)
      .then((data) => {
        if (!controller.signal.aborted) setState({ status: "ready", key: fetchKey, data });
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setState({ status: "error", key: fetchKey, message: errorMessage(error) });
        }
      });

    return () => controller.abort();
  }, [fetchKey, group, queryKey, range, workspaceId]);

  return state;
}
