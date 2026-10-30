"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { NotificationSoundSection } from "@/components/settings/notification-sound-section";
import { NotificationEventsTable } from "@/components/settings/notification-events-table";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { DEFAULT_NOTIFICATION_EVENTS } from "@/lib/notifications/events";
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
} from "@/components/settings/notifications-settings-actions";
import { ExternalProvidersSection } from "@/components/settings/notifications-settings-external-providers";
import { WebhookProvidersSection } from "@/components/settings/notifications-settings-webhook-providers";
import { webhookHeadersError } from "@/components/settings/notifications-settings-webhook-helpers";
import { SettingsGroup } from "@/components/settings/settings-group";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";

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
  const creatingWebhook = state.showWebhookForm && state.webhookFormMode === "create";
  const missingAppriseUrl = creatingApprise && state.appriseUrls.trim().length === 0;
  const missingWebhookUrl = creatingWebhook && state.webhookUrl.trim().length === 0;
  const createHeadersError = creatingWebhook ? webhookHeadersError(state.webhookHeaders) : null;
  const editHeadersError = Object.entries(state.webhookHeaderDrafts)
    .filter(([providerId]) => !state.pendingDeletes.has(providerId))
    .map(([, headers]) => webhookHeadersError(headers))
    .find(Boolean);
  const headersError = createHeadersError ?? editHeadersError ?? null;
  const canSave = !missingAppriseUrl && !missingWebhookUrl && !headersError;
  const revision = useMemo(
    () =>
      JSON.stringify({
        providers: state.providers,
        appriseEdits: state.appriseEdits,
        appriseNameEdits: state.appriseNameEdits,
        webhookHeaderDrafts: state.webhookHeaderDrafts,
        pendingDeletes: [...state.pendingDeletes].sort(),
        createDraft: creatingApprise ? { name: state.appriseName, urls: state.appriseUrls } : null,
        webhookDraft: creatingWebhook
          ? {
              name: state.webhookName,
              url: state.webhookUrl,
              headers: state.webhookHeaders,
              secret: state.webhookSecret,
            }
          : null,
      }),
    [
      creatingApprise,
      creatingWebhook,
      state.providers,
      state.appriseEdits,
      state.appriseNameEdits,
      state.webhookHeaderDrafts,
      state.pendingDeletes,
      state.appriseName,
      state.appriseUrls,
      state.webhookName,
      state.webhookUrl,
      state.webhookHeaders,
      state.webhookSecret,
    ],
  );
  let invalidReason: string | undefined;
  if (missingWebhookUrl) invalidReason = t("settings:webhookUrlRequired");
  else if (missingAppriseUrl) invalidReason = t("settings:appriseUrlRequired");
  else if (headersError) invalidReason = t(headersError);
  return {
    providerIsDirty,
    cardIsDirty: providerIsDirty || soundIsDirty,
    canSave,
    invalidReason,
    revision,
  };
}

function WebhookSettingsSection({
  state,
  actions,
}: {
  state: NotificationsState;
  actions: ReturnType<typeof useNotificationsActions>;
}) {
  return (
    <WebhookProvidersSection
      providers={state.providers.filter((provider) => provider.type === "webhook")}
      baselineProviders={state.baselineProviders}
      formMode={state.webhookFormMode}
      activeWebhookId={state.activeWebhookId}
      name={state.webhookName}
      url={state.webhookUrl}
      headers={state.webhookHeaders}
      headerDrafts={state.webhookHeaderDrafts}
      secret={state.webhookSecret}
      showForm={state.showWebhookForm}
      appriseCreateFormOpen={state.showAppriseForm && state.appriseFormMode === "create"}
      setName={state.setWebhookName}
      setUrl={state.setWebhookUrl}
      setHeaders={state.setWebhookHeaders}
      setSecret={state.setWebhookSecret}
      onNameEdit={(providerId, value) => {
        actions.handleWebhookNameEdit(providerId, value);
        state.setWebhookName(value);
      }}
      onUrlEdit={(providerId, value) => {
        actions.handleWebhookEdit(providerId, { url: value });
        state.setWebhookUrl(value);
      }}
      onHeadersEdit={(providerId, value) => {
        actions.handleWebhookEdit(providerId, { headers: value });
        state.setWebhookHeaders(value);
      }}
      onSecretEdit={(providerId, value) => {
        actions.handleWebhookEdit(providerId, { secret: value });
        state.setWebhookSecret(value);
      }}
      onOpenForm={actions.openWebhookForm}
      onCloseForm={actions.closeWebhookForm}
      onCancelForm={actions.cancelWebhookForm}
      onDeleteProvider={actions.handleDeleteProvider}
      onTestProvider={actions.handleTestProvider}
    />
  );
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
    loaded: notificationProvidersLoaded,
    loading: notificationProvidersLoading,
    rescanApprise,
    appriseRescanPending,
    appriseRescanError,
    appriseRescanResult,
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
      contentFrame="none"
    >
      <div className="space-y-6">
        <DesktopNotificationsSection
          notificationPermission={notificationPermission}
          onRequestPermission={actions.handleRequestPermission}
          onRefreshPermission={actions.handleRefreshPermission}
          onTestNotification={actions.handleTestNotification}
        />
        <NotificationSoundSection onDirtyChange={setSoundIsDirty} />
        <ExternalProvidersSection
          appriseAvailable={appriseAvailable}
          appriseProviders={appriseProviders}
          baselineProviders={baselineProviders}
          appriseFormMode={appriseFormMode}
          activeAppriseId={activeAppriseId}
          appriseName={appriseName}
          appriseUrls={appriseUrls}
          showAppriseForm={showAppriseForm}
          webhookCreateFormOpen={state.showWebhookForm && state.webhookFormMode === "create"}
          notificationProvidersLoaded={notificationProvidersLoaded}
          notificationProvidersLoading={notificationProvidersLoading}
          appriseRescanPending={appriseRescanPending}
          appriseRescanError={appriseRescanError}
          appriseRescanResult={appriseRescanResult}
          setAppriseName={setAppriseName}
          setAppriseUrls={setAppriseUrls}
          onAppriseNameEdit={actions.handleAppriseNameEdit}
          onAppriseEdit={actions.handleAppriseEdit}
          onOpenForm={actions.openAppriseForm}
          onCloseForm={actions.closeAppriseForm}
          onCancelForm={actions.cancelAppriseForm}
          onDeleteProvider={actions.handleDeleteProvider}
          onTestProvider={actions.handleTestProvider}
          onRescanApprise={rescanApprise}
        />
        <WebhookSettingsSection state={state} actions={actions} />
        <SettingsGroup
          title={t("settings:notificationEvents")}
          description={t("settings:notificationEventsDescription")}
          discoveryTargetId={GENERAL_SETTINGS_TARGETS.notificationEvents}
        >
          {tableProviders.length > 0 && (
            <NotificationEventsTable
              tableProviders={tableProviders}
              baselineProviders={baselineProviders}
              tableEvents={tableEvents}
              onToggleEvent={actions.handleToggleEvent}
              onTestProvider={actions.handleTestProvider}
            />
          )}
        </SettingsGroup>
      </div>
    </SettingsPageTemplate>
  );
}
