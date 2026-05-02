"use client";

import { getLinearConfig } from "@/lib/api/domains/linear-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { useLinearEnabled } from "./use-linear-enabled";

const fetchLinearConfig = () => getLinearConfig();

export function useLinearAuthed(): boolean {
  return useIntegrationAuthed(fetchLinearConfig);
}

export function useLinearAvailable(): boolean {
  return useIntegrationAvailable({
    useEnabled: useLinearEnabled,
    fetchConfig: fetchLinearConfig,
  });
}
