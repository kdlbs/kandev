"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useToast } from "@/components/toast-provider";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import { t } from "@/lib/i18n";
import {
  checkPromptSync,
  deletePromptSyncConfig,
  getPromptSyncConfig,
  getPromptSyncKeptVersions,
  runPromptSync,
  setPromptSyncConfig,
} from "@/lib/api/domains/prompt-sync-api";
import type {
  PromptConflictResolutions,
  PromptSyncConfig,
  PromptSyncSetConfigRequest,
} from "@/lib/types/prompt-sync";
import {
  usePromptConflictReview,
  type PromptConflictReview,
} from "@/hooks/domains/settings/use-prompt-conflict-review";
import type { WorkflowSyncProvider } from "@/lib/types/workflow-sync";
import { PUBLIC_GITHUB_HOST } from "@/lib/github/host";
import {
  DEFAULT_PROMPTS_PATH,
  displayUrl,
  formForProvider,
  formFromUrl,
  repoFormFromConfig,
  repoSyncPayload,
  repoTargetMissing,
  repoUrlInvalid,
  type RepoSyncFormState,
} from "@/lib/settings/repo-sync-form";

export type PromptSyncFormState = RepoSyncFormState & { workspace_id: string };

type Draft = { form: PromptSyncFormState; url: string };

