"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { IconDeviceFloppy } from "@tabler/icons-react";
import { RemoteRepoProviderTabs } from "@/components/task-create-dialog-remote-repo-provider-tabs";
import type { RemoteRepositoryProvider } from "@/hooks/domains/integrations/use-remote-repositories";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { OfficeConfigSyncStatusCard } from "@/components/office/settings/office-config-sync-status-card";
import { useAppStore } from "@/components/state-provider";
import { useOfficeConfigSync } from "@/hooks/domains/office/use-office-config-sync";
import type { OfficeConfigSyncFormState } from "@/hooks/domains/office/use-office-config-sync";
import type { OfficeConfigSyncProvider } from "@/lib/types/office-config-sync";

const SYNC_PROVIDERS: OfficeConfigSyncProvider[] = ["github", "gitlab"];

// RemoteRepoProviderTabs is shared with the task-creation flow, which
// supports a broader, string-extensible provider set than config sync's
// backend contract (github | gitlab only); narrow before forwarding.
function isSyncProvider(provider: RemoteRepositoryProvider): provider is OfficeConfigSyncProvider {
  return provider === "github" || provider === "gitlab";
}

type FieldsProps = {
  form: OfficeConfigSyncFormState;
  update: <K extends keyof OfficeConfigSyncFormState>(
    key: K,
    value: OfficeConfigSyncFormState[K],
  ) => void;
};

