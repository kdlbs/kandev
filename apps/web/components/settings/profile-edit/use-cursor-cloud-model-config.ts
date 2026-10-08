import { useEffect, useState } from "react";
import { listExecutors } from "@/lib/api/domains/settings-api";
import { loadCursorCloudCatalog } from "@/lib/api/domains/cursor-cloud-api";
import type { ModelConfig } from "@/lib/types/http";
import { cursorCloudModelEntries, cursorCloudProfiles } from "./cursor-cloud-models";

// i18n-exempt: stable managed-agent identity from the backend contract
const CURSOR_CLOUD_AGENT_ID = "cursor_cloud";

export function useCursorCloudModelConfig(agentId: string, modelConfig: ModelConfig): ModelConfig {
  const [models, setModels] = useState<ModelConfig["available_models"]>([]);
  const [loading, setLoading] = useState(agentId === CURSOR_CLOUD_AGENT_ID);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (agentId !== CURSOR_CLOUD_AGENT_ID) return;
    let active = true;
    setLoading(true);
    setFailed(false);
    void listExecutors()
      .then(async ({ executors }) => {
        const profiles = cursorCloudProfiles(executors);
        const catalogs = await Promise.allSettled(
          profiles.map((profile) => loadCursorCloudCatalog(profile.secretId, profile.callbackUrl)),
        );
        const loadedCatalogs = catalogs.flatMap((catalog) =>
          catalog.status === "fulfilled" ? [catalog.value] : [],
        );
        if (!active) return;
        setModels(cursorCloudModelEntries(loadedCatalogs));
        setFailed(profiles.length > 0 && loadedCatalogs.length === 0);
      })
      .catch(() => {
        if (active) setFailed(true);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [agentId]);

  if (agentId !== CURSOR_CLOUD_AGENT_ID) return modelConfig;
  let status: ModelConfig["status"] = "ok";
  if (loading) status = "probing";
  else if (failed) status = "failed";
  return {
    ...modelConfig,
    available_models: models,
    supports_dynamic_models: false,
    // i18n-exempt: capability state received by profile form logic
    status,
  };
}
