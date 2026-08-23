"use client";

import { useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import type { TFunction } from "i18next";
import type {
  WorkflowSyncController,
  WorkflowSyncFormState,
} from "@/hooks/domains/settings/use-workflow-sync";
import type { WorkflowSyncProvider } from "@/lib/types/workflow-sync";
import { RemoteRepoProviderTabs } from "@/components/task-create-dialog-remote-repo-provider-tabs";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import type { RemoteRepositoryProvider } from "@/hooks/domains/integrations/use-remote-repositories";

const WORKFLOW_EXPORT_FORMAT = "kandev_workflow";
const WORKFLOW_EXPORT_EXTENSIONS = ".yml/.yaml/.json";
const SYNC_PROVIDERS: RemoteRepositoryProvider[] = ["github", "gitlab"];
const REMOVE_TEST_ID = "workflow-sync-remove";
const REMOVE_CONFIRMATION_TEST_ID = "workflow-sync-remove-confirmation";
const REMOVE_CONFIRM_TEST_ID = "workflow-sync-remove-confirm";

type ProviderFieldProps = {
  provider: WorkflowSyncProvider;
  onChange: (provider: WorkflowSyncProvider) => void;
};

// ProviderField lets the user pick which host workflow sync reads from.
// Only one source syncs per workspace — picking a different provider than
// the currently-saved config replaces it on the next save (see the warning
// surfaced in WorkflowSyncDialog).
function ProviderField({ provider, onChange }: ProviderFieldProps) {
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
function RepoUrlField({ provider, url, invalid, resolved, onChange }: RepoUrlFieldProps) {
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
      <Label htmlFor="workflow-sync-url">
        {provider === "gitlab" ? t("workflows:projectPathOrLink") : t("workflows:repositoryLink")}
      </Label>
      <Input
        id="workflow-sync-url"
        data-testid="workflow-sync-url-input"
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
        <p className="text-xs text-muted-foreground" data-testid="workflow-sync-resolved">
          {resolved || hint}
        </p>
      )}
    </div>
  );
}

type FieldsProps = {
  form: WorkflowSyncFormState;
  update: <K extends keyof WorkflowSyncFormState>(key: K, value: WorkflowSyncFormState[K]) => void;
};

