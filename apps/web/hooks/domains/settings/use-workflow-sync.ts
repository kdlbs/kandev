"use client";

import { useCallback, useEffect, useState } from "react";
import { t } from "@/lib/i18n";
import { useToast } from "@/components/toast-provider";
import { useRouter } from "@/lib/routing/client-router";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import {
  getWorkflowSyncConfig,
  setWorkflowSyncConfig,
  deleteWorkflowSyncConfig,
  forceWorkflowSync,
} from "@/lib/api/domains/workflow-sync-api";
import type {
  WorkflowSyncConfig,
  WorkflowSyncProvider,
  WorkflowSyncSetConfigRequest,
} from "@/lib/types/workflow-sync";
import { buildGitHubRepoUrl, parseGitHubRepoUrl } from "@/lib/utils/github-repo-url";
import { parseGitLabProjectUrl } from "@/lib/utils/gitlab-repo-url";

export type WorkflowSyncFormState = {
  provider: WorkflowSyncProvider;
  repo_owner: string;
  repo_name: string;
  project_path: string;
  branch: string;
  path: string;
  interval_seconds: number;
  poll_enabled: boolean;
};

const DEFAULT_FORM: WorkflowSyncFormState = {
  provider: "github",
  repo_owner: "",
  repo_name: "",
  project_path: "",
  branch: "main",
  path: ".kandev/workflows",
  interval_seconds: 300,
  poll_enabled: true,
};

