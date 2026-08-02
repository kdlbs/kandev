"use client";

import { useMemo, useState, type FormEvent } from "react";
import { IconBell } from "@tabler/icons-react";
import { Trans, useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Separator } from "@kandev/ui/separator";
import { Textarea } from "@kandev/ui/textarea";
import { NotificationSoundSection } from "@/components/settings/notification-sound-section";
import { NotificationEventsTable } from "@/components/settings/notification-events-table";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { DEFAULT_NOTIFICATION_EVENTS } from "@/lib/notifications/events";
import type { NotificationProvider } from "@/lib/types/http";
import {
  DesktopNotificationsSection,
  useNotificationPermission,
} from "@/components/settings/notification-permission-section";
import {
  useNotificationsState,
  useSaveRequest,
  useNotificationsActions,
  useIsDirty,
  type NotificationsState,
  type AppriseFormMode,
} from "@/components/settings/notifications-settings-actions";

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
                onCancel={onCloseForm}
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
  setAppriseName: (v: string) => void;
  setAppriseUrls: (v: string) => void;
  onAppriseNameEdit: (id: string, v: string) => void;
  onAppriseEdit: (id: string, v: string) => void;
  onOpenForm: (mode: AppriseFormMode, provider?: NotificationProvider) => void;
  onCloseForm: () => void;
  onCancelForm: () => void;
  onDeleteProvider: (id: string) => void;
  onTestProvider: (id: string) => Promise<void>;
  onTextareaInput: (e: FormEvent<HTMLTextAreaElement>) => void;
};

