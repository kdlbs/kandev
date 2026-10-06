"use client";

import { useCallback, useEffect, useLayoutEffect, useState } from "react";
import { t } from "@/lib/i18n";
import { useWorkflowSyncLifetime, type WorkflowSyncLifetime } from "./use-workflow-sync-lifetime";
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
  lifetime: WorkflowSyncLifetime,
) {
  useEffect(() => {
    let cancelled = false;
    const id = setInterval(() => {
      if (!lifetime.active) return;
      getWorkflowSyncConfig({ workspaceId })
        .then((cfg) => {
          if (!cancelled && lifetime.active) setConfig(cfg);
        })
        .catch(() => {
          /* transient failures are fine — next tick retries */
        });
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [workspaceId, setConfig, lifetime]);
}

// useWorkflowSyncForm owns the editable form state. The repository link is
// the primary input: owner/repo (GitHub) or project path (GitLab), plus
// directory, only change through it (or a loaded config), while branch and
// interval remain individually editable via `update`. Branch and directory
// are only overwritten when the link actually carried them (/tree/... or
// /blob/... forms), so a bare repo reference keeps the current values.
function useWorkflowSyncForm(lifetime: WorkflowSyncLifetime) {
  const [form, setForm] = useState<WorkflowSyncFormState>(DEFAULT_FORM);
  const [url, setUrl] = useState("");

  const update = useCallback(
    <K extends keyof WorkflowSyncFormState>(key: K, value: WorkflowSyncFormState[K]) => {
      if (lifetime.active) setForm((prev) => ({ ...prev, [key]: value }));
    },
    [lifetime],
  );

  const setUrlInput = useCallback(
    (value: string) => {
      if (!lifetime.active) return;
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
    },
    [lifetime],
  );

  // Switching providers changes what the link/project-path field means, so
  // the field-set that belonged to the other provider is cleared — otherwise
  // a stale repo_owner would sit alongside a new project_path, and Normalize
  // on the backend rejects a request carrying both.
  const setProvider = useCallback(
    (provider: WorkflowSyncProvider) => {
      if (!lifetime.active) return;
      setUrl("");
      setForm((prev) => ({
        ...prev,
        provider,
        repo_owner: "",
        repo_name: "",
        project_path: "",
      }));
    },
    [lifetime],
  );

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
  lifetime: WorkflowSyncLifetime,
) {
  useEffect(() => {
    if (!lifetime.active) return;
    let cancelled = false;
    const ticket = Symbol();
    lifetime.loading = ticket;
    setLoading(true);
    getWorkflowSyncConfig({ workspaceId })
      .then((cfg) => {
        if (cancelled || !lifetime.active) return;
        setConfig(cfg);
        reset(cfg);
      })
      .catch((err) => {
        if (cancelled || !lifetime.active) return;
        toast({
          description: t("workflows:failedToLoadSyncConfig", { error: String(err) }),
          variant: "error",
        });
      })
      .finally(() => {
        if (!cancelled && lifetime.active && lifetime.loading === ticket) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId, lifetime]);
}

export function useWorkflowSync(workspaceId: string) {
  const lifetime = useWorkflowSyncLifetime(workspaceId);
  const { toast } = useToast();
  const router = useRouter();
  const [config, setConfig] = useState<WorkflowSyncConfig | null>(null);
  const { form, url, urlInvalid, update, setUrlInput, setProvider, reset } =
    useWorkflowSyncForm(lifetime);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);

  useLayoutEffect(() => {
    if (!lifetime.active) return;
    setConfig(null);
    reset(null);
    setLoading(true);
    setSaving(false);
    setSyncing(false);
  }, [lifetime, reset]);

  useWorkflowSyncInitialLoad(workspaceId, { setConfig, reset, setLoading, toast }, lifetime);

  useWorkflowSyncConfigRefresh(workspaceId, setConfig, lifetime);

  const actions = useWorkflowSyncActions({
    workspaceId,
    form,
    lifetime,
    setConfig,
    reset,
    toast,
    router,
    setSaving,
    setSyncing,
  });

  return {
    lifetime: lifetime.identity,
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
    ...actions,
  };
}

export type WorkflowSyncController = ReturnType<typeof useWorkflowSync>;

function savePayload(form: WorkflowSyncFormState): WorkflowSyncSetConfigRequest {
  return form.provider === "gitlab"
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
}

type WorkflowSyncActionDeps = Omit<InitialLoadDeps, "setLoading"> & {
  workspaceId: string;
  form: WorkflowSyncFormState;
  lifetime: WorkflowSyncLifetime;
  router: ReturnType<typeof useRouter>;
  setSaving: (value: boolean) => void;
  setSyncing: (value: boolean) => void;
};

function useWorkflowSyncActions({
  workspaceId,
  form,
  lifetime,
  setConfig,
  reset,
  toast,
  router,
  setSaving,
  setSyncing,
}: WorkflowSyncActionDeps) {
  const handleSave = useCallback(async () => {
    if (!lifetime.active) return false;
    const ticket = Symbol();
    lifetime.saving = ticket;
    setSaving(true);
    try {
      const payload = savePayload(form);
      const saved = await setWorkflowSyncConfig(payload, { workspaceId });
      if (lifetime.active) {
        setConfig(saved);
        reset(saved);
        toast({ description: t("workflows:syncConfigSaved"), variant: "success" });
      }
      return true;
    } catch (err) {
      if (lifetime.active)
        toast({ description: t("workflows:saveFailed", { error: String(err) }), variant: "error" });
      return false;
    } finally {
      if (lifetime.active && lifetime.saving === ticket) setSaving(false);
    }
  }, [workspaceId, form, toast, reset, lifetime]);

  const handleDelete = useCallback(async () => {
    if (!lifetime.active) return false;
    try {
      await deleteWorkflowSyncConfig({ workspaceId });
      if (!lifetime.active) return true;
      setConfig(null);
      reset(null);
      toast({ description: t("workflows:syncRemoved") });
      // Released workflows lose their read-only state server-side; reload the
      // page data so the cards unlock without a manual refresh.
      router.refresh();
      return true;
    } catch (err) {
      if (!lifetime.active) return false;
      toast({
        description: t("workflows:syncRemoveFailed", { error: String(err) }),
        variant: "error",
      });
      return false;
    }
  }, [workspaceId, toast, reset, router, lifetime]);

  const handleSyncNow = useCallback(async () => {
    if (!lifetime.active) return;
    const ticket = Symbol();
    lifetime.syncing = ticket;
    setSyncing(true);
    try {
      const res = await forceWorkflowSync({ workspaceId });
      if (!lifetime.active) return;
      setConfig(res.config);
      reset(res.config);
      toast(syncOutcomeToast(res.error, res.result?.warnings));
      // Reload the workflow list when the sync actually changed something so
      // created/updated/deleted workflows show up without a manual refresh.
      if (res.result && !res.result.unchanged) {
        router.refresh();
      }
    } catch (err) {
      if (!lifetime.active) return;
      toast({
        description: t("workflows:syncFailedWithError", { error: String(err) }),
        variant: "error",
      });
    } finally {
      if (lifetime.active && lifetime.syncing === ticket) setSyncing(false);
    }
  }, [workspaceId, toast, reset, router, lifetime]);

  return { handleSave, handleDelete, handleSyncNow };
}
