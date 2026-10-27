"use client";

import { useEffect } from "react";
import { useIsMutating, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  fetchSandboxApprovals,
  decideSandboxApproval,
  type SandboxApproval,
  type SandboxDecision,
} from "@/lib/api/domains/sandbox-approvals-api";
import { getBackendConfig } from "@/lib/config";
import {
  isApprovalBound,
  isApprovalPending,
  sandboxApprovalContext,
} from "@/lib/sandbox-approval-context";
import { subscribeSandboxApprovalChanges } from "@/lib/ws/handlers/sandbox-approvals";
import { submitSandboxOnce } from "@/lib/sandbox-submission";
import { useExecutionClock } from "@/hooks/use-execution-clock";

type Context = ReturnType<typeof sandboxApprovalContext>;
// i18n-exempt: Private control-flow sentinel. The UI displays a localized decision failure.
const STALE_APPROVAL = Symbol("stale approval");

export function useSandboxApprovals(sessionId: string) {
  const client = useQueryClient();
  const store = useAppStoreApi();
  const backend = getBackendConfig().apiBaseUrl;
  const context = useAppStore(
    useShallow((state) => sandboxApprovalContext(state, sessionId, backend)),
  );
  const queryKey = ["sandbox-approvals", sessionId, context.key];
  useEffect(
    () =>
      subscribeSandboxApprovalChanges(store, sessionId, () => {
        void client.resetQueries({
          queryKey: ["sandbox-approvals", sessionId, context.key],
          exact: true,
        });
      }),
    [store, sessionId, client, context.key],
  );
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => fetchSandboxApprovals(sessionId, { init: { signal } }),
    enabled: context.enabled,
    retry: false,
    gcTime: 0,
  });
  const approvals = context.enabled
    ? (query.data ?? []).filter((row) => isApprovalBound(row, context))
    : [];
  const deadlines = approvals
    .filter((row) => row.state === "pending")
    .map((row) => Date.parse(row.expires_at))
    .filter((deadline) => Number.isFinite(deadline) && deadline > Date.now());
  const clock = useExecutionClock(deadlines.length > 0 ? Math.min(...deadlines) : null);
  const actions = useApprovalDecision(sessionId, context, backend);
  return {
    approvals,
    snapshot: context.enabled ? context.snapshot : null,
    isPending: context.enabled && query.isPending,
    isError: context.enabled && query.isError,
    isAvailable: context.enabled && query.isSuccess,
    ...actions,
    canDecide: (approval: SandboxApproval) =>
      !actions.isBusy &&
      !query.isFetching &&
      context.enabled &&
      context.operator &&
      approvals.some(
        (row) =>
          row.request_id === approval.request_id &&
          row.version === approval.version &&
          isApprovalPending(row, clock || Date.now()),
      ),
  };
}

function useApprovalDecision(sessionId: string, context: Context, backend: string) {
  const store = useAppStoreApi();
  const client = useQueryClient();
  const mutationKey = ["sandbox-approvals", sessionId, context.key];
  const mutation = useMutation({
    mutationKey,
    retry: false,
    mutationFn: ({
      approval,
      decision,
    }: {
      approval: SandboxApproval;
      decision: SandboxDecision;
    }) =>
      submitSandboxOnce(
        client,
        JSON.stringify(["approval", context.key, approval.request_id]),
        () => {
          const current = sandboxApprovalContext(store.getState(), sessionId, backend);
          const rows = client.getQueryData<SandboxApproval[]>(mutationKey) ?? [];
          const row = rows.find(
            (value) =>
              value.request_id === approval.request_id &&
              value.version === approval.version &&
              isApprovalBound(value, current),
          );
          if (
            current.key !== context.key ||
            !current.enabled ||
            !current.operator ||
            !row ||
            !isApprovalPending(row, Date.now()) ||
            client.isFetching({ queryKey: mutationKey }) > 0
          )
            throw STALE_APPROVAL;
          return decideSandboxApproval(row, decision);
        },
      ),
    onSettled: () => client.invalidateQueries({ queryKey: mutationKey, exact: true }),
  });
  const isBusy = useIsMutating({ mutationKey, exact: true }) > 0;
  return {
    isBusy,
    decisionFailed: mutation.isError,
    failedRequestId: mutation.isError ? mutation.variables?.approval.request_id : undefined,
    decide: async (approval: SandboxApproval, decision: SandboxDecision) => {
      try {
        await mutation.mutateAsync({ approval, decision });
        return true;
      } catch {
        return false;
      }
    },
  };
}
