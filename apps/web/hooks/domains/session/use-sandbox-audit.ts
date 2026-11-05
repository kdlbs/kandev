"use client";

import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { generateUUID } from "@/lib/uuid";
import { getBackendConfig } from "@/lib/config";
import { sandboxApprovalContext } from "@/lib/sandbox-approval-context";
import { isSandboxReportBound } from "@/lib/sandbox-report";
import { auditReportBinding, isAuditTrustBound, isAuditViewBound } from "@/lib/sandbox-audit";
import {
  fetchSandboxAuditTrust,
  fetchSandboxAuditHistory,
  fetchSandboxAuditVerification,
  verifySandboxAudit,
  saveSandboxAuditTrust,
  type SandboxAuditTrust,
  type SandboxAuditTrustRequest,
  type SandboxAuditView,
  type SandboxAuditPage,
} from "@/lib/api/domains/sandbox-audit-api";
import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";

const INVALID_AUDIT_BINDING = Symbol();

type AuditReadScope = {
  sessionId: string;
  backend: string;
  contextKey: string;
  binding: ReturnType<typeof auditReportBinding>;
  enabled: boolean;
};

function isPendingAudit(view?: SandboxAuditView): boolean {
  return view?.verification.state === "pending" || view?.verification.state === "running";
}

function useAuditPendingJob(scope: AuditReadScope, history: SandboxAuditView[]) {
  const activeId = history.find(isPendingAudit)?.verification.id ?? "";
  return useQuery({
    queryKey: ["sandbox-audit-job", scope, activeId],
    queryFn: async ({ signal }) => {
      const view = await fetchSandboxAuditVerification(
        scope.sessionId,
        scope.binding.attempt_id,
        activeId,
        { baseUrl: scope.backend, init: { signal } },
      );
      if (!isAuditViewBound(view, scope.binding) || view.verification.id !== activeId)
        throw INVALID_AUDIT_BINDING;
      return view;
    },
    enabled: scope.enabled && Boolean(activeId),
    retry: false,
    gcTime: 0,
    refetchInterval: (query) =>
      query.state.status === "success" && isPendingAudit(query.state.data) ? 1000 : false,
  });
}

function useAuditReads(sessionId: string, report: SandboxReport) {
  const backend = getBackendConfig().apiBaseUrl;
  const context = useAppStore(
    useShallow((state) => sandboxApprovalContext(state, sessionId, backend)),
  );
  const enabled = context.enabled && isSandboxReportBound(report, context.snapshot);
  const binding = auditReportBinding(report);
  const scope = { sessionId, backend, contextKey: context.key, binding, enabled };
  const historyKey = ["sandbox-audit-history", sessionId, backend, context.key, binding];
  const trustKey = ["sandbox-audit-trust", sessionId, backend, context.key, binding];
  const trustQuery = useQuery({
    queryKey: trustKey,
    queryFn: async ({ signal }) => {
      const value = await fetchSandboxAuditTrust(sessionId, binding.attempt_id, {
        baseUrl: backend,
        init: { signal },
      });
      if (!isAuditTrustBound(value, binding)) throw INVALID_AUDIT_BINDING;
      return value;
    },
    enabled,
    retry: false,
    gcTime: 0,
  });
  const historyQuery = useInfiniteQuery({
    queryKey: historyKey,
    queryFn: async ({ signal, pageParam }) => {
      const page = await fetchSandboxAuditHistory(sessionId, binding.attempt_id, pageParam, {
        baseUrl: backend,
        init: { signal },
      });
      if (
        !Array.isArray(page?.verifications) ||
        page.verifications.length > 20 ||
        !page.verifications.every((view) => isAuditViewBound(view, binding))
      )
        throw INVALID_AUDIT_BINDING;
      return page;
    },
    enabled,
    retry: false,
    gcTime: 0,
    initialPageParam: "",
    getNextPageParam: (page) => page.next_cursor || undefined,
  });
  const trust = enabled ? (trustQuery.data ?? null) : null;
  const captured = enabled
    ? (historyQuery.data?.pages.flatMap((page) => page.verifications) ?? [])
    : [];
  const jobQuery = useAuditPendingJob(scope, captured);
  const history = captured.map((view) =>
    jobQuery.data?.verification.id === view.verification.id ? jobQuery.data : view,
  );
  const active = history.some(isPendingAudit);
  return {
    backend,
    context,
    enabled,
    trust,
    history,
    active,
    trustQuery,
    historyQuery,
    jobQuery,
    historyKey,
    trustKey,
  };
}

export function useSandboxAudit(sessionId: string, report: SandboxReport) {
  const reads = useAuditReads(sessionId, report);
  const action = useAuditActions(sessionId, report, reads, verificationAvailable(reads, report));
  return {
    canLoadMore: Boolean(
      reads.enabled &&
      reads.historyQuery.hasNextPage &&
      !reads.historyQuery.isFetching &&
      !reads.historyQuery.isError &&
      !action.isBusy,
    ),
    isLoadingMore: reads.historyQuery.isFetchingNextPage,
    loadMore: async () => {
      if (
        !reads.enabled ||
        !reads.historyQuery.hasNextPage ||
        reads.historyQuery.isError ||
        action.isBusy
      )
        return false;
      const result = await reads.historyQuery.fetchNextPage({ cancelRefetch: false });
      return !result.isError;
    },
    canRefresh: reads.enabled && !reads.historyQuery.isFetching && !reads.trustQuery.isFetching,
    refresh: async () => {
      if (!reads.enabled) return;
      await Promise.all([
        reads.trustQuery.refetch(),
        reads.historyQuery.refetch(),
        reads.active ? reads.jobQuery.refetch({ cancelRefetch: false }) : Promise.resolve(),
      ]);
    },
    ...action,
    trust: reads.trust,
    currentTrust: reads.enabled && !reads.trustQuery.isError ? reads.trust : null,
    operator: reads.context.operator,
    history: reads.history,
    isPending: reads.enabled && (reads.trustQuery.isPending || reads.historyQuery.isPending),
    failed:
      reads.enabled &&
      (reads.trustQuery.isError ||
        reads.historyQuery.isError ||
        reads.jobQuery.isError ||
        action.failed),
  };
}

