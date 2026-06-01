"use client";

import { getLinearConfig } from "@/lib/api/domains/linear-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { useLinearEnabled } from "./use-linear-enabled";

const fetchLinearConfig = () => getLinearConfig();

export function useLinearAuthed(): boolean {
  return useIntegrationAuthed("linear", fetchLinearConfig);
}

export function useLinearAvailable(): boolean {
  return useIntegrationAvailable({
    kind: "linear",
    useEnabled: useLinearEnabled,
    fetchConfig: fetchLinearConfig,
  });
}
