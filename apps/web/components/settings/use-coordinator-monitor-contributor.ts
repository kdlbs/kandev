"use client";

import { useEffect, useRef, useState } from "react";
import {
  getCoordinatorMonitoringAction,
  setCoordinatorMonitoringAction,
} from "@/app/actions/workspaces";
import { t } from "@/lib/i18n";
import type { CoordinatorStepMonitorEntry } from "@/lib/types/http";
import type { WorkspaceId } from "@/lib/types/ids";
import { useToast } from "@/components/toast-provider";
import { useSettingsSaveContributor } from "./settings-save-provider";
import type { MonitorConfigMap } from "./coordinator-monitor-section";

const TEMP_WORKFLOW_PREFIX = "temp-workflow-";

function entriesToConfigMap(entries: CoordinatorStepMonitorEntry[]): MonitorConfigMap {
  const map: MonitorConfigMap = {};
  for (const entry of entries) {
    map[entry.workflow_step_id] = { selected: entry.selected, prompt: entry.prompt };
  }
  return map;
}

function configMapToEntries(config: MonitorConfigMap): CoordinatorStepMonitorEntry[] {
  return Object.entries(config).map(([workflow_step_id, cfg]) => ({
    workflow_step_id,
    selected: cfg.selected,
    prompt: cfg.prompt,
  }));
}

/** Treats an unselected step with no prompt as equivalent to "not present". */
function isMeaningfulEntry(cfg: { selected: boolean; prompt: string }): boolean {
  return cfg.selected || cfg.prompt !== "";
}

function configMapsEqual(a: MonitorConfigMap, b: MonitorConfigMap): boolean {
  const aKeys = Object.keys(a).filter((key) => isMeaningfulEntry(a[key]));
  const bKeys = Object.keys(b).filter((key) => isMeaningfulEntry(b[key]));
  if (aKeys.length !== bKeys.length) return false;
  return aKeys.every((key) => {
    const bValue = b[key];
    return (
      bValue !== undefined && a[key].selected === bValue.selected && a[key].prompt === bValue.prompt
    );
  });
}

type CoordinatorMonitorContributorArgs = {
  workflowId: string;
  workspaceId: WorkspaceId;
};

/**
 * Loads, saves, and dirty-tracks the Coordinator plugin's per-step monitoring
 * policy (checked steps + custom prompts) for a workflow. Backed by host
 * storage (Settings > Workspace > Workflow configuration); registers with the
 * shared settings save coordinator so the floating Save/Discard control owns
 * persistence, matching the pattern in use-workflow-draft-contributor.ts.
 */
export function useCoordinatorMonitorContributor({
  workflowId,
  workspaceId,
}: CoordinatorMonitorContributorArgs) {
  const { toast } = useToast();
  const [draftConfig, setDraftConfig] = useState<MonitorConfigMap>({});
  const [savedConfig, setSavedConfig] = useState<MonitorConfigMap>({});
  const [loading, setLoading] = useState(!workflowId.startsWith(TEMP_WORKFLOW_PREFIX));
  const draftConfigRef = useRef(draftConfig);
  draftConfigRef.current = draftConfig;
  const savedConfigRef = useRef(savedConfig);
  savedConfigRef.current = savedConfig;

  useEffect(() => {
    if (workflowId.startsWith(TEMP_WORKFLOW_PREFIX)) {
      setDraftConfig({});
      setSavedConfig({});
      setLoading(false);
      return;
    }
    let cancelled = false;
    const load = async () => {
      setDraftConfig({});
      setSavedConfig({});
      setLoading(true);
      try {
        const res = await getCoordinatorMonitoringAction(workflowId);
        if (cancelled) return;
        const map = entriesToConfigMap(res.entries ?? []);
        setDraftConfig(map);
        setSavedConfig(map);
      } catch {
        if (!cancelled) {
          toast({ title: t("workflows:failedToLoadCoordinatorMonitoring"), variant: "error" });
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    load();
    return () => {
      cancelled = true;
    };
  }, [workflowId, toast]);

  const isDirty = !configMapsEqual(draftConfig, savedConfig);
  const revision = JSON.stringify(draftConfig);

  useSettingsSaveContributor({
    id: `coordinator-monitor:${workflowId}`,
    revision,
    isDirty,
    save: async () => {
      const entries = configMapToEntries(draftConfigRef.current);
      const res = await setCoordinatorMonitoringAction(workflowId, {
        workspace_id: workspaceId,
        entries,
      });
      const saved = entriesToConfigMap(res.entries ?? []);
      setSavedConfig(saved);
      setDraftConfig(saved);
    },
    discard: () => {
      setDraftConfig(savedConfigRef.current);
    },
  });

  return {
    draftConfig,
    setDraftConfig,
    isDirty,
    loading,
  };
}