function draftFromConfig(cfg: PromptSyncConfig | null, workspaceId: string): Draft {
  const form: PromptSyncFormState = cfg
    ? { ...repoFormFromConfig(cfg), workspace_id: cfg.workspace_id }
    : {
        workspace_id: workspaceId,
        provider: "github",
        github_host: PUBLIC_GITHUB_HOST,
        repo_owner: "",
        repo_name: "",
        project_path: "",
        branch: "main",
        path: DEFAULT_PROMPTS_PATH,
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

function saveToast(error?: string) {
  if (error) {
    return {
      description: t("settings:promptSyncSavedSyncFailed", { error }),
      variant: "error" as const,
    };
  }
  return { description: t("settings:promptSyncSaved"), variant: "success" as const };
}

function draftPayload(draft: Draft): PromptSyncSetConfigRequest {
  return { ...repoSyncPayload(draft.form), workspace_id: draft.form.workspace_id };
}

// reviewUnresolved reopens the review for conflicts a sync left unresolved,
// with their contents from a check of the saved configuration. It reports
// whether the review is open; a failed check leaves the conflicts to the
// Prompts page banner.
async function reviewUnresolved(review: PromptConflictReview, names: string[] | undefined) {
  if (!names?.length) return false;
  try {
    const details = await checkPromptSync();
    if (details.conflicts.length === 0) return false;
    review.open(details.conflicts);
    return true;
  } catch {
    return false;
  }
}

type SaveDeps = {
  latest: React.RefObject<Draft>;
  reset: (cfg: PromptSyncConfig | null) => void;
  setConfig: (cfg: PromptSyncConfig | null) => void;
  toast: ReturnType<typeof useToast>["toast"];
  review: PromptConflictReview;
};

// persistDraft saves the draft, which runs one sync with the choices. It
// resolves true only when the dialog may close: no conflict is left to review
// and the saved draft is still the one on screen.
async function persistDraft(
  submitted: Draft,
  resolutions: PromptConflictResolutions,
  deps: SaveDeps,
): Promise<boolean> {
  const res = await setPromptSyncConfig({ ...draftPayload(submitted), resolutions });
  deps.setConfig(res.config);
  if (await reviewUnresolved(deps.review, res.result?.conflicts)) return false;
  deps.review.close();
  deps.toast(saveToast(res.error));
  if (!draftsEqual(deps.latest.current, submitted)) return false;
  deps.reset(res.config);
  return true;
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

// useSaveHandler returns Save: it first checks the entered source without
// saving. Conflicts open the review step instead of saving; from the review
// step it saves with the choices. It resolves true only when the dialog may
// close; edits made while a request was in flight are kept, not reset.
function useSaveHandler(
  draft: Draft,
  setSaving: (saving: boolean) => void,
  fence: Fence,
  deps: SaveDeps,
) {
  const { latest, reset, setConfig, toast, review } = deps;
  return useCallback(async () => {
    const submitted = draft;
    setSaving(true);
    fence.begin();
    try {
      if (!review.active) {
        const check = await checkPromptSync(draftPayload(submitted));
        if (check.conflicts.length > 0) {
          review.open(check.conflicts);
          return false;
        }
      }
      return await persistDraft(submitted, review.choices, {
        latest,
        reset,
        setConfig,
        toast,
        review,
      });
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
  }, [draft, latest, reset, setConfig, toast, fence, review, setSaving]);
}

// useRemovalReview holds the removal step: the prompts with two versions
// under review, the prompts that only came from the repository, and whether
// to delete those. It is active while either list has entries.
function useRemovalReview() {
  const review = usePromptConflictReview();
  const { close: closeReview } = review;
  const [repositoryOnly, setRepositoryOnly] = useState<string[]>([]);
  const [deleteRepositoryOnly, setDeleteRepositoryOnly] = useState(false);
  const close = useCallback(() => {
    closeReview();
    setRepositoryOnly([]);
    setDeleteRepositoryOnly(false);
  }, [closeReview]);
  return useMemo(
    () => ({
      ...review,
      active: review.active || repositoryOnly.length > 0,
      close,
      repositoryOnly,
      setRepositoryOnly,
      deleteRepositoryOnly,
      setDeleteRepositoryOnly,
    }),
    [review, close, repositoryOnly, deleteRepositoryOnly],
  );
}

export type PromptRemovalReview = ReturnType<typeof useRemovalReview>;

// useRemoval owns Remove sync. startRemoval lists prompts that have both a
// local and a repository version and prompts that only came from the
// repository: with any, it opens the removal step, each two-version prompt
// starting on the version it uses now ("review"); with none, the plain
// confirmation applies ("confirm"); a failed read removes nothing ("failed").
// handleDelete sends the chosen versions and the reviewed prompts to delete.
function useRemoval(
  reset: (cfg: PromptSyncConfig | null) => void,
  setConfig: (cfg: PromptSyncConfig | null) => void,
  toast: ReturnType<typeof useToast>["toast"],
  fence: Fence,
) {
  const removal = useRemovalReview();
  const { open, close, setRepositoryOnly, choices, deleteRepositoryOnly, repositoryOnly } = removal;
  const startRemoval = useCallback(async (): Promise<"review" | "confirm" | "failed"> => {
    try {
      const review = await getPromptSyncKeptVersions();
      if (review.prompts.length === 0 && review.repository_only.length === 0) return "confirm";
      open(review.prompts, "repository");
      setRepositoryOnly(review.repository_only);
      return "review";
    } catch (err) {
      toast({
        description: t("settings:promptSyncRemoveFailed", { error: String(err) }),
        variant: "error",
      });
      return "failed";
    }
  }, [open, setRepositoryOnly, toast]);
  const handleDelete = useCallback(async () => {
    fence.begin();
    try {
      // Only the prompts the user saw are deleted; one synced after the review
      // is released instead.
      await deletePromptSyncConfig({
        choices,
        delete_repository_only: deleteRepositoryOnly ? repositoryOnly : [],
      });
      close();
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
  }, [choices, deleteRepositoryOnly, repositoryOnly, close, setConfig, reset, toast, fence]);
  return { removal, startRemoval, handleDelete };
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
  const {
    draft,
    latest,
    urlInvalid,
    update,
    setUrlInput,
    setProvider,
    reset: resetDraft,
  } = useDraft(defaultWorkspaceId);
  const review = usePromptConflictReview();
  const closeReview = review.close;
  const reset = useCallback(
    (cfg: PromptSyncConfig | null) => {
      closeReview();
      resetDraft(cfg);
    },
    [closeReview, resetDraft],
  );
  const fence = useRequestFence();
  useInitialLoad(setConfig, reset, setLoading, toast, fence);
  useConfigRefresh(!!config, setConfig, fence);

  const handleSave = useSaveHandler(draft, setSaving, fence, {
    latest,
    reset,
    setConfig,
    toast,
    review,
  });

  const { removal, startRemoval, handleDelete } = useRemoval(reset, setConfig, toast, fence);

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
    review,
    backToFields: closeReview,
    removal,
    startRemoval,
    handleSave,
    handleDelete,
    handleSyncNow,
  };
}

export type PromptSyncController = ReturnType<typeof usePromptSync>;