function configToForm(cfg: WorkflowSyncConfig | null): WorkflowSyncFormState {
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

// displayUrl redisplays only the repo identity (owner/repo, or GitLab
// project_path) — never branch or directory. Branch and directory are their
// own directly-editable fields (see BranchField/DirectoryField in the
// dialog), specifically because a branch name can itself contain slashes
// (e.g. "features/my-ticket"), which makes splitting a combined
// project+branch+path string ambiguous by construction — no client-side
// parse can always get it right. Baking branch/path back into this field
// would let it silently disagree with the authoritative Branch/Directory
// fields after a direct edit.
function displayUrl(
  form: Pick<WorkflowSyncFormState, "provider" | "repo_owner" | "repo_name" | "project_path">,
): string {
  if (form.provider === "gitlab") {
    return form.project_path;
  }
  return form.repo_owner
    ? buildGitHubRepoUrl({ owner: form.repo_owner, repo: form.repo_name })
    : "";
}

function parseRepoUrl(provider: WorkflowSyncProvider, value: string) {
  return provider === "gitlab" ? parseGitLabProjectUrl(value) : parseGitHubRepoUrl(value);
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
    let cancelled = false;
    const id = setInterval(() => {
      getWorkflowSyncConfig({ workspaceId })
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

// useWorkflowSyncForm owns the editable form state. The repository link is
// the primary input: owner/repo (GitHub) or project path (GitLab), plus
// directory, only change through it (or a loaded config), while branch and
// interval remain individually editable via `update`. Branch and directory
// are only overwritten when the link actually carried them (/tree/... or
// /blob/... forms), so a bare repo reference keeps the current values.
function useWorkflowSyncForm() {
  const [form, setForm] = useState<WorkflowSyncFormState>(DEFAULT_FORM);
  const [url, setUrl] = useState("");

  const update = useCallback(
    <K extends keyof WorkflowSyncFormState>(key: K, value: WorkflowSyncFormState[K]) =>
      setForm((prev) => ({ ...prev, [key]: value })),
    [],
  );

  const setUrlInput = useCallback((value: string) => {
    setUrl(value);
    setForm((prev) => {
      const parsed = parseRepoUrl(prev.provider, value);
      if (!parsed) return prev;
      if (prev.provider === "gitlab" && "projectPath" in parsed) {
        return {
          ...prev,
          project_path: parsed.projectPath,
          branch: parsed.branch ?? prev.branch,
          path: parsed.path ?? prev.path,
        };
      }
      if (prev.provider === "github" && "owner" in parsed) {
        return {
          ...prev,
          repo_owner: parsed.owner,
          repo_name: parsed.repo,
          branch: parsed.branch ?? prev.branch,
          path: parsed.path ?? prev.path,
        };
      }
      return prev;
    });
  }, []);

  // Switching providers changes what the link/project-path field means, so
  // the field-set that belonged to the other provider is cleared — otherwise
  // a stale repo_owner would sit alongside a new project_path, and Normalize
  // on the backend rejects a request carrying both.
  const setProvider = useCallback((provider: WorkflowSyncProvider) => {
    setUrl("");
    setForm((prev) => ({
      ...prev,
      provider,
      repo_owner: "",
      repo_name: "",
      project_path: "",
    }));
  }, []);

  // reset re-derives both the structured form and the displayed link from a
  // loaded/saved config (or clears them when the config was removed).
  const reset = useCallback((cfg: WorkflowSyncConfig | null) => {
    const next = configToForm(cfg);
    setForm(next);
    setUrl(displayUrl(next));
  }, []);

  const urlInvalid = !!url.trim() && !parseRepoUrl(form.provider, url);
  return { form, url, urlInvalid, update, setUrlInput, setProvider, reset };
}

type SyncToast = { description: string; variant?: "success" | "error" | "default" };

function syncOutcomeToast(error: string | undefined, warnings: string[] | undefined): SyncToast {
  if (error)
    return { description: t("workflows:syncFailedWithError", { error }), variant: "error" };
  if (warnings?.length) {
    return { description: t("workflows:syncCompletedWithWarnings"), variant: "default" };
  }
  return { description: t("workflows:workflowSyncCompleted"), variant: "success" };
}

type InitialLoadDeps = {
  setConfig: (cfg: WorkflowSyncConfig | null) => void;
  reset: (cfg: WorkflowSyncConfig | null) => void;
  setLoading: (loading: boolean) => void;
  toast: ReturnType<typeof useToast>["toast"];
};

// useWorkflowSyncInitialLoad fetches the stored config on mount / workspace
// change. It guards against stale responses: if the hook re-renders for a
// different workspace while a fetch is in flight, the old workspace's config
// must not populate the new one's form.
function useWorkflowSyncInitialLoad(
  workspaceId: string,
  { setConfig, reset, setLoading, toast }: InitialLoadDeps,
) {
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getWorkflowSyncConfig({ workspaceId })
      .then((cfg) => {
        if (cancelled) return;
        setConfig(cfg);
        reset(cfg);
      })
      .catch((err) => {
        if (cancelled) return;
        toast({
          description: t("workflows:failedToLoadSyncConfig", { error: String(err) }),
          variant: "error",
        });
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId]);
}

export function useWorkflowSync(workspaceId: string) {
  const { toast } = useToast();
  const router = useRouter();
  const [config, setConfig] = useState<WorkflowSyncConfig | null>(null);
  const { form, url, urlInvalid, update, setUrlInput, setProvider, reset } = useWorkflowSyncForm();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);

  useWorkflowSyncInitialLoad(workspaceId, { setConfig, reset, setLoading, toast });

  useWorkflowSyncConfigRefresh(workspaceId, setConfig);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const payload: WorkflowSyncSetConfigRequest =
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
      const saved = await setWorkflowSyncConfig(payload, { workspaceId });
      setConfig(saved);
      reset(saved);
      toast({ description: t("workflows:syncConfigSaved"), variant: "success" });
      return true;
    } catch (err) {
      toast({ description: t("workflows:saveFailed", { error: String(err) }), variant: "error" });
      return false;
    } finally {
      setSaving(false);
    }
  }, [workspaceId, form, toast, reset]);

  const handleDelete = useCallback(async () => {
    if (!confirm(t("workflows:removeSyncConfirm"))) {
      return false;
    }
    try {
      await deleteWorkflowSyncConfig({ workspaceId });
      setConfig(null);
      reset(null);
      toast({ description: t("workflows:syncRemoved") });
      // Released workflows lose their read-only state server-side; reload the
      // page data so the cards unlock without a manual refresh.
      router.refresh();
      return true;
    } catch (err) {
      toast({
        description: t("workflows:syncRemoveFailed", { error: String(err) }),
        variant: "error",
      });
      return false;
    }
  }, [workspaceId, toast, reset, router]);

  const handleSyncNow = useCallback(async () => {
    setSyncing(true);
    try {
      const res = await forceWorkflowSync({ workspaceId });
      setConfig(res.config);
      reset(res.config);
      toast(syncOutcomeToast(res.error, res.result?.warnings));
      // Reload the workflow list when the sync actually changed something so
      // created/updated/deleted workflows show up without a manual refresh.
      if (res.result && !res.result.unchanged) {
        router.refresh();
      }
    } catch (err) {
      toast({
        description: t("workflows:syncFailedWithError", { error: String(err) }),
        variant: "error",
      });
    } finally {
      setSyncing(false);
    }
  }, [workspaceId, toast, reset, router]);

  return {
    config,
    form,
    url,
    urlInvalid,
    loading,
    saving,
    syncing,
    update,
    setUrlInput,
    setProvider,
    handleSave,
    handleDelete,
    handleSyncNow,
  };
}

export type WorkflowSyncController = ReturnType<typeof useWorkflowSync>;
