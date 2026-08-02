"use client";

import { useEffect, useState } from "react";
import {
  DEFAULT_AZURE_PULL_REQUEST_ACTIONS,
  DEFAULT_AZURE_PULL_REQUEST_QUERIES,
  DEFAULT_AZURE_WORK_ITEM_ACTIONS,
  DEFAULT_AZURE_WORK_ITEM_QUERIES,
} from "@/components/azure-devops/azure-devops-workspace-defaults";
import { getAzureDevOpsWorkspaceSettings } from "@/lib/api/domains/azure-devops-api";
import type { AzureDevOpsWorkspaceSettings } from "@/lib/types/azure-devops";

function defaults(workspaceId = ""): AzureDevOpsWorkspaceSettings {
  return {
    workspaceId,
    workItemQueries: DEFAULT_AZURE_WORK_ITEM_QUERIES,
    pullRequestQueries: DEFAULT_AZURE_PULL_REQUEST_QUERIES,
    workItemActions: DEFAULT_AZURE_WORK_ITEM_ACTIONS,
    pullRequestActions: DEFAULT_AZURE_PULL_REQUEST_ACTIONS,
  };
}

export function useAzureDevOpsWorkspaceSettings(workspaceId?: string) {
  const [settings, setSettings] = useState<AzureDevOpsWorkspaceSettings>(() =>
    defaults(workspaceId),
  );
  const [loading, setLoading] = useState(Boolean(workspaceId));

  useEffect(() => {
    if (!workspaceId) {
      setSettings(defaults());
      setLoading(false);
      return;
    }
    let current = true;
    setLoading(true);
    void getAzureDevOpsWorkspaceSettings(workspaceId)
      .then((next) => current && setSettings(next))
      .catch(() => current && setSettings(defaults(workspaceId)))
      .finally(() => current && setLoading(false));
    return () => {
      current = false;
    };
  }, [workspaceId]);

  return { settings, loading };
}
