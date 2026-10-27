"use client";

import { useQueries, useQuery } from "@tanstack/react-query";
import {
  fetchSandboxPolicies,
  fetchSandboxProviders,
  fetchSandboxRevision,
} from "@/lib/api/domains/sandbox-api";
import type { SandboxExecutorPolicy } from "@/lib/types/sandbox";

export function useSandboxExecutorReads(
  identity: string,
  policy: SandboxExecutorPolicy | undefined,
  canEdit: boolean,
) {
  const providers = useQuery({
    queryKey: ["sandbox", "providers", identity],
    queryFn: ({ signal }) => fetchSandboxProviders({ init: { signal } }),
    enabled: canEdit,
    retry: false,
    gcTime: 0,
  });
  const catalog = useQuery({
    queryKey: ["sandbox", "policies", identity, ""],
    queryFn: ({ signal }) => fetchSandboxPolicies("", { init: { signal } }),
    enabled: canEdit,
    retry: false,
    gcTime: 0,
  });
  const ids = [
    ...new Set(
      [
        policy?.ceiling_revision_id,
        policy?.default_revision_id,
        ...(policy?.eligible_revision_ids ?? []),
      ].filter((id): id is string => Boolean(id)),
    ),
  ];
  const revisions = useQueries({
    queries: ids.map((id) => ({
      queryKey: ["sandbox", "revision", identity, id],
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        fetchSandboxRevision(id, { init: { signal } }),
      retry: false,
      gcTime: 0,
    })),
  });
  const all = new Map((catalog.data ?? []).map((item) => [item.id, item]));
  for (const result of revisions) if (result.data) all.set(result.data.id, result.data);
  return {
    providers,
    catalog,
    revisions: [...all.values()],
    loading: catalog.isFetching || revisions.some((item) => item.isFetching),
    failed: catalog.isError || providers.isError || revisions.some((item) => item.isError),
  };
}
