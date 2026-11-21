"use client";

import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import type { WorkflowSyncProvider } from "@/lib/types/workflow-sync";
import type { RepoSyncFormState, RepoSyncTarget } from "@/lib/settings/repo-sync-form";
import { RemoteRepoProviderTabs } from "@/components/task-create-dialog-remote-repo-provider-tabs";
import type { RemoteRepositoryProvider } from "@/hooks/domains/integrations/use-remote-repositories";

const SYNC_PROVIDERS: RemoteRepositoryProvider[] = ["github", "gitlab"];

type ProviderFieldProps = {
  provider: WorkflowSyncProvider;
  onChange: (provider: WorkflowSyncProvider) => void;
};

// ProviderField picks the repository host a sync source reads from; saving a
// different provider replaces the saved source.
export function ProviderField({ provider, onChange }: ProviderFieldProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label>{t("workflows:syncProviderLabel")}</Label>
      <div className="overflow-hidden rounded-md border">
        <RemoteRepoProviderTabs
          providers={SYNC_PROVIDERS}
          value={provider}
          onChange={(next) => onChange(next as WorkflowSyncProvider)}
        />
      </div>
    </div>
  );
}

type RepoUrlFieldProps = {
  idPrefix: string;
  provider: WorkflowSyncProvider;
  url: string;
  invalid: boolean;
  resolved: string;
  onChange: (value: string) => void;
};

// RepoUrlField captures the repository identity — owner/repo for GitHub,
// project_path for GitLab. Pasting a full link (including a /tree/.../-/tree/
// suffix) still convenience-fills Branch and Directory below, but those are
// their own directly-editable fields: a branch name can itself contain
// slashes (e.g. "features/my-ticket"), which makes splitting a combined
// "project/branch/path" string ambiguous by construction — no client-side
// parse can always place that boundary correctly. If a paste gets it wrong,
// fix it directly in Branch/Directory rather than needing a different link.
// GitLab sync has no host of its own (it always uses the workspace's
// configured GitLab connection, including self-managed instances), so a
// saved GitLab config redisplays as a bare "group/project" reference.
export function RepoUrlField({
  idPrefix,
  provider,
  url,
  invalid,
  resolved,
  onChange,
}: RepoUrlFieldProps) {
  const { t } = useTranslation();
  const providerLabel = provider === "gitlab" ? "GitLab" : "GitHub";
  // GitLab's placeholder is a bare path, not a URL: a full link (any host —
  // gitlab.com or a self-managed instance) is accepted, but nothing here
  // needs a host at all, since syncing always uses this workspace's own
  // connected GitLab host regardless of what's typed in this field. Showing
  // a "gitlab.com" example would wrongly suggest a self-managed host has to
  // match it.
  const placeholder = provider === "gitlab" ? "group/project" : "https://github.com/kdlbs/kandev";
  const hint =
    provider === "gitlab"
      ? t("workflows:pasteGitLabPathHint")
      : t("workflows:pasteRepositoryLinkHint", { provider: providerLabel });
  return (
    <div className="space-y-1.5">
      <Label htmlFor={`${idPrefix}-url`}>
        {provider === "gitlab" ? t("workflows:projectPathOrLink") : t("workflows:repositoryLink")}
      </Label>
      <Input
        id={`${idPrefix}-url`}
        data-testid={`${idPrefix}-url-input`}
        placeholder={placeholder}
        value={url}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={invalid}
      />
      {invalid ? (
        <p className="text-xs text-destructive">
          {t("workflows:notARecognizedRepositoryLink", { provider: providerLabel })}
        </p>
      ) : (
        <p className="text-xs text-muted-foreground" data-testid={`${idPrefix}-resolved`}>
          {resolved || hint}
        </p>
      )}
    </div>
  );
}

type FieldsProps = {
  idPrefix: string;
  form: RepoSyncFormState;
  update: <K extends keyof RepoSyncFormState>(key: K, value: RepoSyncFormState[K]) => void;
};

// BranchDirectoryFields exposes branch and directory as their own inputs
// rather than only ever deriving them from a parsed link. A pasted link
// still convenience-fills both, but a branch name with a slash in it (a
// common convention — "features/TICKET-123") can't always be told apart
// from the directory that follows it, so these fields are the way to
// directly correct a wrong guess instead of needing a differently-shaped
// link.
export function BranchDirectoryFields({
  idPrefix,
  form,
  update,
  directoryPlaceholder,
}: FieldsProps & { directoryPlaceholder: string }) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-2 gap-3">
      <div className="space-y-1.5">
        <Label htmlFor={`${idPrefix}-branch`}>{t("workflows:branchLabel")}</Label>
        <Input
          id={`${idPrefix}-branch`}
          data-testid={`${idPrefix}-branch-input`}
          placeholder="main"
          value={form.branch}
          onChange={(e) => update("branch", e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`${idPrefix}-directory`}>{t("workflows:directoryLabel")}</Label>
        <Input
          id={`${idPrefix}-directory`}
          data-testid={`${idPrefix}-directory-input`}
          placeholder={directoryPlaceholder}
          value={form.path}
          onChange={(e) => update("path", e.target.value)}
        />
      </div>
    </div>
  );
}

// PollFields is a single compact row: the auto-sync switch and, when on, the
// interval right beside it.
export function PollFields({ idPrefix, form, update }: FieldsProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-center gap-3">
        <Switch
          id={`${idPrefix}-poll-toggle`}
          data-testid={`${idPrefix}-poll-toggle`}
          checked={form.poll_enabled}
          onCheckedChange={(checked) => update("poll_enabled", checked)}
          className="cursor-pointer"
        />
        <Label htmlFor={`${idPrefix}-poll-toggle`} className="cursor-pointer">
          {t("workflows:autoSync")}
        </Label>
        {form.poll_enabled && (
          <div className="ml-auto flex items-center gap-2">
            <Label htmlFor={`${idPrefix}-interval`} className="sr-only">
              {t("workflows:pollIntervalSeconds")}
            </Label>
            <Input
              id={`${idPrefix}-interval`}
              data-testid={`${idPrefix}-interval-input`}
              type="number"
              min={60}
              className="w-24"
              value={form.interval_seconds}
              onChange={(e) => update("interval_seconds", Number(e.target.value) || 0)}
            />
            <span className="text-xs text-muted-foreground">{t("workflows:seconds")}</span>
          </div>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        {form.poll_enabled ? t("workflows:pollEnabledHint") : t("workflows:pollDisabledHint")}
      </p>
    </div>
  );
}

export function resolvedTarget(t: TFunction, form: RepoSyncTarget): string {
  const directory = form.path || t("workflows:repositoryRoot");
  // `main` is git's default branch name, not copy.
  const branch = form.branch || "main";
  if (form.provider === "gitlab") {
    return form.project_path
      ? t("workflows:syncResolvedTargetGitLab", {
          projectPath: form.project_path,
          branch,
          directory,
        })
      : "";
  }
  return form.repo_owner
    ? t("workflows:syncResolvedTarget", {
        owner: form.repo_owner,
        repo: form.repo_name,
        branch,
        directory,
      })
    : "";
}
