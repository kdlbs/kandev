"use client";

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

type RepoUrlFieldProps = {
  url: string;
  invalid: boolean;
  resolved: string;
  onChange: (value: string) => void;
};

// RepoUrlField is the primary input: a full GitHub link (optionally with
// /tree/<branch>/<directory>) that resolves into the stored owner, repo,
// branch, and directory. The resolved target is echoed underneath so the
// structured fields stay visible to the user.
function RepoUrlField({ url, invalid, resolved, onChange }: RepoUrlFieldProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label htmlFor="workflow-sync-url">{t("workflows:repositoryLink")}</Label>
      <Input
        id="workflow-sync-url"
        data-testid="workflow-sync-url-input"
        placeholder="https://github.com/kdlbs/kandev/tree/main/.kandev/workflows"
        value={url}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={invalid}
      />
      {invalid ? (
        <p className="text-xs text-destructive">{t("workflows:notARecognizedGitHubLink")}</p>
      ) : (
        <p className="text-xs text-muted-foreground" data-testid="workflow-sync-resolved">
          {resolved || t("workflows:pasteGitHubLinkHint")}
        </p>
      )}
    </div>
  );
}

type FieldsProps = {
  form: WorkflowSyncFormState;
  update: <K extends keyof WorkflowSyncFormState>(key: K, value: WorkflowSyncFormState[K]) => void;
};

// PollFields is a single compact row: the auto-sync switch and, when on, the
// interval right beside it. The branch needs no field of its own — it comes
// from the pasted link (or defaults to main) and is echoed in the resolved
// summary under the link input.
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

type WorkflowSyncDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sync: WorkflowSyncController;
};

// WorkflowSyncDialog holds the GitHub Sync configuration form. It closes
// itself after a successful save or removal; failures keep it open with the
// error surfaced via toast.
export function WorkflowSyncDialog({ open, onOpenChange, sync }: WorkflowSyncDialogProps) {
  const { t } = useTranslation();
  const hasConfig = !!sync.config;
  const intervalInvalid =
    sync.form.poll_enabled &&
    (!Number.isInteger(sync.form.interval_seconds) || sync.form.interval_seconds < 60);
  const disableSave =
    sync.saving ||
    sync.loading ||
    sync.urlInvalid ||
    intervalInvalid ||
    !sync.form.repo_owner.trim() ||
    !sync.form.repo_name.trim();
  const resolved = sync.form.repo_owner
    ? t("workflows:syncResolvedTarget", {
        owner: sync.form.repo_owner,
        repo: sync.form.repo_name,
        // `main` is git's default branch name, not copy.
        branch: sync.form.branch || "main",
        directory: sync.form.path || t("workflows:repositoryRoot"),
      })
    : "";

  const handleSave = async () => {
    if (await sync.handleSave()) onOpenChange(false);
  };
  const handleRemove = async () => {
    if (await sync.handleDelete()) onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg" data-testid="workflow-sync-dialog">
        <DialogHeader>
          <DialogTitle>{t("workflows:githubSync")}</DialogTitle>
          <DialogDescription>{t("workflows:githubSyncDescription")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <RepoUrlField
            url={sync.url}
            invalid={sync.urlInvalid}
            resolved={resolved}
            onChange={sync.setUrlInput}
          />
          <PollFields form={sync.form} update={sync.update} />
          <p className="text-xs text-muted-foreground">{t("workflows:syncDirectoryHelp")}</p>
        </div>
        <DialogFooter>
          {hasConfig && (
            <Button
              type="button"
              variant="destructive"
              onClick={handleRemove}
              disabled={sync.saving}
              className="sm:mr-auto cursor-pointer"
              data-testid="workflow-sync-remove"
            >
              {t("workflows:remove")}
            </Button>
          )}
          <Button
            type="button"
            onClick={handleSave}
            disabled={disableSave}
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