// BranchDirectoryFields exposes branch and directory as their own inputs
// rather than only ever deriving them from a parsed link. A pasted link
// still convenience-fills both, but a branch name with a slash in it (a
// common convention — "features/TICKET-123") can't always be told apart
// from the directory that follows it, so these fields are the way to
// directly correct a wrong guess instead of needing a differently-shaped
// link.
function BranchDirectoryFields({ form, update }: FieldsProps) {
  const { t } = useTranslation();
  const directoryPlaceholder = ".kandev/workflows";
  return (
    <div className="grid grid-cols-2 gap-3">
      <div className="space-y-1.5">
        <Label htmlFor="workflow-sync-branch">{t("workflows:branchLabel")}</Label>
        <Input
          id="workflow-sync-branch"
          data-testid="workflow-sync-branch-input"
          placeholder="main"
          value={form.branch}
          onChange={(e) => update("branch", e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="workflow-sync-directory">{t("workflows:directoryLabel")}</Label>
        <Input
          id="workflow-sync-directory"
          data-testid="workflow-sync-directory-input"
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
function PollFields({ form, update }: FieldsProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-center gap-3">
        <Switch
          id="workflow-sync-poll-toggle"
          data-testid="workflow-sync-poll-toggle"
          checked={form.poll_enabled}
          onCheckedChange={(checked) => update("poll_enabled", checked)}
          className="cursor-pointer"
        />
        <Label htmlFor="workflow-sync-poll-toggle" className="cursor-pointer">
          {t("workflows:autoSync")}
        </Label>
        {form.poll_enabled && (
          <div className="ml-auto flex items-center gap-2">
            <Label htmlFor="workflow-sync-interval" className="sr-only">
              {t("workflows:pollIntervalSeconds")}
            </Label>
            <Input
              id="workflow-sync-interval"
              data-testid="workflow-sync-interval-input"
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

function resolvedTarget(t: TFunction, form: WorkflowSyncFormState): string {
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

function isSaveDisabled(sync: WorkflowSyncController, intervalInvalid: boolean): boolean {
  const targetMissing =
    sync.form.provider === "gitlab"
      ? !sync.form.project_path.trim()
      : !sync.form.repo_owner.trim() || !sync.form.repo_name.trim();
  return sync.saving || sync.loading || sync.urlInvalid || intervalInvalid || targetMissing;
}

type WorkflowSyncRemovalActionsProps = {
  hasConfig: boolean;
  confirming: boolean;
  saving: boolean;
  onRequest: () => void;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
};

function WorkflowSyncRemovalActions({
  hasConfig,
  confirming,
  saving,
  onRequest,
  onCancel,
  onConfirm,
}: WorkflowSyncRemovalActionsProps) {
  const { t } = useTranslation();
  if (!hasConfig) return null;

  const removeLabel = t("workflows:remove");
  if (confirming) {
    return (
      <InlineConfirmActions
        density="touch"
        testId={REMOVE_CONFIRMATION_TEST_ID}
        ariaLabel={removeLabel}
        description={t("workflows:removeSyncConfirm")}
        cancelLabel={t("common:cancel")}
        confirmLabel={removeLabel}
        confirmAriaLabel={removeLabel}
        confirmTestId={REMOVE_CONFIRM_TEST_ID}
        onCancel={onCancel}
        onConfirm={onConfirm}
      />
    );
  }

  return (
    <Button
      type="button"
      variant="destructive"
      onClick={onRequest}
      disabled={saving}
      className="sm:mr-auto cursor-pointer"
      data-testid={REMOVE_TEST_ID}
    >
      {removeLabel}
    </Button>
  );
}

function useClearWorkflowSyncRemovalConfirmation(
  open: boolean,
  sync: WorkflowSyncController,
  setRemoveConfirming: Dispatch<SetStateAction<boolean>>,
) {
  // Clear a pending confirmation when the dialog closes or when its configured
  // target changes. Status-only refreshes leave target fields unchanged, so
  // they do not interrupt an action the user is already confirming.
  useEffect(() => {
    setRemoveConfirming(false);
  }, [
    open,
    sync.config?.workspace_id,
    sync.config?.provider,
    sync.config?.repo_owner,
    sync.config?.repo_name,
    sync.config?.project_path,
    sync.config?.branch,
    sync.config?.path,
    sync.form.provider,
    sync.form.repo_owner,
    sync.form.repo_name,
    sync.form.project_path,
    sync.form.branch,
    sync.form.path,
    setRemoveConfirming,
  ]);
}

type WorkflowSyncDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sync: WorkflowSyncController;
};

// WorkflowSyncDialog holds the workflow sync configuration form. It closes
// itself after a successful save or removal; failures keep it open with the
// error surfaced via toast.
export function WorkflowSyncDialog({ open, onOpenChange, sync }: WorkflowSyncDialogProps) {
  const { t } = useTranslation();
  const [removeConfirming, setRemoveConfirming] = useState(false);
  const hasConfig = !!sync.config;
  const switchingProvider = hasConfig && sync.config?.provider !== sync.form.provider;
  const intervalInvalid =
    sync.form.poll_enabled &&
    (!Number.isInteger(sync.form.interval_seconds) || sync.form.interval_seconds < 60);
  const disableSave = isSaveDisabled(sync, intervalInvalid);
  const resolved = resolvedTarget(t, sync.form);

  useClearWorkflowSyncRemovalConfirmation(open, sync, setRemoveConfirming);

  const handleSave = async () => {
    if (await sync.handleSave()) onOpenChange(false);
  };
  const handleRemove = async () => {
    if (await sync.handleDelete()) {
      onOpenChange(false);
      return;
    }
    // InlineConfirmActions restores its own shell when this callback rejects.
    // Keep parent confirmation state mounted so the existing failure toast and
    // retry path remain inside this dialog.
    return Promise.reject();
  };
  const handleDialogOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) setRemoveConfirming(false);
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleDialogOpenChange}>
      <DialogContent
        className="sm:max-w-lg"
        data-testid="workflow-sync-dialog"
        onEscapeKeyDown={(event) => {
          if (!removeConfirming) return;
          event.preventDefault();
          setRemoveConfirming(false);
        }}
      >
        <DialogHeader>
          <DialogTitle>{t("workflows:syncTitle")}</DialogTitle>
          <DialogDescription>{t("workflows:syncDescription")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <ProviderField provider={sync.form.provider} onChange={sync.setProvider} />
          {switchingProvider && (
            <p
              className="text-xs text-destructive"
              data-testid="workflow-sync-provider-switch-warning"
            >
              {t("workflows:switchProviderWarning")}
            </p>
          )}
          <RepoUrlField
            provider={sync.form.provider}
            url={sync.url}
            invalid={sync.urlInvalid}
            resolved={resolved}
            onChange={sync.setUrlInput}
          />
          <BranchDirectoryFields form={sync.form} update={sync.update} />
          <PollFields form={sync.form} update={sync.update} />
          <p className="text-xs text-muted-foreground">
            {t("workflows:syncDirectoryHelp", {
              format: WORKFLOW_EXPORT_FORMAT,
              extensions: WORKFLOW_EXPORT_EXTENSIONS,
            })}
          </p>
        </div>
        <DialogFooter>
          <WorkflowSyncRemovalActions
            hasConfig={hasConfig}
            confirming={removeConfirming}
            saving={sync.saving}
            onRequest={() => setRemoveConfirming(true)}
            onCancel={() => setRemoveConfirming(false)}
            onConfirm={handleRemove}
          />
          <Button
            type="button"
            onClick={handleSave}
            disabled={disableSave || removeConfirming}
            className="cursor-pointer"
            data-testid="workflow-sync-save"
            data-dialog-default-action
          >
            {saveLabel(t, sync.saving, hasConfig)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function saveLabel(t: TFunction, saving: boolean, hasConfig: boolean): string {
  if (saving) return t("workflows:saving");
  return hasConfig ? t("workflows:update") : t("workflows:save");
}
