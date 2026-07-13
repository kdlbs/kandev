"use client";

import { useCallback } from "react";
import { getLinearConfig } from "@/lib/api/domains/linear-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { useLinearEnabled } from "./use-linear-enabled";

export function useLinearAuthed(workspaceId?: string | null): boolean {
  const fetchConfig = useCallback(
    () => getLinearConfig(workspaceId ? { workspaceId } : undefined),
    [workspaceId],
  );
  return useIntegrationAuthed(fetchConfig);
}

export function useLinearAvailable(workspaceId?: string | null): boolean {
  const fetchConfig = useCallback(
    () => getLinearConfig(workspaceId ? { workspaceId } : undefined),
    [workspaceId],
  );
  return useIntegrationAvailable({
    useEnabled: useLinearEnabled,
    fetchConfig,
  });
}
