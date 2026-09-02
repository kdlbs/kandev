"use client";

import { useCallback, useEffect, useState } from "react";
import { t } from "@/lib/i18n";
import { toast } from "@/lib/toast/sonner";
import { useRouter } from "@/lib/routing/client-router";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import {
  getOfficeConfigSyncConfig,
  setOfficeConfigSyncConfig,
  deleteOfficeConfigSyncConfig,
  forceOfficeConfigSync,
} from "@/lib/api/domains/office-config-sync-api";
import type {
  OfficeConfigSyncConfig,
  OfficeConfigSyncProvider,
  OfficeConfigSyncSetConfigRequest,
} from "@/lib/types/office-config-sync";

export type OfficeConfigSyncFormState = {
  provider: OfficeConfigSyncProvider;
  repo_owner: string;
  repo_name: string;
  project_path: string;
  branch: string;
  path: string;
  interval_seconds: number;
  poll_enabled: boolean;
};

const DEFAULT_FORM: OfficeConfigSyncFormState = {
  provider: "github",
  repo_owner: "",
  repo_name: "",
  project_path: "",
  branch: "main",
  path: "",
  interval_seconds: 300,
  poll_enabled: true,
};

function configToForm(cfg: OfficeConfigSyncConfig | null): OfficeConfigSyncFormState {
  if (!cfg) return DEFAULT_FORM;
  return {
    provider: cfg.provider,
    repo_owner: cfg.repo_owner,
    repo_name: cfg.repo_name,
    project_path: cfg.project_path,
    branch: cfg.branch,
    path: cfg.path,
    interval_seconds: cfg.interval_seconds,
    poll_enabled: cfg.poll_enabled,
  };
}

// Background refresh so the status card picks up new poller results
// (last_ok / last_error / last_warnings) without requiring a page reload. We
// re-fetch the config rather than the loud full `load()` to avoid flashing
// the form while the user is editing it.
function useOfficeConfigSyncRefresh(
  workspaceId: string,
  setConfig: (cfg: OfficeConfigSyncConfig | null) => void,
) {
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    const id = setInterval(() => {
      getOfficeConfigSyncConfig(workspaceId)
        .then((cfg) => {
          if (!cancelled) setConfig(cfg);
        })
        .catch(() => {
          /* transient failures are fine — next tick retries */
        });
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [workspaceId, setConfig]);
}

// useOfficeConfigSyncForm owns the editable form state. Switching providers
// clears the fields that belonged to the other provider, since a stale
// repo_owner sitting alongside a new project_path fails backend validation
// (a request cannot carry both).
function useOfficeConfigSyncForm() {
  const [form, setForm] = useState<OfficeConfigSyncFormState>(DEFAULT_FORM);

  const update = useCallback(
    <K extends keyof OfficeConfigSyncFormState>(key: K, value: OfficeConfigSyncFormState[K]) =>
      setForm((prev) => ({ ...prev, [key]: value })),
    [],
  );

  const setProvider = useCallback((provider: OfficeConfigSyncProvider) => {
    setForm((prev) => ({
      ...prev,
      provider,
      repo_owner: "",
      repo_name: "",
      project_path: "",
    }));
  }, []);

  const reset = useCallback((cfg: OfficeConfigSyncConfig | null) => {
    setForm(configToForm(cfg));
  }, []);

  return { form, update, setProvider, reset };
}

function syncOutcomeToast(error: string | undefined, warnings: string[] | undefined): void {
  if (error) {
    toast.error(t("office:configSyncSyncFailedWithError", { error }));
    return;
  }
  if (warnings?.length) {
    toast.success(t("office:configSyncSyncCompletedWithWarnings"));
    return;
  }
  toast.success(t("office:configSyncSyncCompleted"));
}

type InitialLoadDeps = {
  setConfig: (cfg: OfficeConfigSyncConfig | null) => void;
  reset: (cfg: OfficeConfigSyncConfig | null) => void;
  setLoading: (loading: boolean) => void;
};

// useOfficeConfigSyncInitialLoad fetches the stored config on mount /
// workspace change. It guards against stale responses: if the hook
// re-renders for a different workspace while a fetch is in flight, the old
// workspace's config must not populate the new one's form.
function useOfficeConfigSyncInitialLoad(
  workspaceId: string,
  { setConfig, reset, setLoading }: InitialLoadDeps,
) {
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    setLoading(true);
    getOfficeConfigSyncConfig(workspaceId)
      .then((cfg) => {
        if (cancelled) return;
        setConfig(cfg);
        reset(cfg);
      })
      .catch((err) => {
        if (cancelled) return;
        toast.error(t("office:configSyncFailedToLoadConfig", { error: String(err) }));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, setConfig, reset, setLoading]);
}

export function useOfficeConfigSync(workspaceId: string) {
  const router = useRouter();
  const [config, setConfig] = useState<OfficeConfigSyncConfig | null>(null);
  const { form, update, setProvider, reset } = useOfficeConfigSyncForm();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);

  useOfficeConfigSyncInitialLoad(workspaceId, { setConfig, reset, setLoading });
  useOfficeConfigSyncRefresh(workspaceId, setConfig);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const payload: OfficeConfigSyncSetConfigRequest =
        form.provider === "gitlab"
          ? {
              provider: "gitlab",
              project_path: form.project_path.trim(),
              branch: form.branch.trim(),
              path: form.path.trim(),
              interval_seconds: form.interval_seconds,
              poll_enabled: form.poll_enabled,
            }
          : {
              provider: "github",
              repo_owner: form.repo_owner.trim(),
              repo_name: form.repo_name.trim(),
              branch: form.branch.trim(),
              path: form.path.trim(),
              interval_seconds: form.interval_seconds,
              poll_enabled: form.poll_enabled,
            };
      const saved = await setOfficeConfigSyncConfig(workspaceId, payload);
      setConfig(saved);
      reset(saved);
      toast.success(t("office:configSyncConfigSaved"));
      return true;
    } catch (err) {
      toast.error(t("office:configSyncSaveFailed", { error: String(err) }));
      return false;
    } finally {
      setSaving(false);
    }
  }, [workspaceId, form, reset]);

  const handleDelete = useCallback(async () => {
    try {
      await deleteOfficeConfigSyncConfig(workspaceId);
      setConfig(null);
      reset(null);
      toast.success(t("office:configSyncRemoved"));
      // Config sync also releases the mutual-exclusion guard on filesystem
      // import/export and raw-git clone/pull server-side; reload so those
      // surfaces unlock without a manual refresh.
      router.refresh();
      return true;
    } catch (err) {
      toast.error(t("office:configSyncRemoveFailed", { error: String(err) }));
      return false;
    }
  }, [workspaceId, reset, router]);

  const handleSyncNow = useCallback(async () => {
    setSyncing(true);
    try {
      const res = await forceOfficeConfigSync(workspaceId);
      setConfig(res.config);
      reset(res.config);
      syncOutcomeToast(res.error, res.result?.warnings);
      // Reload agent/skill/project/routine lists when the sync actually
      // changed something so they show up without a manual refresh.
      if (res.result && !res.result.unchanged) {
        router.refresh();
      }
    } catch (err) {
      toast.error(t("office:configSyncSyncFailedWithError", { error: String(err) }));
    } finally {
      setSyncing(false);
    }
  }, [workspaceId, reset, router]);

  return {
    config,
    form,
    loading,
    saving,
    syncing,
    update,
    setProvider,
    handleSave,
    handleDelete,
    handleSyncNow,
  };
}

export type OfficeConfigSyncController = ReturnType<typeof useOfficeConfigSync>;
