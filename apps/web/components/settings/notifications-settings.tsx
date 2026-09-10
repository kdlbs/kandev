"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Separator } from "@kandev/ui/separator";
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
import { SettingsTarget } from "@/components/settings/settings-target";
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
      <Separator className="my-4" />
      <SettingsTarget targetId={GENERAL_SETTINGS_TARGETS.notificationEvents} className="space-y-4">
        <div>
          <div className="text-sm font-medium">{t("settings:notificationEvents")}</div>
          <p className="text-xs text-muted-foreground">
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
      </SettingsTarget>
    </SettingsPageTemplate>
  );
}
