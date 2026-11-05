"use client";

import { useQuery } from "@tanstack/react-query";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { fetchSandboxReport } from "@/lib/api/domains/sandbox-reports-api";
import { getBackendConfig } from "@/lib/config";
import { sandboxApprovalContext } from "@/lib/sandbox-approval-context";
import { isSandboxReportBound } from "@/lib/sandbox-report";

export function useSandboxReport(sessionId: string) {
  const backend = getBackendConfig().apiBaseUrl;
  const context = useAppStore(
    useShallow((state) => sandboxApprovalContext(state, sessionId, backend)),
  );
  const attemptId = context.snapshot?.attempt_id ?? "";
  const query = useQuery({
    queryKey: ["sandbox-report", sessionId, attemptId, context.key],
    queryFn: ({ signal }) => fetchSandboxReport(sessionId, attemptId, { init: { signal } }),
    enabled: context.enabled,
    retry: false,
    gcTime: 0,
  });
  const bound = query.data && isSandboxReportBound(query.data, context.snapshot);
  return {
    report: context.enabled && bound ? query.data! : null,
    isPending: context.enabled && query.isPending,
    isError: context.enabled && (query.isError || Boolean(query.data && !bound)),
    isFetching: query.isFetching,
    canRefresh: context.enabled && !query.isFetching,
    refresh: async () => {
      if (context.enabled) await query.refetch();
    },
  };
}
