"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useToast } from "@/components/toast-provider";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import { t } from "@/lib/i18n";
import {
  deletePromptSyncConfig,
  getPromptSyncConfig,
  runPromptSync,
  setPromptSyncConfig,
} from "@/lib/api/domains/prompt-sync-api";
import type { PromptSyncConfig } from "@/lib/types/prompt-sync";
import type { WorkflowSyncProvider } from "@/lib/types/workflow-sync";
import { PUBLIC_GITHUB_HOST } from "@/lib/github/host";
import {
  displayUrl,
  formForProvider,
  formFromUrl,
  repoSyncPayload,
  repoTargetMissing,
  repoUrlInvalid,
  type RepoSyncFormState,
} from "@/lib/settings/repo-sync-form";

export type PromptSyncFormState = RepoSyncFormState & { workspace_id: string };

type Draft = { form: PromptSyncFormState; url: string };

// The default directory is a repository path, not copy.
const DEFAULT_PATH = ".kandev/prompts"; // i18n-exempt: repository path

function draftFromConfig(cfg: PromptSyncConfig | null, workspaceId: string): Draft {
  const form: PromptSyncFormState = cfg
    ? {
        workspace_id: cfg.workspace_id,
        provider: cfg.provider,
        github_host: cfg.provider === "gitlab" ? "" : (cfg.github_host ?? PUBLIC_GITHUB_HOST),
        repo_owner: cfg.repo_owner,
        repo_name: cfg.repo_name,
        project_path: cfg.project_path,
        branch: cfg.branch,
        path: cfg.path,
        interval_seconds: cfg.interval_seconds,
        poll_enabled: cfg.poll_enabled,
      }
    : {
        workspace_id: workspaceId,
        provider: "github",
        github_host: PUBLIC_GITHUB_HOST,
        repo_owner: "",
        repo_name: "",
        project_path: "",
        branch: "main",
        path: DEFAULT_PATH,
        interval_seconds: 300,
        poll_enabled: true,
      };
  return { form, url: displayUrl(form) };
}

// RequestFence lets a mutation invalidate configuration reads that started
// before it: a read applies its reply only if no mutation began or is still
// running since the read started.
type RequestFence = { epoch: number; mutating: number };

function useRequestFence() {
  const fence = useRef<RequestFence>({ epoch: 0, mutating: 0 });
  return useMemo(
    () => ({
      begin: () => {
        fence.current.epoch += 1;
        fence.current.mutating += 1;
      },
      end: () => {
        fence.current.mutating -= 1;
      },
      snapshot: () => fence.current.epoch,
      current: (epoch: number) => fence.current.epoch === epoch && fence.current.mutating === 0,
    }),
    [],
  );
}

type Fence = ReturnType<typeof useRequestFence>;

