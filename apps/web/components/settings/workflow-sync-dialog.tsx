"use client";

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
import type {
  WorkflowSyncController,
  WorkflowSyncFormState,
} from "@/hooks/domains/settings/use-workflow-sync";

const HELP_TEXT =
  "The directory should contain workflow export files (.yml/.yaml/.json) in the kandev_workflow format — the same format produced by workflow export.";

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
  return (
    <div className="space-y-1.5">
      <Label htmlFor="workflow-sync-url">Repository link</Label>
      <Input
        id="workflow-sync-url"
        data-testid="workflow-sync-url-input"
        placeholder="https://github.com/kdlbs/kandev/tree/main/.kandev/workflows"
        value={url}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={invalid}
      />
      {invalid ? (
        <p className="text-xs text-destructive">Not a recognized GitHub repository link.</p>
      ) : (
        <p className="text-xs text-muted-foreground" data-testid="workflow-sync-resolved">
          {resolved || "Paste a GitHub link — /tree/… links carry the branch and directory too."}
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
          Auto-sync
        </Label>
        {form.poll_enabled && (
          <div className="ml-auto flex items-center gap-2">
            <Label htmlFor="workflow-sync-interval" className="sr-only">
              Poll interval (seconds)
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
            <span className="text-xs text-muted-foreground">seconds</span>
          </div>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        {form.poll_enabled
          ? "Checks the repository on this interval (minimum 60s)."
          : "Syncs only run when you press Sync now."}
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
  const hasConfig = !!sync.config;
  const disableSave =
    sync.saving || sync.urlInvalid || !sync.form.repo_owner.trim() || !sync.form.repo_name.trim();
  const resolved = sync.form.repo_owner
    ? `Syncing ${sync.form.repo_owner}/${sync.form.repo_name} @ ${sync.form.branch || "main"} — directory ${sync.form.path || "(repository root)"}.`
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
          <DialogTitle>GitHub Sync</DialogTitle>
          <DialogDescription>
            Automatically sync workflow definitions from a GitHub repository into this workspace.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <RepoUrlField
            url={sync.url}
            invalid={sync.urlInvalid}
            resolved={resolved}
            onChange={sync.setUrlInput}
          />
          <PollFields form={sync.form} update={sync.update} />
          <p className="text-xs text-muted-foreground">{HELP_TEXT}</p>
        </div>
        <DialogFooter>
          {hasConfig && (
            <Button
              type="button"
              variant="destructive"
              onClick={handleRemove}
              className="sm:mr-auto cursor-pointer"
              data-testid="workflow-sync-remove"
            >
              Remove
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
            {saveLabel(sync.saving, hasConfig)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function saveLabel(saving: boolean, hasConfig: boolean): string {
  if (saving) return "Saving...";
  return hasConfig ? "Update" : "Save";
}
