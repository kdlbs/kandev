"use client";

import { useCallback, useEffect, useState } from "react";
import { useToast } from "@/components/toast-provider";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import {
  getWorkflowSyncConfig,
  setWorkflowSyncConfig,
  deleteWorkflowSyncConfig,
  forceWorkflowSync,
} from "@/lib/api/domains/workflow-sync-api";
import type { WorkflowSyncConfig, WorkflowSyncSetConfigRequest } from "@/lib/types/workflow-sync";
import type { ParsedGitHubRepoUrl } from "@/lib/utils/github-repo-url";

export type WorkflowSyncFormState = {
  repo_owner: string;
  repo_name: string;
  branch: string;
  path: string;
  interval_seconds: number;
};

const DEFAULT_FORM: WorkflowSyncFormState = {
  repo_owner: "",
  repo_name: "",
  branch: "main",
  path: ".kandev/workflows",
  interval_seconds: 300,
};

function configToForm(cfg: WorkflowSyncConfig | null): WorkflowSyncFormState {
  if (!cfg) return DEFAULT_FORM;
  return {
    repo_owner: cfg.repo_owner,
    repo_name: cfg.repo_name,
    branch: cfg.branch,
    path: cfg.path,
    interval_seconds: cfg.interval_seconds,
  };
}

// Background refresh so the status banner picks up new poller results
// (last_ok / last_error / last_warnings) without requiring a page reload. We
// re-fetch the config rather than the loud full `load()` to avoid flashing
// the form while the user is editing it.
function useWorkflowSyncConfigRefresh(
  workspaceId: string,
  setConfig: (cfg: WorkflowSyncConfig | null) => void,
) {
  useEffect(() => {
    const id = setInterval(() => {
      getWorkflowSyncConfig({ workspaceId })
        .then((cfg) => setConfig(cfg))
        .catch(() => {
          /* transient failures are fine — next tick retries */
        });
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => clearInterval(id);
  }, [workspaceId, setConfig]);
}

// useWorkflowSyncForm owns the editable form state: per-field updates plus
// bulk fill from a pasted GitHub link. Branch and directory are only
// overwritten when the link actually carried them (/tree/... or /blob/...
// forms), so a bare repo URL keeps the defaults.
function useWorkflowSyncForm() {
  const [form, setForm] = useState<WorkflowSyncFormState>(DEFAULT_FORM);

  const update = useCallback(
    <K extends keyof WorkflowSyncFormState>(key: K, value: WorkflowSyncFormState[K]) =>
      setForm((prev) => ({ ...prev, [key]: value })),
    [],
  );

  const applyParsedUrl = useCallback(
    (parsed: ParsedGitHubRepoUrl) =>
      setForm((prev) => ({
        ...prev,
        repo_owner: parsed.owner,
        repo_name: parsed.repo,
        branch: parsed.branch ?? prev.branch,
        path: parsed.path ?? prev.path,
      })),
    [],
  );

  return { form, setForm, update, applyParsedUrl };
}

export function useWorkflowSync(workspaceId: string) {
  const { toast } = useToast();
  const [config, setConfig] = useState<WorkflowSyncConfig | null>(null);
  const { form, setForm, update, applyParsedUrl } = useWorkflowSyncForm();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const cfg = await getWorkflowSyncConfig({ workspaceId });
      setConfig(cfg);
      setForm(configToForm(cfg));
    } catch (err) {
      toast({
        description: `Failed to load workflow sync config: ${String(err)}`,
        variant: "error",
      });
    } finally {
      setLoading(false);
    }
  }, [workspaceId, toast]);

  useEffect(() => {
    void load();
  }, [load]);

  useWorkflowSyncConfigRefresh(workspaceId, setConfig);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const payload: WorkflowSyncSetConfigRequest = {
        repo_owner: form.repo_owner.trim(),
        repo_name: form.repo_name.trim(),
        branch: form.branch.trim(),
        path: form.path.trim(),
        interval_seconds: form.interval_seconds,
      };
      const saved = await setWorkflowSyncConfig(payload, { workspaceId });
      setConfig(saved);
      setForm(configToForm(saved));
      toast({ description: "Workflow sync configuration saved", variant: "success" });
    } catch (err) {
      toast({ description: `Save failed: ${String(err)}`, variant: "error" });
    } finally {
      setSaving(false);
    }
  }, [workspaceId, form, toast]);

  const handleDelete = useCallback(async () => {
    if (
      !confirm("Remove workflow sync configuration? This will not delete already-synced workflows.")
    ) {
      return;
    }
    try {
      await deleteWorkflowSyncConfig({ workspaceId });
      setConfig(null);
      setForm(DEFAULT_FORM);
      toast({ description: "Workflow sync configuration removed", variant: "success" });
    } catch (err) {
      toast({ description: `Delete failed: ${String(err)}`, variant: "error" });
    }
  }, [workspaceId, toast]);

  const handleSyncNow = useCallback(async () => {
    setSyncing(true);
    try {
      const res = await forceWorkflowSync({ workspaceId });
      setConfig(res.config);
      setForm(configToForm(res.config));
      if (res.error) {
        toast({ description: `Sync failed: ${res.error}`, variant: "error" });
      } else if (res.result?.warnings?.length) {
        toast({ description: "Sync completed with warnings", variant: "default" });
      } else {
        toast({ description: "Workflow sync completed", variant: "success" });
      }
    } catch (err) {
      toast({ description: `Sync failed: ${String(err)}`, variant: "error" });
    } finally {
      setSyncing(false);
    }
  }, [workspaceId, toast]);

  return {
    config,
    form,
    loading,
    saving,
    syncing,
    update,
    applyParsedUrl,
    handleSave,
    handleDelete,
    handleSyncNow,
  };
}