function TargetFields({ form, update }: FieldsProps) {
  const { t } = useTranslation();
  if (form.provider === "gitlab") {
    return (
      <div className="space-y-1.5">
        <Label htmlFor="office-config-sync-project-path">{t("office:configSyncProjectPath")}</Label>
        <Input
          id="office-config-sync-project-path"
          placeholder="group/project"
          value={form.project_path}
          onChange={(e) => update("project_path", e.target.value)}
        />
      </div>
    );
  }
  return (
    <div className="grid grid-cols-2 gap-3">
      <div className="space-y-1.5">
        <Label htmlFor="office-config-sync-repo-owner">{t("office:configSyncRepoOwner")}</Label>
        <Input
          id="office-config-sync-repo-owner"
          placeholder="kdlbs"
          value={form.repo_owner}
          onChange={(e) => update("repo_owner", e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="office-config-sync-repo-name">{t("office:configSyncRepoName")}</Label>
        <Input
          id="office-config-sync-repo-name"
          placeholder="kandev-office-config"
          value={form.repo_name}
          onChange={(e) => update("repo_name", e.target.value)}
        />
      </div>
    </div>
  );
}

function BranchDirectoryFields({ form, update }: FieldsProps) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-2 gap-3">
      <div className="space-y-1.5">
        <Label htmlFor="office-config-sync-branch">{t("office:configSyncBranchLabel")}</Label>
        <Input
          id="office-config-sync-branch"
          placeholder="main"
          value={form.branch}
          onChange={(e) => update("branch", e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="office-config-sync-directory">{t("office:configSyncDirectoryLabel")}</Label>
        <Input
          id="office-config-sync-directory"
          placeholder={t("office:configSyncRepositoryRoot")}
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
          id="office-config-sync-poll-toggle"
          checked={form.poll_enabled}
          onCheckedChange={(checked) => update("poll_enabled", checked)}
          className="cursor-pointer"
        />
        <Label htmlFor="office-config-sync-poll-toggle" className="cursor-pointer">
          {t("office:configSyncAutoSync")}
        </Label>
        {form.poll_enabled && (
          <div className="ml-auto flex items-center gap-2">
            <Label htmlFor="office-config-sync-interval" className="sr-only">
              {t("office:configSyncPollIntervalSeconds")}
            </Label>
            <Input
              id="office-config-sync-interval"
              type="number"
              min={60}
              className="w-24"
              value={form.interval_seconds}
              onChange={(e) => update("interval_seconds", Number(e.target.value) || 0)}
            />
            <span className="text-xs text-muted-foreground">{t("office:configSyncSeconds")}</span>
          </div>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        {form.poll_enabled
          ? t("office:configSyncPollEnabledHint")
          : t("office:configSyncPollDisabledHint")}
      </p>
    </div>
  );
}

function isSaveDisabled(
  form: OfficeConfigSyncFormState,
  saving: boolean,
  loading: boolean,
): boolean {
  const targetMissing =
    form.provider === "gitlab"
      ? !form.project_path.trim()
      : !form.repo_owner.trim() || !form.repo_name.trim();
  const intervalInvalid =
    form.poll_enabled && (!Number.isInteger(form.interval_seconds) || form.interval_seconds < 60);
  return saving || loading || targetMissing || intervalInvalid;
}

function ConfigureForm({
  form,
  update,
  setProvider,
  saving,
  loading,
  onSave,
}: FieldsProps & {
  setProvider: (provider: OfficeConfigSyncProvider) => void;
  saving: boolean;
  loading: boolean;
  onSave: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t("office:configSyncDescription")}</p>
      <div className="space-y-1.5">
        <Label>{t("office:configSyncProviderLabel")}</Label>
        <div className="overflow-hidden rounded-md border">
          <RemoteRepoProviderTabs
            providers={SYNC_PROVIDERS}
            value={form.provider}
            onChange={(provider) => {
              if (isSyncProvider(provider)) setProvider(provider);
            }}
          />
        </div>
      </div>
      <TargetFields form={form} update={update} />
      <BranchDirectoryFields form={form} update={update} />
      <PollFields form={form} update={update} />
      <Button
        onClick={onSave}
        disabled={isSaveDisabled(form, saving, loading)}
        className="cursor-pointer"
        data-testid="office-config-sync-save"
      >
        <IconDeviceFloppy className="h-4 w-4 mr-1.5" />
        {saving ? t("office:configSyncSaving") : t("common:save")}
      </Button>
    </div>
  );
}

function RemoveAction({ saving, onRemove }: { saving: boolean; onRemove: () => Promise<boolean> }) {
  const { t } = useTranslation();
  const [confirming, setConfirming] = useState(false);
  if (!confirming) {
    return (
      <Button
        type="button"
        variant="destructive"
        size="sm"
        onClick={() => setConfirming(true)}
        disabled={saving}
        className="cursor-pointer"
        data-testid="office-config-sync-remove"
      >
        {t("office:configSyncRemove")}
      </Button>
    );
  }
  return (
    <InlineConfirmActions
      density="touch"
      testId="office-config-sync-remove-confirmation"
      ariaLabel={t("office:configSyncRemove")}
      description={t("office:configSyncRemoveConfirm")}
      cancelLabel={t("common:cancel")}
      confirmLabel={t("office:configSyncRemove")}
      confirmTestId="office-config-sync-remove-confirm"
      onCancel={() => setConfirming(false)}
      onConfirm={async () => {
        if (!(await onRemove())) return Promise.reject();
      }}
    />
  );
}

// OfficeConfigSyncSection lets a workspace pull its agent/skill/project/
// routine configuration from a GitHub or GitLab repository on a schedule.
// This is Office's own status card and form — deliberately not sharing code
// with the unrelated internal/workflowsync feature's UI.
export function OfficeConfigSyncSection() {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((s) => s.workspaces?.activeId ?? "");
  const sync = useOfficeConfigSync(activeWorkspaceId);

  return (
    <section className="space-y-4">
      <p className="text-xs text-muted-foreground">{t("office:configSyncDescription")}</p>
      {sync.config ? (
        <div className="space-y-3">
          <OfficeConfigSyncStatusCard
            config={sync.config}
            syncing={sync.syncing}
            onSyncNow={sync.handleSyncNow}
          />
          <details className="text-sm">
            <summary className="cursor-pointer text-muted-foreground">
              {t("office:configSyncEditConfiguration")}
            </summary>
            <div className="mt-3 space-y-3">
              <ConfigureForm
                form={sync.form}
                update={sync.update}
                setProvider={sync.setProvider}
                saving={sync.saving}
                loading={sync.loading}
                onSave={sync.handleSave}
              />
              <RemoveAction saving={sync.saving} onRemove={sync.handleDelete} />
            </div>
          </details>
        </div>
      ) : (
        <ConfigureForm
          form={sync.form}
          update={sync.update}
          setProvider={sync.setProvider}
          saving={sync.saving}
          loading={sync.loading}
          onSave={sync.handleSave}
        />
      )}
    </section>
  );
}
