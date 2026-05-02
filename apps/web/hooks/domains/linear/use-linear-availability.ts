"use client";

import { getLinearConfig } from "@/lib/api/domains/linear-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { useLinearEnabled } from "./use-linear-enabled";

export function useLinearAuthed(workspaceId: string | undefined): boolean {
  return useIntegrationAuthed(workspaceId, getLinearConfig);
}

export function useLinearAvailable(workspaceId: string | null | undefined): boolean {
  return useIntegrationAvailable(workspaceId, {
    useEnabled: useLinearEnabled,
    fetchConfig: getLinearConfig,
  });
}
