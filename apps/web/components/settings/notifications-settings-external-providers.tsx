"use client";

import { type FormEvent } from "react";
import { IconBell, IconRefresh } from "@tabler/icons-react";
import { Trans, useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { SettingsTarget } from "@/components/settings/settings-target";
import { type AppriseFormMode } from "@/components/settings/notifications-settings-actions";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import type { NotificationProvider } from "@/lib/types/http";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";

function AppriseProviderCardActions({
  provider,
  onOpenForm,
  onDeleteProvider,
  onTestProvider,
}: {
  provider: NotificationProvider;
  onOpenForm: (mode: AppriseFormMode, provider: NotificationProvider) => void;
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
              className="h-8 w-8 cursor-pointer"
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
        size="sm"
        className="cursor-pointer"
        onClick={() => onOpenForm("edit", provider)}
      >
        {t("settings:edit")}
      </Button>
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer"
        onClick={() => onDeleteProvider(provider.id)}
      >
        {t("settings:remove")}
      </Button>
    </div>
  );
}

function AppriseProviderList({
  providers,
  baselineProviders,
  appriseFormMode,
  activeAppriseId,
  appriseName,
  appriseUrls,
  onNameChange,
  onUrlsChange,
  onAppriseNameEdit,
  onAppriseEdit,
  onOpenForm,
  onCloseForm,
  onCancelForm,
  onDeleteProvider,
  onTestProvider,
  onTextareaInput,
}: {
  providers: NotificationProvider[];
  baselineProviders: NotificationProvider[];
  appriseFormMode: AppriseFormMode;
  activeAppriseId: string | null;
  appriseName: string;
  appriseUrls: string;
  onNameChange: (value: string) => void;
  onUrlsChange: (value: string) => void;
  onAppriseNameEdit: (providerId: string, value: string) => void;
  onAppriseEdit: (providerId: string, value: string) => void;
  onOpenForm: (mode: AppriseFormMode, provider?: NotificationProvider) => void;
  onCloseForm: () => void;
  onCancelForm: () => void;
  onDeleteProvider: (providerId: string) => void;
  onTestProvider: (providerId: string) => Promise<void>;
  onTextareaInput: (event: FormEvent<HTMLTextAreaElement>) => void;
}) {
  return (
    <>
      {providers.map((provider) => {
        const isEditing = appriseFormMode === "edit" && activeAppriseId === provider.id;
        const baseline = baselineProviders.find((candidate) => candidate.id === provider.id);
        const nameIsDirty = isEditing && provider.name !== baseline?.name;
        const urlsIsDirty =
          isEditing &&
          JSON.stringify(provider.config?.urls ?? []) !==
            JSON.stringify(baseline?.config?.urls ?? []);
        return (
          <div
            key={provider.id}
            className="rounded-lg border border-muted p-4 space-y-3"
            data-settings-dirty={nameIsDirty || urlsIsDirty}
            data-settings-dirty-level="container"
          >
            {isEditing ? (
              <AppriseProviderForm
                mode="edit"
                name={appriseName}
                urls={appriseUrls}
                onNameChange={(value) => {
                  onNameChange(value);
                  onAppriseNameEdit(provider.id, value);
                }}
                onUrlsChange={(value) => {
                  onUrlsChange(value);
                  onAppriseEdit(provider.id, value);
                }}
                onSubmit={onCloseForm}
                onCancel={onCancelForm}
                onInput={onTextareaInput}
                nameIsDirty={nameIsDirty}
                urlsIsDirty={urlsIsDirty}
              />
            ) : (
              <div className="flex items-center justify-between gap-4">
                <div className="space-y-1 flex-1">
                  <div className="font-medium">{provider.name}</div>
                  {/* Product name — never translated (docs/i18n.md "Do not translate"). */}
                  <div className="text-xs text-muted-foreground">Apprise</div>
                </div>
                <AppriseProviderCardActions
                  provider={provider}
                  onOpenForm={onOpenForm}
                  onDeleteProvider={onDeleteProvider}
                  onTestProvider={onTestProvider}
                />
              </div>
            )}
          </div>
        );
      })}
    </>
  );
}

type ExternalProvidersSectionProps = {
  appriseAvailable: boolean;
  appriseProviders: NotificationProvider[];
  baselineProviders: NotificationProvider[];
  appriseFormMode: AppriseFormMode;
  activeAppriseId: string | null;
  appriseName: string;
  appriseUrls: string;
  showAppriseForm: boolean;
  notificationProvidersLoaded: boolean;
  notificationProvidersLoading: boolean;
  appriseRescanPending: boolean;
  appriseRescanError: boolean;
  appriseRescanResult: boolean | null;
  setAppriseName: (v: string) => void;
  setAppriseUrls: (v: string) => void;
  onAppriseNameEdit: (id: string, v: string) => void;
  onAppriseEdit: (id: string, v: string) => void;
  onOpenForm: (mode: AppriseFormMode, provider?: NotificationProvider) => void;
  onCloseForm: () => void;
  onCancelForm: () => void;
  onDeleteProvider: (id: string) => void;
  onTestProvider: (id: string) => Promise<void>;
  onRescanApprise: () => Promise<void>;
};

function AppriseRescanControls({
  notificationProvidersLoaded,
  notificationProvidersLoading,
  appriseRescanPending,
  appriseRescanError,
  appriseRescanResult,
  onRescanApprise,
}: Pick<
  ExternalProvidersSectionProps,
  | "notificationProvidersLoaded"
  | "notificationProvidersLoading"
  | "appriseRescanPending"
  | "appriseRescanError"
  | "appriseRescanResult"
  | "onRescanApprise"
>) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-col items-start gap-1 md:items-end">
      <Button
        variant="outline"
        className="min-h-11 cursor-pointer md:min-h-9"
        data-testid="apprise-rescan"
        disabled={
          !notificationProvidersLoaded || notificationProvidersLoading || appriseRescanPending
        }
        onClick={() => void onRescanApprise()}
      >
        <IconRefresh className={appriseRescanPending ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
        {appriseRescanPending ? t("settings:checkingApprise") : t("settings:rescanApprise")}
      </Button>
      <p className="text-xs text-muted-foreground">{t("settings:appriseDetectionServer")}</p>
      {appriseRescanError && (
        <p className="text-xs text-amber-600" role="alert" data-testid="apprise-rescan-error">
          {t("settings:appriseRescanFailed")}
        </p>
      )}
      {!appriseRescanPending && !appriseRescanError && appriseRescanResult !== null && (
        <p
          className="text-xs text-muted-foreground"
          role="status"
          data-testid="apprise-rescan-result"
        >
          {appriseRescanResult ? t("settings:appriseDetected") : t("settings:appriseNotDetected")}
        </p>
      )}
    </div>
  );
}