function useConfigRefresh(
  enabled: boolean,
  setConfig: (cfg: PromptSyncConfig | null) => void,
  fence: Fence,
) {
  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const id = setInterval(() => {
      const epoch = fence.snapshot();
      getPromptSyncConfig()
        .then((cfg) => {
          if (!cancelled && fence.current(epoch)) setConfig(cfg);
        })
        .catch(() => {
          /* transient failures are fine; the next tick retries */
        });
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [enabled, setConfig, fence]);
}

function useInitialLoad(
  setConfig: (cfg: PromptSyncConfig | null) => void,
  reset: (cfg: PromptSyncConfig | null) => void,
  setLoading: (loading: boolean) => void,
  toast: ReturnType<typeof useToast>["toast"],
  fence: Fence,
) {
  const resetRef = useRef(reset);
  useEffect(() => {
    resetRef.current = reset;
  }, [reset]);
  useEffect(() => {
    let cancelled = false;
    const epoch = fence.snapshot();
    getPromptSyncConfig()
      .then((cfg) => {
        if (cancelled || !fence.current(epoch)) return;
        setConfig(cfg);
        resetRef.current(cfg);
      })
      .catch((err) => {
        if (cancelled) return;
        toast({
          description: t("settings:promptSyncLoadFailed", { error: String(err) }),
          variant: "error",
        });
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [setConfig, setLoading, toast, fence]);
}

function useDraft(defaultWorkspaceId: string) {
  const [draft, setDraft] = useState<Draft>(() => draftFromConfig(null, defaultWorkspaceId));
  const { status } = useGitHubStatus(draft.form.workspace_id || null);
  const configuredHost = status?.automation?.github_host;
  const latest = useRef(draft);
  useLayoutEffect(() => {
    latest.current = draft;
  }, [draft]);
  // A link pasted before the workspace's GitHub host was known is parsed
  // again once it is, so the user does not have to retype it.
  // Only a link that has not produced repository coordinates is re-parsed,
  // so fields edited after a successful parse keep their values.
  useEffect(() => {
    if (!configuredHost) return;
    setDraft((current) => {
      if (!repoTargetMissing(current.form)) return current;
      const form = formFromUrl(current.form, current.url, configuredHost);
      return form === current.form ? current : { ...current, form };
    });
  }, [configuredHost]);
  const update = useCallback(
    <K extends keyof PromptSyncFormState>(key: K, value: PromptSyncFormState[K]) =>
      setDraft((current) => ({ ...current, form: { ...current.form, [key]: value } })),
    [],
  );
  const setUrlInput = useCallback(
    (value: string) =>
      setDraft((current) => ({
        form: value.trim()
          ? formFromUrl(current.form, value, configuredHost)
          : { ...current.form, repo_owner: "", repo_name: "", project_path: "" },
        url: value,
      })),
    [configuredHost],
  );
  const setProvider = useCallback(
    (provider: WorkflowSyncProvider) =>
      setDraft((current) => ({
        url: "",
        form: formForProvider(current.form, provider, configuredHost),
      })),
    [configuredHost],
  );
  const reset = useCallback(
    (cfg: PromptSyncConfig | null) => setDraft(draftFromConfig(cfg, defaultWorkspaceId)),
    [defaultWorkspaceId],
  );
  const urlInvalid = repoUrlInvalid(draft.form, draft.url, configuredHost);
  return { draft, latest, urlInvalid, update, setUrlInput, setProvider, reset };
}

function draftsEqual(left: Draft, right: Draft): boolean {
  return left.url === right.url && JSON.stringify(left.form) === JSON.stringify(right.form);
}

function syncToast(error?: string, warnings?: string[]) {
  if (error)
    return { description: t("settings:promptSyncFailed", { error }), variant: "error" as const };
  if (warnings?.length) {
    return {
      description: t("settings:promptSyncCompletedWithWarnings"),
      variant: "default" as const,
    };
  }
  return { description: t("settings:promptSyncCompleted"), variant: "success" as const };
}

// usePromptSync owns the install-wide Prompt Sync configuration, its editable
// draft, and the save, remove, and Sync now actions. Prompt list changes
// arrive through the existing prompts.changed WebSocket refresh.
export function usePromptSync(defaultWorkspaceId: string) {
  const { toast } = useToast();
  const [config, setConfig] = useState<PromptSyncConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const { draft, latest, urlInvalid, update, setUrlInput, setProvider, reset } =
    useDraft(defaultWorkspaceId);
  const fence = useRequestFence();
  useInitialLoad(setConfig, reset, setLoading, toast, fence);
  useConfigRefresh(!!config, setConfig, fence);

  // handleSave resolves true only when the saved draft is still the one on
  // screen; edits made while the request was in flight are kept, not reset.
  const handleSave = useCallback(async () => {
    const submitted = draft;
    setSaving(true);
    fence.begin();
    try {
      const saved = await setPromptSyncConfig({
        ...repoSyncPayload(submitted.form),
        workspace_id: submitted.form.workspace_id,
      });
      setConfig(saved);
      toast({ description: t("settings:promptSyncSaved"), variant: "success" });
      if (!draftsEqual(latest.current, submitted)) return false;
      reset(saved);
      return true;
    } catch (err) {
      toast({
        description: t("settings:promptSyncSaveFailed", { error: String(err) }),
        variant: "error",
      });
      return false;
    } finally {
      fence.end();
      setSaving(false);
    }
  }, [draft, latest, reset, toast, fence]);

  const handleDelete = useCallback(async () => {
    fence.begin();
    try {
      await deletePromptSyncConfig();
      setConfig(null);
      reset(null);
      toast({ description: t("settings:promptSyncRemoved") });
      return true;
    } catch (err) {
      toast({
        description: t("settings:promptSyncRemoveFailed", { error: String(err) }),
        variant: "error",
      });
      return false;
    } finally {
      fence.end();
    }
  }, [reset, toast, fence]);

  const handleSyncNow = useCallback(async () => {
    setSyncing(true);
    fence.begin();
    try {
      const res = await runPromptSync();
      setConfig(res.config);
      toast(syncToast(res.error, res.result?.warnings));
    } catch (err) {
      toast({
        description: t("settings:promptSyncFailed", { error: String(err) }),
        variant: "error",
      });
    } finally {
      fence.end();
      setSyncing(false);
    }
  }, [toast, fence]);

  return {
    config,
    form: draft.form,
    url: draft.url,
    urlInvalid,
    loading,
    saving,
    syncing,
    update,
    setUrlInput,
    setProvider,
    reset,
    handleSave,
    handleDelete,
    handleSyncNow,
  };
}

export type PromptSyncController = ReturnType<typeof usePromptSync>;
