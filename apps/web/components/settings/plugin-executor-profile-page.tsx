"use client";

import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { SettingsPageHeader } from "@/components/settings/settings-typography";
import { DeleteProfileDialog } from "@/components/settings/profile-edit/profile-edit-page-chrome";
import { PluginExecutorProfileFields } from "@/components/settings/plugin-executor-profile-fields";
import { localizedExecutorProviderMessage } from "@/lib/executor-provider-display";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { EXECUTOR_ICON_MAP } from "@/lib/executor-icons";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import { RemoteCredentialsCard } from "@/components/settings/profile-edit/remote-credentials-card";
import type { usePluginExecutorCredentials } from "@/components/settings/plugin-executor-credentials";
import { settingsActionClassName } from "@/components/settings/settings-control";
import {
  usePluginExecutorProfileData,
  usePluginExecutorProfileDelete,
  usePluginExecutorProfileSave,
} from "@/components/settings/use-plugin-executor-profile-page";

function SavedFieldsFallback({ profile }: { profile: ExecutorProfile }) {
  const { t } = useTranslation();
  const fields = Object.entries(profile.config ?? {});
  return (
    <div className="space-y-3 rounded-md border p-4">
      <p className="text-sm text-muted-foreground">
        {t("executors:providerSavedFieldsUnavailable")}
      </p>
      {fields.map(([name, value]) => (
        <div key={name} className="min-w-0">
          <dt className="text-xs font-medium text-muted-foreground">{name}</dt>
          <dd className="break-all text-sm">{value}</dd>
        </div>
      ))}
      {Object.entries(profile.secret_fields ?? {})
        .filter(([, configured]) => configured)
        .map(([name]) => (
          <div key={name} className="text-sm text-muted-foreground">
            {t("executors:providerFieldConfigured", { field: name })}
          </div>
        ))}
    </div>
  );
}

function ProfileNameAndRetention({
  name,
  disabled,
  retentionText,
  onNameChange,
}: {
  name: string;
  disabled: boolean;
  retentionText: string | null;
  onNameChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid min-w-0 grid-cols-1 gap-5 md:grid-cols-2">
      <div className="min-w-0 space-y-1.5">
        <Label htmlFor="executor-profile-name">{t("executors:name")}</Label>
        <Input
          id="executor-profile-name"
          value={name}
          disabled={disabled}
          aria-invalid={!name.trim()}
          className="w-full"
          onChange={(event) => onNameChange(event.target.value)}
        />
        {!name.trim() && (
          <p className="text-sm text-destructive">{t("executors:providerProfileNameRequired")}</p>
        )}
      </div>
      {retentionText && (
        <p
          data-testid="executor-profile-retention"
          className="self-end text-sm text-muted-foreground"
        >
          {retentionText}
        </p>
      )}
    </div>
  );
}

function ProfileEditorStatus({
  loading,
  loadError,
  unavailableReason,
}: {
  loading: boolean;
  loadError: string | null;
  unavailableReason: string | null;
}) {
  const { t } = useTranslation();
  return (
    <>
      {loading && (
        <p role="status" className="text-sm text-muted-foreground">
          {t("executors:providerProfileReloading")}
        </p>
      )}
      {loadError && (
        <p role="alert" className="text-sm text-destructive">
          {loadError}
        </p>
      )}
      {unavailableReason && (
        <p
          role="status"
          className="rounded-md border border-amber-500/40 bg-amber-500/5 p-3 text-sm"
        >
          {unavailableReason}
        </p>
      )}
    </>
  );
}

function visibleFieldErrors(save: ReturnType<typeof usePluginExecutorProfileSave>) {
  if (Object.keys(save.fieldErrors).length > 0) return save.fieldErrors;
  return save.dirty ? save.validationErrors : {};
}

function ProfileSchemaFields({
  data,
  save,
}: {
  data: ReturnType<typeof usePluginExecutorProfileData>;
  save: ReturnType<typeof usePluginExecutorProfileSave>;
}) {
  const { t } = useTranslation();
  if (!data.parsed.supported) {
    if (data.loading) return null;
    if (data.currentProfile.provider) {
      return (
        <p role="alert" className="text-sm text-destructive">
          {t("executors:providerSchemaUnsupported")}
        </p>
      );
    }
    return <SavedFieldsFallback profile={data.currentProfile} />;
  }
  return (
    <PluginExecutorProfileFields
      fields={data.parsed.fields}
      provider={data.currentProfile.provider}
      values={data.values}
      configuredSecrets={save.configuredSecrets}
      clearedSecrets={data.clearedSecrets}
      errors={visibleFieldErrors(save)}
      disabled={data.loading || Boolean(data.loadError)}
      onValueChange={save.onValueChange}
      onClearSecret={save.onClearSecret}
      onRestoreSecret={save.onRestoreSecret}
    />
  );
}

