"use client";

import { IconBell } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { SettingsGroup } from "@/components/settings/settings-group";
import { type AppriseFormMode } from "@/components/settings/notifications-settings-actions";
import { webhookHeadersError } from "@/components/settings/notifications-settings-webhook-helpers";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import type { NotificationProvider } from "@/lib/types/http";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";

type WebhookProviderFormProps = {
  mode: AppriseFormMode;
  name: string;
  url: string;
  headers: string;
  secret: string;
  onNameChange: (value: string) => void;
  onUrlChange: (value: string) => void;
  onHeadersChange: (value: string) => void;
  onSecretChange: (value: string) => void;
  onSubmit: () => void;
  onCancel: () => void;
  nameIsDirty?: boolean;
  urlIsDirty?: boolean;
  headersIsDirty?: boolean;
  secretIsDirty?: boolean;
  formIsDirty?: boolean;
  showSubmit?: boolean;
};

function WebhookProviderForm({
  mode,
  name,
  url,
  headers,
  secret,
  onNameChange,
  onUrlChange,
  onHeadersChange,
  onSecretChange,
  onSubmit,
  onCancel,
  nameIsDirty = false,
  urlIsDirty = false,
  headersIsDirty = false,
  secretIsDirty = false,
  formIsDirty = nameIsDirty || urlIsDirty || headersIsDirty || secretIsDirty,
  showSubmit = true,
}: WebhookProviderFormProps) {
  const { t } = useTranslation();
  const headersInvalid = webhookHeadersError(headers) !== null;
  return (
    <div
      className="rounded-lg border border-dashed border-muted p-4 space-y-3"
      data-settings-dirty={formIsDirty}
      data-settings-dirty-level="container"
    >
      <div className="text-sm font-medium">{t("settings:webhookProvider")}</div>
      <Input
        value={name}
        onChange={(event) => onNameChange(event.target.value)}
        placeholder={t("settings:providerName")}
        data-settings-dirty={nameIsDirty}
      />
      <Input
        value={url}
        onChange={(event) => onUrlChange(event.target.value)}
        placeholder={t("settings:webhookUrl")}
        data-settings-dirty={urlIsDirty}
      />
      <Textarea
        value={headers}
        onChange={(event) => onHeadersChange(event.target.value)}
        placeholder={t("settings:webhookHeadersPlaceholder")}
        rows={2}
        data-settings-dirty={headersIsDirty}
        aria-invalid={headersInvalid}
        aria-describedby={headersInvalid ? "webhook-headers-error" : undefined}
      />
      {headersInvalid && (
        <p id="webhook-headers-error" className="text-sm text-destructive" role="alert">
          {t("settings:webhookHeadersInvalid")}
        </p>
      )}
      <Input
        type="password"
        value={secret}
        onChange={(event) => onSecretChange(event.target.value)}
        placeholder={t("settings:webhookSecret")}
        data-settings-dirty={secretIsDirty}
      />
      <div className="flex items-center gap-2">
        {showSubmit && (
          <Button className="cursor-pointer" onClick={onSubmit}>
            {mode === "create" ? t("settings:addProvider") : t("settings:done")}
          </Button>
        )}
        <Button variant="ghost" className="cursor-pointer" onClick={onCancel}>
          {t("settings:cancel")}
        </Button>
      </div>
    </div>
  );
}

function webhookValues(provider: NotificationProvider) {
  const url = typeof provider.config?.url === "string" ? provider.config.url : "";
  const secret = typeof provider.config?.secret === "string" ? provider.config.secret : "";
  return { url, secret };
}

function WebhookProviderActions({
  provider,
  onOpenForm,
  onDeleteProvider,
  onTestProvider,
}: {
  provider: NotificationProvider;
  onOpenForm: (mode: AppriseFormMode, provider?: NotificationProvider) => void;
  onDeleteProvider: (providerId: string) => void;
  onTestProvider: (providerId: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="outline"
              size="icon"
              className="cursor-pointer"
              aria-label={t("settings:sendTestNotificationFor", { name: provider.name })}
              onClick={() => void onTestProvider(provider.id)}
            >
              <IconBell className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("settings:sendTestNotification")}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
      <Button
        variant="outline"
        className="cursor-pointer"
        onClick={() => onOpenForm("edit", provider)}
      >
        {t("settings:edit")}
      </Button>
      <Button
        variant="outline"
        className="cursor-pointer"
        onClick={() => onDeleteProvider(provider.id)}
      >
        {t("settings:remove")}
      </Button>
    </div>
  );
}

function WebhookProviderSummary({
  provider,
  onOpenForm,
  onDeleteProvider,
  onTestProvider,
}: {
  provider: NotificationProvider;
  onOpenForm: (mode: AppriseFormMode, provider?: NotificationProvider) => void;
  onDeleteProvider: (providerId: string) => void;
  onTestProvider: (providerId: string) => Promise<void>;
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="min-w-0 space-y-1">
        <div className="font-medium">{provider.name}</div>
        {/* Product name — never translated. */}
        <div className="truncate text-xs text-muted-foreground">Webhook</div>
      </div>
      <WebhookProviderActions
        provider={provider}
        onOpenForm={onOpenForm}
        onDeleteProvider={onDeleteProvider}
        onTestProvider={onTestProvider}
      />
    </div>
  );
}