function verificationAvailable(
  reads: ReturnType<typeof useAuditReads>,
  report: SandboxReport,
): boolean {
  return Boolean(
    reads.enabled &&
    reads.context.operator &&
    reads.trust &&
    reads.historyQuery.data &&
    report.data.finalized &&
    report.data.final_audit &&
    !reads.active &&
    !reads.trustQuery.isError &&
    !reads.historyQuery.isError &&
    !reads.jobQuery.isError,
  );
}

function useAuditActions(
  sessionId: string,
  report: SandboxReport,
  reads: ReturnType<typeof useAuditReads>,
  ready: boolean,
) {
  const { backend, context, trust, historyQuery } = reads;
  const client = useQueryClient();
  const store = useAppStoreApi();
  const actionKey = JSON.stringify([context.key, report.digest, trust?.version, trust?.digest]);
  const runner = useAuditActionRunner(
    actionKey,
    () => {
      const current = sandboxApprovalContext(
        store.getState(),
        sessionId,
        getBackendConfig().apiBaseUrl,
      );
      return current.key === context.key && current.enabled && current.operator;
    },
    () => Promise.all([reads.trustQuery.refetch(), historyQuery.refetch()]),
  );
  const canVerify = ready && !runner.isBusy;
  const canManageTrust = Boolean(
    reads.enabled && context.operator && trust && !reads.trustQuery.isError && !runner.isBusy,
  );
  function verify() {
    const receipt = report.data.final_audit;
    if (!canVerify || !trust || !receipt) return Promise.resolve(false);
    return runner.run(async () => {
      const view = await verifySandboxAudit(
        sessionId,
        report.attempt_id,
        {
          request_id: generateUUID(),
          expected_report_digest: report.digest,
          expected_artifact_digest: receipt.artifact_set_digest,
          expected_trust_version: trust.version,
        },
        { baseUrl: backend },
      );
      if (!isAuditViewBound(view, report)) throw INVALID_AUDIT_BINDING;
      if (!runner.current()) return false;
      await client.cancelQueries({ queryKey: reads.historyKey, exact: true });
      if (!runner.current()) return false;
      client.setQueryData<InfiniteData<SandboxAuditPage, string>>(reads.historyKey, (previous) => ({
        pages: [mergeAcceptedAudit(previous?.pages[0], view)],
        pageParams: [""],
      }));
      await historyQuery.refetch();
      return runner.current();
    });
  }
  function saveTrust(request: SandboxAuditTrustRequest) {
    if (
      !canManageTrust ||
      !trust ||
      request.expected_version !== trust.version ||
      !validTrustInput(request)
    )
      return Promise.resolve(false);
    return runner.run(async () => {
      const saved = await saveSandboxAuditTrust(sessionId, report.attempt_id, request, {
        baseUrl: backend,
      });
      if (
        !isAuditTrustBound(saved, report) ||
        saved.version !== request.expected_version + 1 ||
        saved.revoked !== Boolean(request.revoke)
      )
        throw INVALID_AUDIT_BINDING;
      if (!runner.current()) return false;
      await client.cancelQueries({ queryKey: reads.trustKey, exact: true });
      if (!runner.current()) return false;
      client.setQueryData<SandboxAuditTrust>(reads.trustKey, saved);
      await historyQuery.refetch();
      return true;
    });
  }
  return {
    verify,
    saveTrust,
    canVerify,
    canManageTrust,
    isBusy: runner.isBusy,
    failed: runner.failed,
  };
}

function validTrustInput(request: SandboxAuditTrustRequest): boolean {
  if (request.revoke) return !request.public_key_pem;
  return Boolean(request.public_key_pem?.trim() && request.public_key_pem.length <= 16_384);
}

function useAuditActionRunner(
  key: string,
  authority: () => boolean,
  reconcile: () => Promise<unknown>,
) {
  const latest = useRef(key);
  latest.current = key;
  const flight = useRef<string | null>(null);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const [action, setAction] = useState<{ key: string; busy: boolean; failed: boolean } | null>(
    null,
  );
  const current = () => mounted.current && latest.current === key && authority();
  async function run(submit: () => Promise<boolean>): Promise<boolean> {
    if (flight.current === key || !current()) return false;
    flight.current = key;
    setAction({ key, busy: true, failed: false });
    try {
      return await submit();
    } catch {
      if (current()) {
        await reconcile();
        if (current()) setAction({ key, busy: false, failed: true });
      }
      return false;
    } finally {
      if (flight.current === key) flight.current = null;
      if (current())
        setAction((previous) => (previous?.key === key ? { ...previous, busy: false } : previous));
    }
  }
  return {
    run,
    current,
    isBusy: action?.key === key && action.busy,
    failed: action?.key === key && action.failed,
  };
}

function mergeAcceptedAudit(
  previous: SandboxAuditPage | undefined,
  view: SandboxAuditView,
): SandboxAuditPage {
  const rows = [
    view,
    ...(previous?.verifications ?? []).filter(
      (row) => row.verification.id !== view.verification.id,
    ),
  ];
  return {
    verifications: rows.slice(0, 20),
    next_cursor: rows.length > 20 ? rows[19].verification.id : previous?.next_cursor,
  };
}
