"use client";

import { useCallback } from "react";
import { getLinearConfig } from "@/lib/api/domains/linear-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { qk } from "@/lib/query/keys";
import { useLinearEnabled } from "./use-linear-enabled";

export function useLinearAuthed(workspaceId?: string | null): boolean {
  const fetchConfig = useCallback(
    () => getLinearConfig(workspaceId ? { workspaceId } : undefined),
    [workspaceId],
  );
  return useIntegrationAuthed({
    active: workspaceId !== null,
    fetchConfig,
    queryKey: qk.integrations.linear.config(workspaceId),
  });
}

export function useLinearAvailable(workspaceId?: string | null): boolean {
  const fetchConfig = useCallback(
    () => getLinearConfig(workspaceId ? { workspaceId } : undefined),
    [workspaceId],
  );
  return useIntegrationAvailable({
    active: workspaceId !== null,
    useEnabled: useLinearEnabled,
    fetchConfig,
    queryKey: qk.integrations.linear.config(workspaceId),
  });
}
