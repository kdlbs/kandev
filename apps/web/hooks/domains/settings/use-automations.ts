"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createAutomation,
  updateAutomation as apiUpdateAutomation,
  deleteAutomation,
  enableAutomation,
  disableAutomation,
  triggerAutomation,
} from "@/lib/api/domains/automation-api";
import { qk } from "@/lib/query/keys";
import { automationsQueryOptions } from "@/lib/query/query-options/automations";
import type {
  Automation,
  CreateAutomationRequest,
  CreateAutomationResponse,
  UpdateAutomationRequest,
} from "@/lib/types/automation";

export function useAutomations(workspaceId: string | null) {
  const queryClient = useQueryClient();
  const query = useQuery({
    ...automationsQueryOptions(workspaceId ?? ""),
    enabled: Boolean(workspaceId),
  });
  const items = query.data ?? [];
  const refetch = query.refetch;

  const create = useCallback(
    async (req: CreateAutomationRequest): Promise<CreateAutomationResponse> => {
      const automation = await createAutomation(req);
      // Strip the one-time webhook_secret before caching so it doesn't leak
      // into devtools or error-reporting SDKs. The full response (with secret)
      // is still returned to the caller for the reveal dialog.
      const { webhook_secret: _secret, ...stored } = automation;
      queryClient.setQueryData(
        qk.automations.list(req.workspace_id),
        (prev: Automation[] | undefined) => [stored, ...(prev ?? [])],
      );
      return automation;
    },
    [queryClient],
  );

  const update = useCallback(
    async (id: string, req: UpdateAutomationRequest) => {
      const automation = await apiUpdateAutomation(id, req);
      if (workspaceId) patchAutomationCache(queryClient, workspaceId, automation);
      return automation;
    },
    [queryClient, workspaceId],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteAutomation(id);
      if (workspaceId) {
        queryClient.setQueryData(
          qk.automations.list(workspaceId),
          (prev: Automation[] | undefined) =>
            (prev ?? []).filter((automation) => automation.id !== id),
        );
      }
    },
    [queryClient, workspaceId],
  );

  const enable = useCallback(
    async (id: string) => {
      const automation = await enableAutomation(id);
      if (workspaceId) patchAutomationCache(queryClient, workspaceId, automation);
      return automation;
    },
    [queryClient, workspaceId],
  );

  const disable = useCallback(
    async (id: string) => {
      const automation = await disableAutomation(id);
      if (workspaceId) patchAutomationCache(queryClient, workspaceId, automation);
      return automation;
    },
    [queryClient, workspaceId],
  );

  const trigger = useCallback(async (id: string) => {
    return triggerAutomation(id);
  }, []);

  const refresh = useCallback(() => {
    void refetch();
  }, [refetch]);

  return {
    items,
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
    create,
    update,
    remove,
    enable,
    disable,
    trigger,
    refresh,
  };
}

function patchAutomationCache(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string,
  automation: Automation,
) {
  queryClient.setQueryData(qk.automations.list(workspaceId), (prev: Automation[] | undefined) => {
    const items = prev ?? [];
    const index = items.findIndex((item) => item.id === automation.id);
    if (index === -1) return [automation, ...items];
    const copy = [...items];
    copy[index] = automation;
    return copy;
  });
}