function PluginExecutorCredentialsCard({
  credentials,
}: {
  credentials: ReturnType<typeof usePluginExecutorCredentials>;
}) {
  const { items: secrets } = useSecrets();
  const { form, baseline } = credentials;
  return (
    <RemoteCredentialsCard
      isRemote
      selectedIds={form.remoteCredentials}
      baselineSelectedIds={baseline.remoteCredentials}
      onChange={credentials.setRemoteCredentials}
      configBundleIds={form.configBundleIds}
      baselineConfigBundleIds={baseline.configBundleIds}
      onConfigBundleChange={credentials.setConfigBundleIds}
      agentEnvVars={form.agentEnvVars}
      baselineAgentEnvVars={baseline.agentEnvVars}
      onAgentEnvVarChange={credentials.onAgentEnvVarChange}
      secrets={secrets}
      gitIdentityMode={form.gitIdentityMode}
      baselineGitIdentityMode={baseline.gitIdentityMode}
      onGitIdentityModeChange={credentials.setGitIdentityMode}
      gitUserName={form.gitUserName}
      gitUserEmail={form.gitUserEmail}
      baselineGitUserName={baseline.gitUserName}
      baselineGitUserEmail={baseline.gitUserEmail}
      onGitUserNameChange={credentials.setGitUserName}
      onGitUserEmailChange={credentials.setGitUserEmail}
      localGitIdentity={form.localGitIdentity}
    />
  );
}

function ProfileEditorForm({
  data,
  save,
  deletion,
}: {
  data: ReturnType<typeof usePluginExecutorProfileData>;
  save: ReturnType<typeof usePluginExecutorProfileSave>;
  deletion: ReturnType<typeof usePluginExecutorProfileDelete>;
}) {
  const { t } = useTranslation();
  return (
    <Card data-testid="plugin-executor-profile-page">
      <CardContent className="space-y-5 p-4 sm:p-6">
        <ProfileNameAndRetention
          name={data.name}
          disabled={data.loading || Boolean(data.loadError)}
          retentionText={save.retentionText}
          onNameChange={data.setName}
        />
        <ProfileEditorStatus
          loading={data.loading}
          loadError={data.loadError}
          unavailableReason={save.unavailableReason}
        />
        <ProfileSchemaFields data={data} save={save} />
        {save.error && (
          <p role="alert" className="text-sm text-destructive">
            {save.error}
          </p>
        )}
        {deletion.error && (
          <p role="alert" className="text-sm text-destructive">
            {deletion.error}
          </p>
        )}
        <div className="flex flex-col-reverse gap-2 border-t pt-4 sm:flex-row sm:justify-start">
          <Button
            type="button"
            variant="destructive"
            disabled={data.loading || save.saving || deletion.deleting}
            className={settingsActionClassName("cursor-pointer")}
            onClick={() => deletion.setDeleteOpen(true)}
          >
            {t("executors:deleteProfile")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export function PluginExecutorProfilePage({
  executor,
  profile,
}: {
  executor: Executor;
  profile: ExecutorProfile;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const data = usePluginExecutorProfileData(executor, profile, t);
  const save = usePluginExecutorProfileSave({ executor, profile, data, t });
  const deletion = usePluginExecutorProfileDelete(executor, profile);
  const provider = data.currentProfile.provider;
  const providerName = localizedExecutorProviderMessage(
    provider,
    "display_name",
    provider?.display_name ?? t("executors:providerUnavailable"),
    t,
  );
  const providerDescription = localizedExecutorProviderMessage(
    provider,
    "description",
    provider?.description ?? t("executors:providerSavedFieldsUnavailable"),
    t,
  );
  const Icon = EXECUTOR_ICON_MAP.plugin_remote ?? EXECUTOR_ICON_MAP.local;

  return (
    <div className="min-w-0 space-y-6 overflow-x-clip">
      <SettingsPageHeader
        title={
          <span className="flex min-w-0 flex-wrap items-center gap-2 break-words">
            <Icon className="h-5 w-5 text-muted-foreground" />
            <span className="min-w-0 break-words">{data.name || data.currentProfile.name}</span>
            <span className="rounded border px-2 py-0.5 text-xs">{providerName}</span>
          </span>
        }
        description={providerDescription}
        actions={
          <Button
            type="button"
            variant="outline"
            className={settingsActionClassName("cursor-pointer")}
            onClick={() => router.push("/settings/executors")}
          >
            {t("executors:backToExecutors")}
          </Button>
        }
      />
      <ProfileEditorForm data={data} save={save} deletion={deletion} />
      <fieldset
        disabled={
          data.loading || Boolean(data.loadError) || Boolean(save.unavailableReason) || save.saving
        }
        className="min-w-0 disabled:opacity-60"
      >
        <PluginExecutorCredentialsCard credentials={data.credentials} />
      </fieldset>
      <DeleteProfileDialog
        open={deletion.deleteOpen}
        onOpenChange={deletion.setDeleteOpen}
        onDelete={deletion.handleDelete}
        deleting={deletion.deleting}
      />
    </div>
  );
}