function AppriseProviderSectionBody({
  appriseAvailable,
  appriseProviders,
  baselineProviders,
  appriseFormMode,
  activeAppriseId,
  appriseName,
  appriseUrls,
  showAppriseForm,
  setAppriseName,
  setAppriseUrls,
  onAppriseNameEdit,
  onAppriseEdit,
  onOpenForm,
  onCloseForm,
  onCancelForm,
  onDeleteProvider,
  onTestProvider,
}: Omit<
  ExternalProvidersSectionProps,
  | "notificationProvidersLoaded"
  | "notificationProvidersLoading"
  | "appriseRescanPending"
  | "appriseRescanError"
  | "appriseRescanResult"
  | "onRescanApprise"
>) {
  const { t } = useTranslation();
  return (
    <>
      {!appriseAvailable && (
        <p className="text-xs text-muted-foreground">
          {/* The link is part of the sentence, so the whole notice is one
              message and the <2> tag addresses the anchor by child index. */}
          <Trans i18nKey="settings:appriseNotInstalled">
            Apprise is not installed yet. You can add it later to enable remote notifications.{" "}
            <a
              className="underline"
              href="https://github.com/caronc/apprise?tab=readme-ov-file#installation"
              target="_blank"
              rel="noreferrer"
            >
              View installation instructions
            </a>
            .
          </Trans>
        </p>
      )}
      {appriseProviders.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("settings:noAppriseProviders")}</p>
      )}
      <AppriseProviderList
        providers={appriseProviders}
        baselineProviders={baselineProviders}
        appriseFormMode={appriseFormMode}
        activeAppriseId={activeAppriseId}
        appriseName={appriseName}
        appriseUrls={appriseUrls}
        onNameChange={setAppriseName}
        onUrlsChange={setAppriseUrls}
        onAppriseNameEdit={onAppriseNameEdit}
        onAppriseEdit={onAppriseEdit}
        onOpenForm={onOpenForm}
        onCloseForm={onCloseForm}
        onCancelForm={onCancelForm}
        onDeleteProvider={onDeleteProvider}
        onTestProvider={onTestProvider}
        onTextareaInput={handleTextareaInput}
      />
      <div className="space-y-3">
        {appriseAvailable && (
          <Button
            variant="outline"
            className="cursor-pointer"
            onClick={() => onOpenForm("create")}
            disabled={showAppriseForm}
          >
            {t("settings:addAppriseProvider")}
          </Button>
        )}
        {showAppriseForm && appriseFormMode === "create" && (
          <AppriseProviderForm
            mode="create"
            name={appriseName}
            urls={appriseUrls}
            onNameChange={setAppriseName}
            onUrlsChange={setAppriseUrls}
            onSubmit={onCloseForm}
            onCancel={onCancelForm}
            onInput={handleTextareaInput}
            formIsDirty
            nameIsDirty={appriseName.length > 0}
            urlsIsDirty={appriseUrls.length > 0}
            showSubmit={false}
          />
        )}
      </div>
    </>
  );
}

