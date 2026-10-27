"use client";

import { useQuery } from "@tanstack/react-query";
import {
  checkSandboxAvailability,
  fetchSandboxProviders,
  fetchSandboxRevision,
  previewSandboxSelection,
} from "@/lib/api/domains/sandbox-api";
import type { SandboxSelectionContext } from "@/lib/types/sandbox";

export function useSandboxPolicyReads(
  identity: string,
  context: SandboxSelectionContext,
  revisionId: string,
  canEdit: boolean,
  bindingVersion: number | undefined,
) {
  const providers = useQuery({
    queryKey: ["sandbox", "providers", identity],
    queryFn: ({ signal }) => fetchSandboxProviders({ init: { signal } }),
    enabled: canEdit,
    retry: false,
    gcTime: 0,
  });
  const revision = useQuery({
    queryKey: ["sandbox", "revision", identity, revisionId],
    queryFn: ({ signal }) => fetchSandboxRevision(revisionId, { init: { signal } }),
    enabled: Boolean(revisionId),
    retry: false,
    gcTime: 0,
  });
  const preview = useQuery({
    queryKey: ["sandbox", "preview", identity, context, bindingVersion],
    queryFn: ({ signal }) => previewSandboxSelection(context, { init: { signal } }),
    enabled: canEdit && Boolean(context.executor_id),
    retry: false,
    gcTime: 0,
  });
  const availability = useQuery({
    queryKey: ["sandbox", "availability", identity, context, bindingVersion],
    queryFn: ({ signal }) => checkSandboxAvailability(context, { init: { signal } }),
    enabled: false,
    retry: false,
    gcTime: 0,
  });
  return { providers, revision, preview, availability };
}
