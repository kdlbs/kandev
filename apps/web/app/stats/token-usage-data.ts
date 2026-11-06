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
  | { status: "loading"; key: string; scopeKey?: string }
  | {
      status: "ready";
      key: string;
      scopeKey?: string;
      data: TokenUsageResponse;
      refreshing?: boolean;
      error?: string;
    }
  | { status: "error"; key: string; scopeKey?: string; message: string };

export function isUsagePreparing(data: TokenUsageResponse): boolean {
  return (
    !data.projection_generation &&
    (data.projection_status === "preparing" || data.projection_pending === true)
  );
}

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
  if (!queryOptions.allPages || isUsagePreparing(first)) return first;
  const expectedRows = Math.max(0, first.total_rows - (queryOptions.offset ?? 0));
  if (!first.has_more) {
    requireCompleteUsageRows(first.rows.length, expectedRows);
    return first;
  }

  const pageSize = queryOptions.limit ?? 200;
  const { allPages: _allPages, ...pageQuery } = queryOptions;
  const pinnedQuery = usagePublicationQuery(first, pageQuery);
  const rows = [...first.rows];
  const keys = new Set(rows.map((row) => row.key));
  let pending = first.projection_pending;
  let offset = queryOptions.offset ?? 0;
  let page = first;
  while (page.has_more) {
    requestOptions.signal?.throwIfAborted();
    if (page.rows.length === 0) incompleteUsagePage();
    offset += page.rows.length;
    page = await fetchTokenUsage(
      workspaceId,
      { cache: "no-store", init: requestOptions },
      range,
      group,
      {
        ...pinnedQuery,
        limit: pageSize,
        offset,
      },
    );
    validateUsagePage(first.projection_generation, page, keys);
    for (const row of page.rows) keys.add(row.key);
    rows.push(...page.rows);
    pending ||= page.projection_pending;
  }
  requireCompleteUsageRows(rows.length, expectedRows);
  return { ...first, rows, has_more: false, ...(pending && { projection_pending: true }) };
}

function usagePublicationQuery(first: TokenUsageResponse, query: TokenUsageQuery): TokenUsageQuery {
  return {
    ...query,
    ...(first.range_start && { start: first.range_start }),
    ...(first.range_end && { end: first.range_end }),
    ...(first.projection_generation && { projectionGeneration: first.projection_generation }),
  };
}

function validateUsagePage(
  generation: string | undefined,
  page: TokenUsageResponse,
  keys: Set<string>,
): void {
  if (page.projection_generation !== generation || isUsagePreparing(page)) {
    throw new Error(t("stats:usageGenerationChanged"));
  }
  if (page.rows.length === 0 || page.rows.some((row) => keys.has(row.key))) incompleteUsagePage();
}

function requireCompleteUsageRows(actual: number, expected: number): void {
  if (actual !== expected) incompleteUsagePage();
}

function incompleteUsagePage(): never {
  throw new Error(t("stats:usagePagesIncomplete"));
}

export function useTokenUsage(
  workspaceId: string | undefined,
  range: RangeKey,
  retryKey = 0,
  group: TokenUsageGroup = "model",
  queryOptions: TokenUsageLoadOptions = {},
): TokenUsageLoadState {
  const queryKey = JSON.stringify(queryOptions);
  const scopeKey = workspaceId ? JSON.stringify([workspaceId, range, group, queryKey]) : null;
  const fetchKey = scopeKey ? `${scopeKey}::${retryKey}` : null;
  const [state, setState] = useState<TokenUsageLoadState>({
    status: "no-workspace",
    key: null,
  });

  if (fetchKey === null && state.status !== "no-workspace") {
    setState({ status: "no-workspace", key: null });
  } else if (fetchKey !== null && scopeKey !== null && state.key !== fetchKey) {
    setState(
      state.status === "ready" && state.scopeKey === scopeKey
        ? { ...state, key: fetchKey, refreshing: true, error: undefined }
        : { status: "loading", key: fetchKey, scopeKey },
    );
  }

  useEffect(() => {
    if (!workspaceId || !fetchKey || !scopeKey) return;
    const controller = new AbortController();
    const requestOptions = { signal: controller.signal };

    fetchUsage(workspaceId, range, group, requestOptions, queryOptions)
      .then((data) => {
        if (controller.signal.aborted) return;
        setState((current) => {
          if (current.key !== fetchKey) return current;
          const saved =
            current.status === "ready" && !isUsagePreparing(current.data) ? current.data : null;
          const published =
            saved && isUsagePreparing(data)
              ? { ...saved, projection_status: data.projection_status, projection_pending: true }
              : data;
          return { status: "ready", key: fetchKey, scopeKey, data: published };
        });
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setState((current) => {
            if (current.key !== fetchKey) return current;
            const status = (error as { status?: number } | null)?.status;
            const denied = status === 401 || status === 403 || status === 404;
            const message = denied ? t("stats:statsAccessDenied") : errorMessage(error);
            return current.status === "ready" && !isUsagePreparing(current.data) && !denied
              ? { ...current, refreshing: false, error: message }
              : { status: "error", key: fetchKey, scopeKey, message };
          });
        }
      });

    return () => controller.abort();
  }, [fetchKey, group, queryKey, range, scopeKey, workspaceId]);

  return state;
}
