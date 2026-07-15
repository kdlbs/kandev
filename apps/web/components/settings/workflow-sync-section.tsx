"use client";

import { IconBrandGithub, IconLoader2, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { WorkflowSyncStatusBanner } from "@/components/settings/workflow-sync-status-banner";
import {
  useWorkflowSync,
  type WorkflowSyncFormState,
} from "@/hooks/domains/settings/use-workflow-sync";

const HELP_TEXT =
  "The directory should contain workflow export files (.yml/.yaml/.json) in the kandev_workflow format — the same format produced by workflow export.";

type FieldsProps = {
  form: WorkflowSyncFormState;
  loading: boolean;
  update: <K extends keyof WorkflowSyncFormState>(key: K, value: WorkflowSyncFormState[K]) => void;
};

type RepoUrlFieldProps = {
  url: string;
  invalid: boolean;
  resolved: string;
  loading: boolean;
  onChange: (value: string) => void;
};

// RepoUrlField is the primary input: a full GitHub link (optionally with
// /tree/<branch>/<directory>) that resolves into the stored owner, repo,
// branch, and directory. The resolved target is echoed underneath so the
// hidden structured fields stay visible to the user.
function RepoUrlField({ url, invalid, resolved, loading, onChange }: RepoUrlFieldProps) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor="workflow-sync-url">Repository link</Label>
      <Input
        id="workflow-sync-url"
        data-testid="workflow-sync-url-input"
        placeholder="https://github.com/kdlbs/kandev/tree/main/.kandev/workflows"
        value={url}
        onChange={(e) => onChange(e.target.value)}
        disabled={loading}
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

function BranchIntervalFields({ form, loading, update }: FieldsProps) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor="workflow-sync-branch">Branch</Label>
        <Input
          id="workflow-sync-branch"
          data-testid="workflow-sync-branch-input"
          placeholder="main"
          value={form.branch}
          onChange={(e) => update("branch", e.target.value)}
          disabled={loading}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="workflow-sync-interval">Poll interval (seconds)</Label>
        <Input
          id="workflow-sync-interval"
          data-testid="workflow-sync-interval-input"
          type="number"
          min={60}
          value={form.interval_seconds}
          onChange={(e) => update("interval_seconds", Number(e.target.value) || 0)}
          disabled={loading}
        />
        <p className="text-xs text-muted-foreground">Minimum 60 seconds.</p>
      </div>
    </div>
  );
}

type ActionBarProps = {
  hasConfig: boolean;
  saving: boolean;
  syncing: boolean;
  loading: boolean;
  disableSave: boolean;
  onSave: () => void;
  onSyncNow: () => void;
  onDelete: () => void;
};

function saveLabel(saving: boolean, hasConfig: boolean): string {
  if (saving) return "Saving...";
  return hasConfig ? "Update" : "Save";
}

function ActionBar({
  hasConfig,
  saving,
  syncing,
  loading,
  disableSave,
  onSave,
  onSyncNow,
  onDelete,
}: ActionBarProps) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        onClick={onSave}
        disabled={disableSave}
        className="cursor-pointer"
        data-testid="workflow-sync-save"
      >
        {saveLabel(saving, hasConfig)}
      </Button>
      {hasConfig && (
        <Button
          type="button"
          variant="outline"
          onClick={onSyncNow}
          disabled={syncing || loading}
          className="cursor-pointer"
          data-testid="workflow-sync-now"
        >
          {syncing ? (
            <IconLoader2 className="h-4 w-4 mr-2 animate-spin" />
          ) : (
            <IconRefresh className="h-4 w-4 mr-2" />
          )}
          Sync now
        </Button>
      )}
      {hasConfig && (
        <Button
          type="button"
          variant="destructive"
          onClick={onDelete}
          className="ml-auto cursor-pointer"
          data-testid="workflow-sync-remove"
        >
          Remove
        </Button>
      )}
    </div>
  );
}

// WorkflowSyncSection renders the "Sync from GitHub" settings card: a config
// form (repo owner/name/branch/path/interval), a status banner reflecting the
// most recent sync attempt, and Save / Sync now / Remove actions. The form is
// always visible — pre-filled once a config loads — so configuring and
// re-configuring share the same fields (mirrors the Jira settings pattern).
export function WorkflowSyncSection({ workspaceId }: { workspaceId: string }) {
  const s = useWorkflowSync(workspaceId);
  const hasConfig = !!s.config;
  const disableSave =
    s.saving || s.urlInvalid || !s.form.repo_owner.trim() || !s.form.repo_name.trim();
  const resolved = s.form.repo_owner
    ? `Syncing ${s.form.repo_owner}/${s.form.repo_name} — directory ${s.form.path || "(repository root)"}.`
    : "";

  return (
    <SettingsSection
      icon={<IconBrandGithub className="h-5 w-5" />}
      title="Sync from GitHub"
      description="Automatically sync workflow definitions from a GitHub repository into this workspace."
    >
      <Card data-testid="workflow-sync-section">
        <CardContent className="space-y-4 pt-6">
          <WorkflowSyncStatusBanner config={s.config} />
          <RepoUrlField
            url={s.url}
            invalid={s.urlInvalid}
            resolved={resolved}
            loading={s.loading}
            onChange={s.setUrlInput}
          />
          <BranchIntervalFields form={s.form} loading={s.loading} update={s.update} />
          <p className="text-xs text-muted-foreground">{HELP_TEXT}</p>
          <Separator />
          <ActionBar
            hasConfig={hasConfig}
            saving={s.saving}
            syncing={s.syncing}
            loading={s.loading}
            disableSave={disableSave}
            onSave={s.handleSave}
            onSyncNow={s.handleSyncNow}
            onDelete={s.handleDelete}
          />
        </CardContent>
      </Card>
    </SettingsSection>
  );
}