export function ExternalProvidersSection(props: ExternalProvidersSectionProps) {
  const { t } = useTranslation();
  return (
    <SettingsTarget targetId={GENERAL_SETTINGS_TARGETS.notificationProviders} className="space-y-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="text-sm font-medium">{t("settings:externalProviders")}</div>
          <p className="text-xs text-muted-foreground" data-testid="external-providers-description">
            {t("settings:externalProvidersDescription")}
          </p>
        </div>
        <AppriseRescanControls
          notificationProvidersLoaded={props.notificationProvidersLoaded}
          notificationProvidersLoading={props.notificationProvidersLoading}
          appriseRescanPending={props.appriseRescanPending}
          appriseRescanError={props.appriseRescanError}
          appriseRescanResult={props.appriseRescanResult}
          onRescanApprise={props.onRescanApprise}
        />
      </div>
      <AppriseProviderSectionBody {...props} />
    </SettingsTarget>
  );
}

type AppriseProviderFormProps = {
  mode: AppriseFormMode;
  name: string;
  urls: string;
  onNameChange: (value: string) => void;
  onUrlsChange: (value: string) => void;
  onSubmit: () => void | Promise<void>;
  onCancel: () => void;
  onInput: (event: FormEvent<HTMLTextAreaElement>) => void;
  nameIsDirty?: boolean;
  urlsIsDirty?: boolean;
  formIsDirty?: boolean;
  showSubmit?: boolean;
};

function AppriseProviderForm({
  mode,
  name,
  urls,
  onNameChange,
  onUrlsChange,
  onSubmit,
  onCancel,
  onInput,
  nameIsDirty = false,
  urlsIsDirty = false,
  formIsDirty = nameIsDirty || urlsIsDirty,
  showSubmit = true,
}: AppriseProviderFormProps) {
  const { t } = useTranslation();
  return (
    <div
      className="rounded-lg border border-dashed border-muted p-4 space-y-3"
      data-settings-dirty={formIsDirty}
      data-settings-dirty-level="container"
    >
      <div className="text-sm font-medium">{t("settings:appriseProvider")}</div>
      <Input
        value={name}
        onChange={(event) => onNameChange(event.target.value)}
        placeholder={t("settings:providerName")}
        data-settings-dirty={nameIsDirty}
      />
      <Textarea
        value={urls}
        onChange={(event) => onUrlsChange(event.target.value)}
        onInput={onInput}
        placeholder={t("settings:appriseServiceUrls")}
        rows={1}
        className="min-h-0 h-auto"
        data-settings-dirty={urlsIsDirty}
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

function handleTextareaInput(event: FormEvent<HTMLTextAreaElement>) {
  const t = event.currentTarget;
  t.style.height = "auto";
  t.style.height = `${t.scrollHeight}px`;
}