function WebhookProviderCard({
  provider,
  baseline,
  formMode,
  activeWebhookId,
  name,
  url,
  headers,
  headerDrafts,
  secret,
  onNameChange,
  onUrlChange,
  onHeadersChange,
  onSecretChange,
  onOpenForm,
  onCloseForm,
  onCancelForm,
  onDeleteProvider,
  onTestProvider,
}: {
  provider: NotificationProvider;
  baseline: NotificationProvider | undefined;
  formMode: AppriseFormMode;
  activeWebhookId: string | null;
  name: string;
  url: string;
  headers: string;
  headerDrafts: Record<string, string>;
  secret: string;
  onNameChange: (value: string) => void;
  onUrlChange: (value: string) => void;
  onHeadersChange: (value: string) => void;
  onSecretChange: (value: string) => void;
  onOpenForm: (mode: AppriseFormMode, provider?: NotificationProvider) => void;
  onCloseForm: () => void;
  onCancelForm: () => void;
  onDeleteProvider: (providerId: string) => void;
  onTestProvider: (providerId: string) => Promise<void>;
}) {
  const isEditing = formMode === "edit" && activeWebhookId === provider.id;
  const currentValues = webhookValues(provider);
  const baselineValues = webhookValues(baseline ?? provider);
  const configIsDirty =
    currentValues.url !== baselineValues.url ||
    currentValues.secret !== baselineValues.secret ||
    JSON.stringify(provider.config?.headers ?? {}) !==
      JSON.stringify(baseline?.config?.headers ?? {});
  const nameIsDirty = provider.name !== baseline?.name;
  return (
    <div
      className="rounded-lg border border-muted p-4 space-y-3"
      data-settings-dirty={nameIsDirty || configIsDirty || headerDrafts[provider.id] !== undefined}
      data-settings-dirty-level="container"
    >
      {isEditing ? (
        <WebhookProviderForm
          mode="edit"
          name={name}
          url={url}
          headers={headers}
          secret={secret}
          onNameChange={onNameChange}
          onUrlChange={onUrlChange}
          onHeadersChange={onHeadersChange}
          onSecretChange={onSecretChange}
          onSubmit={onCloseForm}
          onCancel={onCancelForm}
          nameIsDirty={nameIsDirty}
          urlIsDirty={currentValues.url !== baselineValues.url}
          headersIsDirty={configIsDirty || headerDrafts[provider.id] !== undefined}
          secretIsDirty={currentValues.secret !== baselineValues.secret}
        />
      ) : (
        <WebhookProviderSummary
          provider={provider}
          onOpenForm={onOpenForm}
          onDeleteProvider={onDeleteProvider}
          onTestProvider={onTestProvider}
        />
      )}
    </div>
  );
}

export type WebhookProvidersSectionProps = {
  providers: NotificationProvider[];
  baselineProviders: NotificationProvider[];
  formMode: AppriseFormMode;
  activeWebhookId: string | null;
  name: string;
  url: string;
  headers: string;
  headerDrafts: Record<string, string>;
  secret: string;
  showForm: boolean;
  appriseCreateFormOpen: boolean;
  setName: (value: string) => void;
  setUrl: (value: string) => void;
  setHeaders: (value: string) => void;
  setSecret: (value: string) => void;
  onNameEdit: (providerId: string, value: string) => void;
  onUrlEdit: (providerId: string, value: string) => void;
  onHeadersEdit: (providerId: string, value: string) => void;
  onSecretEdit: (providerId: string, value: string) => void;
  onOpenForm: (mode: AppriseFormMode, provider?: NotificationProvider) => void;
  onCloseForm: () => void;
  onCancelForm: () => void;
  onDeleteProvider: (providerId: string) => void;
  onTestProvider: (providerId: string) => Promise<void>;
};

export function WebhookProvidersSection(props: WebhookProvidersSectionProps) {
  const { t } = useTranslation();
  const {
    providers,
    baselineProviders,
    formMode,
    activeWebhookId,
    name,
    url,
    headers,
    headerDrafts,
    secret,
    showForm,
    appriseCreateFormOpen,
    onOpenForm,
    onCloseForm,
    onCancelForm,
  } = props;
  return (
    <SettingsGroup
      title={t("settings:webhooks")}
      description={t("settings:webhooksDescription")}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.notificationProviders}
    >
      {providers.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("settings:noWebhookProviders")}</p>
      )}
      {providers.map((provider) => (
        <WebhookProviderCard
          key={provider.id}
          provider={provider}
          baseline={baselineProviders.find((candidate) => candidate.id === provider.id)}
          formMode={formMode}
          activeWebhookId={activeWebhookId}
          name={name}
          url={url}
          headers={headers}
          headerDrafts={headerDrafts}
          secret={secret}
          onNameChange={(value) => props.onNameEdit(provider.id, value)}
          onUrlChange={(value) => props.onUrlEdit(provider.id, value)}
          onHeadersChange={(value) => props.onHeadersEdit(provider.id, value)}
          onSecretChange={(value) => props.onSecretEdit(provider.id, value)}
          onOpenForm={onOpenForm}
          onCloseForm={onCloseForm}
          onCancelForm={onCancelForm}
          onDeleteProvider={props.onDeleteProvider}
          onTestProvider={props.onTestProvider}
        />
      ))}
      <div className="space-y-3">
        <Button
          variant="outline"
          className="cursor-pointer"
          onClick={() => onOpenForm("create")}
          disabled={showForm || appriseCreateFormOpen}
        >
          {t("settings:addWebhookProvider")}
        </Button>
        {showForm && formMode === "create" && (
          <WebhookProviderForm
            mode="create"
            name={name}
            url={url}
            headers={headers}
            secret={secret}
            onNameChange={props.setName}
            onUrlChange={props.setUrl}
            onHeadersChange={props.setHeaders}
            onSecretChange={props.setSecret}
            onSubmit={onCloseForm}
            onCancel={onCancelForm}
            formIsDirty
            nameIsDirty={name.length > 0}
            urlIsDirty={url.length > 0}
            headersIsDirty={headers.length > 0}
            secretIsDirty={secret.length > 0}
            showSubmit={false}
          />
        )}
      </div>
    </SettingsGroup>
  );
}