function ExternalProvidersSection({
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
  onTextareaInput,
}: ExternalProvidersSectionProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <div>
        <div className="text-base font-medium">{t("settings:externalProviders")}</div>
        <p className="text-sm text-muted-foreground">
          {t("settings:externalProvidersDescription")}
        </p>
      </div>
      {!appriseAvailable && (
        <p className="text-sm text-muted-foreground">
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
        <p className="text-sm text-muted-foreground">{t("settings:noAppriseProviders")}</p>
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
        onTextareaInput={onTextareaInput}
      />
      {appriseAvailable && (
        <div className="space-y-3">
          <Button
            variant="outline"
            className="cursor-pointer"
            onClick={() => onOpenForm("create")}
            disabled={showAppriseForm}
          >
            {t("settings:addAppriseProvider")}
          </Button>
          {showAppriseForm && appriseFormMode === "create" && (
            <AppriseProviderForm
              mode="create"
              name={appriseName}
              urls={appriseUrls}
              onNameChange={setAppriseName}
              onUrlsChange={setAppriseUrls}
              onSubmit={onCloseForm}
              onCancel={onCancelForm}
              onInput={onTextareaInput}
              formIsDirty
              nameIsDirty={appriseName.length > 0}
              urlsIsDirty={appriseUrls.length > 0}
              showSubmit={false}
            />
          )}
        </div>
      )}
    </div>
  );
}

function useTableData(state: NotificationsState) {
  const { providers, notificationEvents } = state;
  const tableProviders = useMemo(
    () =>
      [...providers].sort((a, b) => {
        if (a.type === b.type) return a.name.localeCompare(b.name);
        if (a.type === "local") return -1;
        if (b.type === "local") return 1;
        return a.type.localeCompare(b.type);
      }),
    [providers],
  );
  const tableEvents = useMemo(() => {
    if (notificationEvents.length > 0) return notificationEvents;
    const eventSet = new Set<string>();
    for (const provider of providers) {
      for (const event of provider.events ?? []) eventSet.add(event);
    }
    return eventSet.size ? Array.from(eventSet) : DEFAULT_NOTIFICATION_EVENTS;
  }, [notificationEvents, providers]);
  return { tableProviders, tableEvents };
}

function useNotificationPageSaveState(state: NotificationsState, soundIsDirty: boolean) {
  const { t } = useTranslation();
  const providerIsDirty = useIsDirty(state);
  const creatingApprise = state.showAppriseForm && state.appriseFormMode === "create";
  const canSave = !creatingApprise || state.appriseUrls.trim().length > 0;
  const revision = useMemo(
    () =>
      JSON.stringify({
        providers: state.providers,
        appriseEdits: state.appriseEdits,
        appriseNameEdits: state.appriseNameEdits,
        pendingDeletes: [...state.pendingDeletes].sort(),
        createDraft: creatingApprise ? { name: state.appriseName, urls: state.appriseUrls } : null,
      }),
    [
      creatingApprise,
      state.providers,
      state.appriseEdits,
      state.appriseNameEdits,
      state.pendingDeletes,
      state.appriseName,
      state.appriseUrls,
    ],
  );
  return {
    providerIsDirty,
    cardIsDirty: providerIsDirty || soundIsDirty,
    canSave,
    invalidReason: canSave ? undefined : t("settings:appriseUrlRequired"),
    revision,
  };
}

export function NotificationsSettings() {
  const { t } = useTranslation();
  const state = useNotificationsState();
  const { notificationPermission, refreshPermission } = useNotificationPermission();
  const saveRequest = useSaveRequest(state);
  const actions = useNotificationsActions(state, refreshPermission);
  const [soundIsDirty, setSoundIsDirty] = useState(false);
  const saveState = useNotificationPageSaveState(state, soundIsDirty);
  const { tableProviders, tableEvents } = useTableData(state);
  const {
    providers,
    baselineProviders,
    appriseAvailable,
    appriseName,
    setAppriseName,
    appriseUrls,
    setAppriseUrls,
    showAppriseForm,
    appriseFormMode,
    activeAppriseId,
  } = state;
  const appriseProviders = providers.filter((provider) => provider.type === "apprise");
  return (
    <SettingsPageTemplate
      title={t("settings:notifications")}
      description={t("settings:notificationsDescription")}
      isDirty={saveState.providerIsDirty}
      cardIsDirty={saveState.cardIsDirty}
      saveStatus={saveRequest.status}
      saveRevision={saveState.revision}
      canSave={saveState.canSave}
      invalidReason={saveState.invalidReason}
      onSave={() => saveRequest.run()}
      onDiscard={actions.discard}
    >
      <DesktopNotificationsSection
        notificationPermission={notificationPermission}
        onRequestPermission={actions.handleRequestPermission}
        onRefreshPermission={actions.handleRefreshPermission}
        onTestNotification={actions.handleTestNotification}
      />
      <Separator className="my-4" />
      <NotificationSoundSection onDirtyChange={setSoundIsDirty} />
      <Separator className="my-4" />
      <ExternalProvidersSection
        appriseAvailable={appriseAvailable}
        appriseProviders={appriseProviders}
        baselineProviders={baselineProviders}
        appriseFormMode={appriseFormMode}
        activeAppriseId={activeAppriseId}
        appriseName={appriseName}
        appriseUrls={appriseUrls}
        showAppriseForm={showAppriseForm}
        setAppriseName={setAppriseName}
        setAppriseUrls={setAppriseUrls}
        onAppriseNameEdit={actions.handleAppriseNameEdit}
        onAppriseEdit={actions.handleAppriseEdit}
        onOpenForm={actions.openAppriseForm}
        onCloseForm={actions.closeAppriseForm}
        onCancelForm={actions.cancelAppriseForm}
        onDeleteProvider={actions.handleDeleteProvider}
        onTestProvider={actions.handleTestProvider}
        onTextareaInput={handleTextareaInput}
      />
      <Separator className="my-4" />
      <div className="space-y-4">
        <div>
          <div className="text-base font-medium">{t("settings:notificationEvents")}</div>
          <p className="text-sm text-muted-foreground">
            {t("settings:notificationEventsDescription")}
          </p>
        </div>
        {tableProviders.length > 0 && (
          <NotificationEventsTable
            tableProviders={tableProviders}
            baselineProviders={baselineProviders}
            tableEvents={tableEvents}
            onToggleEvent={actions.handleToggleEvent}
            onTestProvider={actions.handleTestProvider}
          />
        )}
      </div>
    </SettingsPageTemplate>
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
      <div className="text-base font-medium">{t("settings:appriseProvider")}</div>
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
